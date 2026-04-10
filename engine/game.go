package engine

import "math/rand"

// ─── Income Track ────────────────────────────────────────────────────────────

var IncomeTrackMap [100]int

func init() {
	for i := 0; i < 100; i++ {
		val := 0
		if i <= 10 {
			val = -10 + i
		} else if i <= 30 {
			val = 0 + (i-10+1)/2
		} else if i <= 60 {
			val = 10 + (i-30+2)/3
		} else {
			val = 20 + (i-60+3)/4
		}
		if val > 30 {
			val = 30
		}
		IncomeTrackMap[i] = val
	}
}

// ─── Era ─────────────────────────────────────────────────────────────────────

type Epoch int

const (
	CanalEra Epoch = iota
	RailEra
)

// ─── Structs ──────────────────────────────────────────────────────────────────

type PlayerState struct {
	ID               PlayerId `json:"id"`
	Money            int      `json:"money"`
	IncomeLevel      int      `json:"income_level"`
	Income           int      `json:"income"`
	AmountSpent      int      `json:"amount_spent"`
	VP               int      `json:"vp"`
	Hand             []Card   `json:"hand"`
	FreeDevelopments int      `json:"free_developments"`

	// Player Board tracking
	CurrentLevel map[IndustryType]int `json:"current_level"`
	TokensLeft   map[IndustryType]int `json:"tokens_left"`

	// Diagnostic/Audit counters
	VPAuditIndustries    int `json:"vp_audit_industries"`
	VPAuditLinks         int `json:"vp_audit_links"`
	ConsumedOpponentCoal int `json:"consumed_opponent_coal"`
	ConsumedOpponentIron int `json:"consumed_opponent_iron"`

	// Scoring breakdown (source -> points)
	ScoringBreakdown map[string]int `json:"scoring_breakdown"`
}

type ScoreEvent struct {
	Source string `json:"source"` // "Birmingham", "Stone <-> Uttoxeter", etc.
	Type   string `json:"type"`   // "Industry", "Link"
	VP     int    `json:"vp"`
	Player int    `json:"player"`
}

type StepMetadata struct {
	ActivePlayer int    `json:"player"`
	ActionName   string `json:"action_name"`
	SlotIndex    int    `json:"slot_idx"`
	IsOverbuild  bool   `json:"is_overbuild"`
	CoalConsumed int    `json:"coal_consumed"`
	IronConsumed int    `json:"iron_consumed"`
	BeerConsumed int    `json:"beer_consumed"`
	Era          string `json:"era"`
	CardsSpent   []Card `json:"cards_spent"`
	CityID       int    `json:"city_id"`
	RouteID      int    `json:"route_id"`

	// Granular scoring added during transitions or specific actions
	ScoreEvents  []ScoreEvent `json:"score_events"`
	ProjectedVPs []int        `json:"projected_vps"`
}

type TokenState struct {
	// Built industries tracking
	Owner     PlayerId     `json:"owner"`
	CityID    CityID       `json:"city_id"`
	SlotIndex int          `json:"slot_idx"`
	Industry  IndustryType `json:"industry"`
	Level     int          `json:"level"`
	Flipped   bool         `json:"flipped"`
	Coal      int          `json:"coal"`
	Iron      int          `json:"iron"`
	Beer      int          `json:"beer"`
}

type MerchantSlot struct {
	CityID        CityID       `json:"city_id"`
	Tile          MerchantTile `json:"tile"`
	AvailableBeer int          `json:"available_beer"`
}

type GameState struct {
	NumPlayers int            `json:"num_players"`
	Epoch      Epoch          `json:"epoch"`
	Players    []*PlayerState `json:"players"`
	Board      *MapGraph      `json:"board"`
	Deck       []Card         `json:"deck"`
	Discard    []Card         `json:"discard"`
	Active     PlayerId       `json:"active"`
	Industries []*TokenState  `json:"industries"`
	Merchants  []MerchantSlot `json:"merchants"`
	CoalMarket Market         `json:"coal_market"`
	IronMarket Market         `json:"iron_market"`

	// Wild card supply tracking (Birmingham deck area)
	WildLocationSupply int `json:"wild_location_supply"`
	WildIndustrySupply int `json:"wild_industry_supply"`

	// Turn and Round Tracking
	TurnOrder        []PlayerId `json:"turn_order"`
	CurrentTurnIdx   int        `json:"current_turn_idx"`
	ActionsRemaining int        `json:"actions_remaining"`
	RoundCounter     int        `json:"round_counter"`

	// Link tracking (moved from MapGraph for purity)
	RouteBuilt  []bool     `json:"route_built"`
	RouteOwners []PlayerId `json:"route_owners"`

	// Per-env isolated RNG
	// and guarantee unique shuffles even when many envs are created concurrently.
	Rng *rand.Rand `json:"-"`

	// GameOver is set to true after the Rail Era is fully exhausted and final scoring is complete.
	GameOver bool `json:"game_over"`

	// BFS scratch — pre-allocated to eliminate map/slice allocations in hot-path graph traversals.
	// Uses a generation counter: a city is "visited" if bfsVisited[cityID] == bfsGen.
	// Incrementing bfsGen effectively clears the visited set in O(1).
	// NOT goroutine-safe: each Env owns its own GameState.
	bfsGen     uint32
	bfsVisited []uint32 // indexed by CityID
	bfsQueue   []CityID // reusable queue; reset to [:0] before each BFS
}

// ─── Constructor ─────────────────────────────────────────────────────────────

func NewGameState(numPlayers int, board *MapGraph, rng *rand.Rand) *GameState {
	gs := &GameState{
		NumPlayers:  numPlayers,
		Epoch:       CanalEra,
		Board:       board,
		Players:     make([]*PlayerState, numPlayers),
		Industries:  make([]*TokenState, 0),
		Merchants:   make([]MerchantSlot, 9),
		TurnOrder:   make([]PlayerId, numPlayers),
		Rng:         rng,
		RouteBuilt:  make([]bool, len(board.Routes)),
		RouteOwners: make([]PlayerId, len(board.Routes)),
	}

	for i := range gs.RouteOwners {
		gs.RouteOwners[i] = -1
	}

	// BFS scratch sized to city count (board must be loaded first)
	n := len(gs.Board.Cities)
	gs.bfsVisited = make([]uint32, n)
	gs.bfsQueue = make([]CityID, 0, n)
	gs.bfsGen = 0

	for i := 0; i < numPlayers; i++ {
		gs.TurnOrder[i] = PlayerId(i)
		gs.Players[i] = &PlayerState{
			ID:               PlayerId(i),
			Money:            17, // Starting money in Brass
			IncomeLevel:      10, // Starting income track position (gives £0)
			Income:           0,
			VP:               0,
			CurrentLevel:     make(map[IndustryType]int),
			TokensLeft:       make(map[IndustryType]int),
			ScoringBreakdown: make(map[string]int),
		}

		// Init tokens on player board based on Level 1 stats
		for ind := CottonType; ind <= BreweryType; ind++ {
			gs.Players[i].CurrentLevel[ind] = 1
			stat := IndustryCatalog[ind][1]
			gs.Players[i].TokensLeft[ind] = stat.Count
		}
	}

	// ── Initialize Merchants ──────────────────────────────────────────────────
	merchantCities := []string{"Shrewsbury", "Gloucester", "Gloucester", "Oxford", "Oxford"}
	if numPlayers == 3 {
		merchantCities = append(merchantCities, "Warrington", "Warrington")
	} else if numPlayers == 4 {
		merchantCities = append(merchantCities, "Warrington", "Warrington", "Nottingham", "Nottingham")
	}

	pool := MerchantPools[numPlayers]
	shuffled := make([]MerchantTile, len(pool))
	copy(shuffled, pool)

	if gs.Rng != nil {
		gs.Rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	}

	gs.Merchants = make([]MerchantSlot, len(merchantCities))
	for i := 0; i < len(merchantCities); i++ {
		gs.Merchants[i] = MerchantSlot{
			CityID: gs.Board.NameMap[merchantCities[i]],
		}
		if i < len(shuffled) {
			gs.Merchants[i].Tile = shuffled[i]
			if len(shuffled[i].Accepts) > 0 {
				gs.Merchants[i].AvailableBeer = 1
			} else {
				gs.Merchants[i].AvailableBeer = 0
			}
			gs.Board.Cities[gs.Merchants[i].CityID].BoardLinkIcons = 2
		} else {
			gs.Merchants[i].Tile = MerchantTile{ID: "empty_0", Accepts: nil}
			gs.Merchants[i].AvailableBeer = 0
		}
	}

	// ── Initialize Markets ────────────────────────────────────────────────────
	gs.CoalMarket = Market{
		Resource:      Coal,
		Prices:        []int{1, 2, 3, 4, 5, 6, 7},
		Capacity:      []int{2, 2, 2, 2, 2, 2, 2},
		CurrentCubes:  []int{1, 2, 2, 2, 2, 2, 2},
		ExternalPrice: 8,
	}
	gs.IronMarket = Market{
		Resource:      Iron,
		Prices:        []int{1, 2, 3, 4, 5},
		Capacity:      []int{2, 2, 2, 2, 2},
		CurrentCubes:  []int{0, 2, 2, 2, 2},
		ExternalPrice: 6,
	}

	gs.Active = gs.TurnOrder[0]
	gs.ActionsRemaining = 1 // Canal Round 1 starts with 1 action
	gs.RoundCounter = 1

	gs.InitializeDeck()

	return gs
}

// ─── Turn / Era Queries ───────────────────────────────────────────────────────

// IsEraOver checks if the current era has reached its total exhaustion.
func (gs *GameState) IsEraOver() bool {
	if len(gs.Deck) > 0 {
		return false
	}
	for _, p := range gs.Players {
		if len(p.Hand) > 0 {
			return false
		}
	}
	return true
}

// TotalCardsLeft returns the sum of all cards in deck and hands.
func (gs *GameState) TotalCardsLeft() int {
	total := len(gs.Deck)
	for _, p := range gs.Players {
		total += len(p.Hand)
	}
	return total
}

// RefillHand ensures a specific player has 8 cards if possible.
func (gs *GameState) RefillHand(pID PlayerId) {
	player := gs.Players[pID]
	for len(player.Hand) < 8 && len(gs.Deck) > 0 {
		card := gs.Deck[0]
		gs.Deck = gs.Deck[1:]
		player.Hand = append(player.Hand, card)
	}
}
