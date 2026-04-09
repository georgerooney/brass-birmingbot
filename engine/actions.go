package engine

import "sync"

type ActionType int

const (
	ActionBuildIndustry ActionType = iota
	ActionBuildLink
	ActionBuildLinkDouble
	ActionDevelop
	ActionSell // Just one action - greedy execution handles permutations internally
	ActionLoan
	ActionScout
	ActionPass
)

type Action struct {
	ID            int
	Type          ActionType
	CityID        CityID
	IndustryType  IndustryType
	IndustryType2 IndustryType // For multi-type Develop: if -1, single develop
	RouteID       int
	RouteID2      int // For Double Rail; -1 if unused
	MerchantIdx   int // For ActionSell: which merchant slot to prioritize; -1 if unused
}

var ActionRegistry []Action
var registryOnce sync.Once

// EnsureActionRegistry initialises the registry exactly once, thread-safely.
// All callers (GetActionMask, Step) must use this instead of a raw len check.
func EnsureActionRegistry(board *MapGraph) {
	registryOnce.Do(func() { BuildActionRegistry(board) })
}
// It iterates the static constraints (e.g. valid slots in cities) to
// generate the ultra-lean flattened 1D action space integer list.
func BuildActionRegistry(board *MapGraph) {
	ActionRegistry = []Action{}
	id := 0

	// 1. Build Industry
	for _, city := range board.Cities {
		// Dedup allowed industries for this city's slots
		allowed := make(map[IndustryType]bool)
		for _, slot := range city.BuildSlots {
			for _, ind := range slot {
				allowed[ind] = true
			}
		}

		for ind := CottonType; ind <= BreweryType; ind++ {
			if allowed[ind] {
				ActionRegistry = append(ActionRegistry, Action{
					ID:           id,
					Type:         ActionBuildIndustry,
					CityID:       city.ID,
					IndustryType: ind,
				})
				id++
			}
		}
	}

	// 2. Build Link
	// (SubRoutes like FB2 edges are evaluated inside env.go, so we only list primary routes here)
	for _, route := range board.Routes {
		ActionRegistry = append(ActionRegistry, Action{
			ID:       id,
			Type:     ActionBuildLink,
			RouteID:  route.ID,
			RouteID2: -1,
		})
		id++
	}

	// 2a. Double Rail (Rail Era only — registered for both eras, masked out in Canal).
	// Full O(R²) pair space is registered here. The mask's fast-path chain rejects nearly
	// all entries in O(1) or O(adj_degree) before any BFS fires, so the registry size
	// is not the performance bottleneck. Static prefiltering is deliberately avoided to
	// prevent missing corner-case valid pairs from large player networks.
	for i := 0; i < len(board.Routes); i++ {
		for j := i + 1; j < len(board.Routes); j++ {
			ActionRegistry = append(ActionRegistry, Action{
				ID:       id,
				Type:     ActionBuildLinkDouble,
				RouteID:  board.Routes[i].ID,
				RouteID2: board.Routes[j].ID,
			})
			id++
		}
	}

	// 3. Develop
	// We generate unique combinations (T1 <= T2) and singles
	for t1 := CottonType; t1 <= BreweryType; t1++ {
		// Single Develop
		ActionRegistry = append(ActionRegistry, Action{
			ID:            id,
			Type:          ActionDevelop,
			IndustryType:  t1,
			IndustryType2: -1, // Sentinel for "None"
		})
		id++

		// Develop Pairs (T1, T2) where T1 <= T2 to avoid duplicates
		for t2 := t1; t2 <= BreweryType; t2++ {
			// Rule: Never allow double pottery develop (T1 == T2 == Pottery)
			if t1 == PotteryType && t2 == PotteryType {
				continue
			}

			ActionRegistry = append(ActionRegistry, Action{
				ID:            id,
				Type:          ActionDevelop,
				IndustryType:  t1,
				IndustryType2: t2,
			})
			id++
		}
	}

	// 4. Sell — one action variant per merchant slot (9 total).
	// The policy chooses which merchant to target first; the engine then sells all remaining
	// reachable industries greedily. This exposes merchant bonus routing to the policy.
	for midx := 0; midx < 9; midx++ {
		ActionRegistry = append(ActionRegistry, Action{ID: id, Type: ActionSell, MerchantIdx: midx})
		id++
	}

	// 5. Constants
	ActionRegistry = append(ActionRegistry, Action{ID: id, Type: ActionLoan, MerchantIdx: -1})
	id++
	ActionRegistry = append(ActionRegistry, Action{ID: id, Type: ActionScout, MerchantIdx: -1})
	id++
	ActionRegistry = append(ActionRegistry, Action{ID: id, Type: ActionPass, MerchantIdx: -1})
	id++
}

func GetActionSpaceSize() int {
	return 12000
}

func (a Action) Name(board *MapGraph) string {
	switch a.Type {
	case ActionBuildIndustry:
		city := board.Cities[a.CityID]
		return "Build " + a.IndustryType.String() + " in " + city.Name
	case ActionBuildLink:
		route := board.Routes[a.RouteID]
		return "Build Link: " + board.Cities[route.CityA].Name + " <-> " + board.Cities[route.CityB].Name
	case ActionBuildLinkDouble:
		r1 := board.Routes[a.RouteID]
		r2 := board.Routes[a.RouteID2]
		return "Build Double Rail: [" + board.Cities[r1.CityA].Name + "-" + board.Cities[r1.CityB].Name + "]" +
			" & [" + board.Cities[r2.CityA].Name + "-" + board.Cities[r2.CityB].Name + "]"
	case ActionDevelop:
		if a.IndustryType2 == -1 {
			return "Develop (Single): " + a.IndustryType.String()
		}
		return "Develop (Double): " + a.IndustryType.String() + " & " + a.IndustryType2.String()
	case ActionSell:
		if a.MerchantIdx == -1 {
			return "Sell (Any)"
		}
		return "Sell via Merchant Slot " + string(rune('0'+a.MerchantIdx))
	case ActionLoan:
		return "Take [£30] Loan"
	case ActionScout:
		return "Scout (Discard 3 cards for Wildcards)"
	case ActionPass:
		return "Pass Turn"
	default:
		return "Unknown Action"
	}
}

func (i IndustryType) String() string {
	switch i {
	case CottonType:
		return "Cotton"
	case CoalMineType:
		return "Coal Mine"
	case IronWorksType:
		return "Iron Works"
	case PotteryType:
		return "Pottery"
	case ManufacturedGoodsType:
		return "Mfg Goods"
	case BreweryType:
		return "Brewery"
	default:
		return "Unknown"
	}
}
