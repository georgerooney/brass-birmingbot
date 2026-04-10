package engine

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed map_graph.json
var mapGraphData []byte

//go:embed map_industries.json
var mapIndustriesData []byte

// JSON Structures
type TargetNodeJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type NodeGroupJSON struct {
	Merchants     []TargetNodeJSON `json:"merchants"`
	Cities        []TargetNodeJSON `json:"cities"`
	FarmBreweries []TargetNodeJSON `json:"farm_breweries"`
}

type EdgeJSON struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // "both", "rail_only", "canal_only"
}

type MapGraphJSON struct {
	Nodes NodeGroupJSON `json:"nodes"`
	Edges []EdgeJSON    `json:"edges"`
}

type MapIndustriesJSON struct {
	BuildSlots map[string][][]string `json:"build_slots"`
}

type CityID int

type City struct {
	ID             CityID
	StringID       string
	Name           string
	Type           string // "Merchant", "City", "FarmBrewery"
	BuildSlots     [][]IndustryType
	BoardLinkIcons int
}

type Route struct {
	ID         int
	CityA      CityID
	CityB      CityID
	Type       string // For now keep it as string "both", "rail_only", "canal_only"
	IsSubRoute bool   // If true, this route is handled implicitly by a parent route
	SubRoutes  []int  // For hyperedges (e.g. FB2 injection)
}

type MapGraph struct {
	Cities  []City
	Routes  []Route
	Adj     map[CityID][]int // CityID -> list of Route IDs
	NameMap map[string]CityID
}

var (
	sharedBoard     *MapGraph
	sharedBoardOnce sync.Once
)

// GetSharedBoard returns a read-only instance of the map layout.
// This allows 32+ parallel environments to share the same memory for the graph.
func GetSharedBoard() *MapGraph {
	sharedBoardOnce.Do(func() {
		sharedBoard = &MapGraph{
			Adj:     make(map[CityID][]int),
			NameMap: make(map[string]CityID),
		}
		sharedBoard.loadMap()
	})
	return sharedBoard
}

func NewMapGraph() *MapGraph {
	// Deprecated: use GetSharedBoard instead for RL environments.
	mg := &MapGraph{
		Adj:     make(map[CityID][]int),
		NameMap: make(map[string]CityID),
	}
	mg.loadMap()
	return mg
}

func (m *MapGraph) loadMap() {
	var graphJSON MapGraphJSON
	err := json.Unmarshal(mapGraphData, &graphJSON)
	if err != nil {
		fmt.Printf("Error unmarshaling map_graph.json: %v\n", err)
		return
	}

	var currentID int

	addNode := func(node TargetNodeJSON, nodeType string) {
		cid := CityID(currentID)
		m.Cities = append(m.Cities, City{
			ID:       cid,
			StringID: node.ID,
			Name:     node.Name,
			Type:     nodeType,
		})
		m.NameMap[node.Name] = cid
		currentID++
	}

	for _, n := range graphJSON.Nodes.Merchants {
		addNode(n, "Merchant")
	}
	for _, n := range graphJSON.Nodes.Cities {
		addNode(n, "City")
	}
	for _, n := range graphJSON.Nodes.FarmBreweries {
		addNode(n, "FarmBrewery")
	}

	routeIDCounter := 0

	addRoute := func(sourceName, targetName, edgeType string) int {
		srcID, ok1 := m.NameMap[sourceName]
		destID, ok2 := m.NameMap[targetName]
		if !ok1 || !ok2 {
			fmt.Printf("Error: Edge references unknown cities: %s -> %s\n", sourceName, targetName)
			return -1
		}

		routeID := routeIDCounter
		m.Routes = append(m.Routes, Route{
			ID:        routeID,
			CityA:     srcID,
			CityB:     destID,
			Type:      edgeType,
			SubRoutes: []int{},
		})

		m.Adj[srcID] = append(m.Adj[srcID], routeID)
		m.Adj[destID] = append(m.Adj[destID], routeID)

		routeIDCounter++
		return routeID
	}

	for _, edge := range graphJSON.Edges {
		routeID := addRoute(edge.Source, edge.Target, edge.Type)

		// Farm Brewery South (FB2) Hyperedge interception
		if (edge.Source == "Kidderminster" && edge.Target == "Worcester") ||
			(edge.Source == "Worcester" && edge.Target == "Kidderminster") {

			// Implicitly create connections between Kidderminster <-> FB2 and Worcester <-> FB2
			subRoute1 := addRoute("Kidderminster", "Farm Brewery South", edge.Type)
			subRoute2 := addRoute("Worcester", "Farm Brewery South", edge.Type)

			// Map these injected subRoutes back into the primary logical route
			m.Routes[subRoute1].IsSubRoute = true
			m.Routes[subRoute2].IsSubRoute = true
			m.Routes[routeID].SubRoutes = append(m.Routes[routeID].SubRoutes, subRoute1, subRoute2)
		}
	}

	// Load Industries
	var indJSON MapIndustriesJSON
	if err := json.Unmarshal(mapIndustriesData, &indJSON); err != nil {
		fmt.Printf("Error unmarshaling map_industries.json: %v\n", err)
	}

	// Helper converter
	stringToInd := func(s string) IndustryType {
		switch s {
		case "cotton":
			return CottonType
		case "goods":
			return ManufacturedGoodsType
		case "coal":
			return CoalMineType
		case "iron":
			return IronWorksType
		case "pottery":
			return PotteryType
		case "brewery":
			return BreweryType
		default:
			return -1
		}
	}

	// Populate slots
	for i, c := range m.Cities {
		if slots, ok := indJSON.BuildSlots[c.Name]; ok {
			var parsedSlots [][]IndustryType
			for _, slotStrings := range slots {
				var parsedSlot []IndustryType
				for _, str := range slotStrings {
					ind := stringToInd(str)
					if ind != -1 {
						parsedSlot = append(parsedSlot, ind)
					}
				}
				parsedSlots = append(parsedSlots, parsedSlot)
			}
			m.Cities[i].BuildSlots = parsedSlots
		}
	}
}

// HasConnection BFS to check connectivity
func (m *MapGraph) HasConnection(gs *GameState, start, target CityID) bool {
	if start == target {
		return true
	}
	// Use GameState's BFS generation for speed/safety
	gs.bfsGen++
	gs.bfsQueue = gs.bfsQueue[:0]

	gs.bfsQueue = append(gs.bfsQueue, start)
	gs.bfsVisited[start] = gs.bfsGen

	for len(gs.bfsQueue) > 0 {
		curr := gs.bfsQueue[0]
		gs.bfsQueue = gs.bfsQueue[1:]

		for _, routeID := range m.Adj[curr] {
			if !gs.RouteBuilt[routeID] {
				continue
			}

			route := m.Routes[routeID]
			var next CityID
			if route.CityA == curr {
				next = route.CityB
			} else {
				next = route.CityA
			}

			if gs.bfsVisited[next] != gs.bfsGen {
				gs.bfsVisited[next] = gs.bfsGen
				gs.bfsQueue = append(gs.bfsQueue, next)
				if next == target {
					return true
				}
			}
		}
	}
	return false
}
