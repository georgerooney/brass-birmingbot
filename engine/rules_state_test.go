package engine

import (
	"testing"
)

func TestInitialDiscard(t *testing.T) {
	env := NewEnv(2)

	// Rule: At the start of the game, 1 card is removed from the deck and placed in the discard pile.
	if len(env.State.Discard) != 1 {
		t.Errorf("Expected 1 card in discard pile at start, got %d", len(env.State.Discard))
	}
}

func TestMerchantAssignment(t *testing.T) {
	env := NewEnv(2)

	// Verify that merchants are assigned to slots
	merchantCount := 0
	for _, m := range env.State.Merchants {
		if m.Tile.ID != "" && m.Tile.ID != "empty_0" {
			merchantCount++
		}
	}

	// For 2 players, how many merchants should there be?
	// In game.go:
	// merchantCities := []string{"Shrewsbury", "Gloucester", "Gloucester", "Oxford", "Oxford"}
	// So 5 slots!
	if merchantCount == 0 {
		t.Errorf("Expected merchants to be assigned, got 0")
	}
}

func TestTurnOrder(t *testing.T) {
	env := NewEnv(2)

	// Set spending
	env.State.Players[0].AmountSpent = 20
	env.State.Players[1].AmountSpent = 10

	// Process turn order
	env.State.ProcessTurnOrder()

	// Player 1 spent less, so should be first!
	if env.State.TurnOrder[0] != 1 {
		t.Errorf("Expected Player 1 to be first in turn order, got %d", env.State.TurnOrder[0])
	}
	if env.State.TurnOrder[1] != 0 {
		t.Errorf("Expected Player 0 to be second in turn order, got %d", env.State.TurnOrder[1])
	}

	// Verify AmountSpent was reset
	if env.State.Players[0].AmountSpent != 0 {
		t.Errorf("Expected Player 0 AmountSpent to be reset, got %d", env.State.Players[0].AmountSpent)
	}
}

func TestCardRefill(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]

	// Reduce hand size to 6
	p.Hand = p.Hand[:6]

	// Ensure deck has cards
	if len(env.State.Deck) < 2 {
		t.Fatal("Deck has too few cards for test")
	}

	initialDeckSize := len(env.State.Deck)

	// Refill hand
	env.State.RefillHand(p.ID)

	// Verify hand size is 8
	if len(p.Hand) != 8 {
		t.Errorf("Expected hand size 8 after refill, got %d", len(p.Hand))
	}

	// Verify deck size decreased by 2
	if len(env.State.Deck) != initialDeckSize-2 {
		t.Errorf("Expected deck size to decrease by 2, started at %d, ended at %d", initialDeckSize, len(env.State.Deck))
	}
}

func TestEraTransition(t *testing.T) {
	env := NewEnv(2)

	// Build a link on the default board (Route 0)
	if len(env.State.Board.Routes) == 0 {
		t.Fatal("Board has no routes")
	}
	env.State.Board.Routes[0].IsBuilt = true
	env.State.Board.Routes[0].Owner = 0

	// Setup industries
	env.State.Industries = []*TokenState{
		{Owner: 0, CityID: 0, Industry: CottonType, Level: 1},
		{Owner: 0, CityID: 1, Industry: CottonType, Level: 2},
	}

	env.State.Epoch = CanalEra

	// Perform transition
	env.State.EndEraTransition()

	// Verify links are wiped
	if env.State.Board.Routes[0].IsBuilt {
		t.Errorf("Expected links to be wiped, but Route 0 is still built")
	}

	// Verify Level 1 industry is removed
	foundLvl1 := false
	foundLvl2 := false
	for _, tok := range env.State.Industries {
		if tok.Level == 1 {
			foundLvl1 = true
		}
		if tok.Level == 2 {
			foundLvl2 = true
		}
	}

	if foundLvl1 {
		t.Errorf("Expected Level 1 industries to be removed, but found one")
	}
	if !foundLvl2 {
		t.Errorf("Expected Level 2 industries to be preserved, but not found")
	}

	// Verify Epoch advanced
	if env.State.Epoch != RailEra {
		t.Errorf("Expected Epoch to advance to RailEra, got %v", env.State.Epoch)
	}
}

func TestScoredAgain(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]

	// Setup a Level 2 industry and FLIP it!
	env.State.Industries = []*TokenState{
		{Owner: p.ID, CityID: 0, Industry: CottonType, Level: 2, Flipped: true},
	}

	env.State.Epoch = CanalEra

	// Manually set initial VPAuditIndustries to simulate it was scored when flipped
	stat := IndustryCatalog[CottonType][2]
	p.VPAuditIndustries = stat.VP

	// Perform transition
	env.State.EndEraTransition()

	// Verify it scored AGAIN!
	expectedVP := stat.VP * 2
	if p.VPAuditIndustries != expectedVP {
		t.Errorf("Expected VPAuditIndustries to be %d (scored twice), got %d", expectedVP, p.VPAuditIndustries)
	}
}

func TestLinkScoring(t *testing.T) {
	env := NewEnv(2)

	// Setup board
	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, BoardLinkIcons: 1},
			{ID: 1, BoardLinkIcons: 2},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, IsBuilt: true, Owner: 0},
		},
	}

	// Setup a flipped industry at City 0
	env.State.Industries = []*TokenState{
		{Owner: 0, CityID: 0, Industry: CottonType, Level: 1, Flipped: true},
	}

	stat := IndustryCatalog[CottonType][1]

	// Perform scoring
	env.State.ScoreEra(false)

	// Expected VP for Player 0:
	// City 0 value: BoardLinkIcons (1) + stat.LinkVP
	// City 1 value: BoardLinkIcons (2)
	// Total link value = (1 + stat.LinkVP) + 2 = 3 + stat.LinkVP

	expectedVP := 3 + stat.LinkVP

	p := env.State.Players[0]
	if p.ScoringBreakdown["Links"] != expectedVP {
		t.Errorf("Expected Link VP to be %d, got %d", expectedVP, p.ScoringBreakdown["Links"])
	}
}

func TestBuyCoalRequiresMerchantConnection(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]

	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, Name: "City0"},
			{ID: 1, Name: "City1", Type: "Merchant"},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, Type: "canal", IsBuilt: false}, // NOT built!
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0},
		},
	}
	BuildActionRegistry(env.State.Board)

	// Merchant at City 1 accepts Cotton
	env.State.Merchants = []MerchantSlot{
		{
			CityID: 1,
			Tile: MerchantTile{
				Accepts: []IndustryType{CottonType},
			},
			AvailableBeer: 0,
		},
	}

	// Give player a card to allow action
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}

	// Case 1: Not connected -> Should fail (return large cost)
	cost := env.State.SourceCoal(0, 1, p.ID)
	if cost != 999999 {
		t.Errorf("Expected SourceCoal to fail (return 999999) when not connected to merchant, got %d", cost)
	}

	// Case 2: Connected -> Should succeed (buy from market)
	env.State.Board.Routes[0].IsBuilt = true
	env.maskDirty = true

	// Ensure market has coal
	env.State.CoalMarket.CurrentCubes[0] = 1

	cost = env.State.SourceCoal(0, 1, p.ID)
	if cost == 999999 {
		t.Errorf("Expected SourceCoal to succeed when connected to merchant, but it failed")
	}
}

func TestGameOver(t *testing.T) {
	env := NewEnv(2)

	// Empty deck
	env.State.Deck = []Card{}

	// Empty hands
	for i := range env.State.Players {
		env.State.Players[i].Hand = []Card{}
	}

	if !env.State.IsEraOver() {
		t.Errorf("Expected IsEraOver to be true when deck and hands are empty")
	}

	// Give one player a card
	env.State.Players[0].Hand = []Card{{Type: LocationCard, CityID: 0}}

	if env.State.IsEraOver() {
		t.Errorf("Expected IsEraOver to be false when a player has cards")
	}
}
