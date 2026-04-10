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
	SlotIndex     int          // For ActionBuildIndustry: which specific map slot
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

	// 1. Build Industry (76 Actions)
	// One action for every valid industry option in every slot.
	for _, city := range board.Cities {
		for slotIdx, slot := range city.BuildSlots {
			for _, ind := range slot {
				ActionRegistry = append(ActionRegistry, Action{
					ID:           id,
					Type:         ActionBuildIndustry,
					CityID:       city.ID,
					IndustryType: ind,
					SlotIndex:    slotIdx,
					RouteID:      -1,
					RouteID2:     -1,
					MerchantIdx:  -1,
				})
				id++
			}
		}
	}

	// 2. Build Link (39 Actions)
	for _, route := range board.Routes {
		if route.IsSubRoute {
			continue
		}
		ActionRegistry = append(ActionRegistry, Action{
			ID:       id,
			Type:     ActionBuildLink,
			RouteID:  route.ID,
			RouteID2: -1,
			SlotIndex: -1,
			MerchantIdx: -1,
		})
		id++
	}

	// 3. Develop (26 Actions)
	// We generate unique combinations (T1 <= T2) and singles
	for t1 := CottonType; t1 <= BreweryType; t1++ {
		// Single Develop
		ActionRegistry = append(ActionRegistry, Action{
			ID:            id,
			Type:          ActionDevelop,
			IndustryType:  t1,
			IndustryType2: -1,
			SlotIndex:     -1,
			RouteID:       -1,
			RouteID2:      -1,
			MerchantIdx:   -1,
		})
		id++

		// Develop Pairs (T1, T2) where T1 <= T2 to avoid duplicates
		for t2 := t1; t2 <= BreweryType; t2++ {
			if t1 == PotteryType && t2 == PotteryType {
				continue
			}
			ActionRegistry = append(ActionRegistry, Action{
				ID:            id,
				Type:          ActionDevelop,
				IndustryType:  t1,
				IndustryType2: t2,
				SlotIndex:     -1,
				RouteID:       -1,
				RouteID2:      -1,
				MerchantIdx:   -1,
			})
			id++
		}
	}

	// 4. Sell (1 Action)
	// A singular "greedy sell" action. The engine's heuristic calculates the most profitable valid sale sequence.
	ActionRegistry = append(ActionRegistry, Action{
		ID: id, 
		Type: ActionSell,
		SlotIndex: -1,
		RouteID: -1,
		RouteID2: -1,
		MerchantIdx: -1,
	})
	id++

	// 5. Economic (3 Actions)
	ActionRegistry = append(ActionRegistry, Action{ID: id, Type: ActionLoan, SlotIndex: -1, RouteID: -1, RouteID2: -1, MerchantIdx: -1})
	id++
	ActionRegistry = append(ActionRegistry, Action{ID: id, Type: ActionScout, SlotIndex: -1, RouteID: -1, RouteID2: -1, MerchantIdx: -1})
	id++
	ActionRegistry = append(ActionRegistry, Action{ID: id, Type: ActionPass, SlotIndex: -1, RouteID: -1, RouteID2: -1, MerchantIdx: -1})
	id++

	// 6. Double Rail (741 Actions)
	for i := 0; i < len(board.Routes); i++ {
		if board.Routes[i].IsSubRoute {
			continue
		}
		for j := i + 1; j < len(board.Routes); j++ {
			if board.Routes[j].IsSubRoute {
				continue
			}
			ActionRegistry = append(ActionRegistry, Action{
				ID:       id,
				Type:     ActionBuildLinkDouble,
				RouteID:  board.Routes[i].ID,
				RouteID2: board.Routes[j].ID,
				SlotIndex: -1,
				MerchantIdx: -1,
			})
			id++
		}
	}
}

func GetActionSpaceSize() int {
	return len(ActionRegistry)
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
