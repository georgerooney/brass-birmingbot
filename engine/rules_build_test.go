package engine

import (
	"testing"
)

func TestBuildFunds(t *testing.T) {
	env := NewEnv(2)
	env.State.Board = &MapGraph{
		Cities: []City{
			{
				ID:   0,
				Name: "TestCity",
				BuildSlots: [][]IndustryType{
					{CottonType},
				},
			},
		},
	}
	BuildActionRegistry(env.State.Board)

	p := env.State.Players[env.State.Active]
	cityID := CityID(0)
	ind := CottonType

	p.Hand = []Card{{Type: LocationCard, CityID: int(cityID)}}
	p.Money = 0
	p.CurrentLevel[CottonType] = 1

	actionID := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionBuildIndustry && a.CityID == cityID && a.IndustryType == ind {
			actionID = a.ID
			break
		}
	}
	if actionID == -1 {
		t.Fatal("Could not find build action in registry")
	}

	mask := env.GetActionMask()
	if mask[actionID] {
		t.Errorf("Expected build action to be masked out (false) due to insufficient funds, got true")
	}

	// Set money to high value and invalidate mask cache
	p.Money = 100
	env.maskDirty = true
	
	mask = env.GetActionMask()
	if !mask[actionID] {
		t.Errorf("Expected build action to be allowed (true) with sufficient funds, got false")
	}
}

func TestBuildEmptyHand(t *testing.T) {
	env := NewEnv(2)
	env.State.Board = &MapGraph{
		Cities: []City{
			{
				ID:   0,
				Name: "TestCity",
				BuildSlots: [][]IndustryType{
					{CottonType},
				},
			},
		},
	}
	BuildActionRegistry(env.State.Board)

	p := env.State.Players[env.State.Active]
	p.Money = 100
	p.Hand = []Card{} // Empty hand

	mask := env.GetActionMask()

	for i, m := range mask {
		if m && ActionRegistry[i].Type != ActionPass {
			t.Errorf("Expected only ActionPass to be true with empty hand, but action %d (%v) was true", i, ActionRegistry[i].Type)
		}
	}
}

func TestLocationCard(t *testing.T) {
	env := NewEnv(2)
	env.State.Board = &MapGraph{
		Cities: []City{
			{
				ID:   0,
				Name: "City0",
				BuildSlots: [][]IndustryType{{CottonType}},
			},
			{
				ID:   1,
				Name: "City1",
				BuildSlots: [][]IndustryType{{CottonType}},
			},
		},
	}
	BuildActionRegistry(env.State.Board)

	p := env.State.Players[env.State.Active]
	p.Money = 100
	p.CurrentLevel[CottonType] = 1

	// Hand has Location card for City 1
	p.Hand = []Card{{Type: LocationCard, CityID: 1}}

	// Find action IDs
	action0 := -1
	action1 := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionBuildIndustry && a.IndustryType == CottonType {
			if a.CityID == 0 {
				action0 = a.ID
			} else if a.CityID == 1 {
				action1 = a.ID
			}
		}
	}

	mask := env.GetActionMask()

	if mask[action0] {
		t.Errorf("Expected cannot build in City 0 with Location card for City 1")
	}
	if !mask[action1] {
		t.Errorf("Expected can build in City 1 with Location card for City 1")
	}
}

func TestIndustryCard(t *testing.T) {
	env := NewEnv(2)
	env.State.Board = &MapGraph{
		Cities: []City{
			{
				ID:   0,
				Name: "City0",
				BuildSlots: [][]IndustryType{{CottonType}},
			},
			{
				ID:   1,
				Name: "City1",
				BuildSlots: [][]IndustryType{{CottonType}},
			},
		},
		Routes: []Route{
			{
				ID:      0,
				CityA:   0,
				CityB:   1,
				Type:    "canal",
				IsBuilt: false,
			},
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0},
		},
	}
	BuildActionRegistry(env.State.Board)

	p := env.State.Players[env.State.Active]
	p.Money = 100
	p.CurrentLevel[CottonType] = 1

	// Hand has Industry card for Cotton
	p.Hand = []Card{{Type: IndustryCard, Industry: CottonType}}

	// Place an industry in City 0 for this player to establish network
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:     p.ID,
		CityID:    0,
		SlotIndex: 0,
		Industry:  CottonType,
		Level:     1,
	})

	action1 := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionBuildIndustry && a.CityID == 1 && a.IndustryType == CottonType {
			action1 = a.ID
			break
		}
	}

	mask := env.GetActionMask()

	// Route is NOT built, so City 1 is NOT connected to player's network (which is only at City 0)
	if mask[action1] {
		t.Errorf("Expected cannot build in City 1 with Industry card when not connected")
	}

	// Now build the route
	env.State.Board.Routes[0].IsBuilt = true
	env.maskDirty = true

	mask = env.GetActionMask()
	if !mask[action1] {
		t.Errorf("Expected can build in City 1 with Industry card when connected")
	}
}

func TestCanalEraLimit(t *testing.T) {
	gs := &GameState{
		Epoch: CanalEra,
		Board: &MapGraph{
			Cities: []City{
				{
					ID:   0,
					Name: "TestCity",
					BuildSlots: [][]IndustryType{
						{CottonType},
						{BreweryType},
					},
				},
			},
		},
	}

	playerID := PlayerId(0)
	cityID := CityID(0)

	// Place an industry for player 0 in city 0
	gs.Industries = append(gs.Industries, &TokenState{
		Owner:     playerID,
		CityID:    cityID,
		SlotIndex: 0,
		Industry:  CottonType,
		Level:     1,
	})

	// Try to get a slot for another industry for the same player in the same city
	slotIdx, overbuild := gs.GetAvailableBuildSlot(cityID, BreweryType, playerID)
	if slotIdx != -1 {
		t.Errorf("Expected no available slot in Canal Era for player with existing industry in city, got slot %d", slotIdx)
	}

	// Opponent should still be able to build
	opponentID := PlayerId(1)
	slotIdx, overbuild = gs.GetAvailableBuildSlot(cityID, BreweryType, opponentID)
	if slotIdx != 1 || overbuild {
		t.Errorf("Expected opponent to be able to build in empty slot 1, got slot %d, overbuild=%t", slotIdx, overbuild)
	}
}

func TestOverbuildOwnTile(t *testing.T) {
	gs := &GameState{
		Epoch: RailEra,
		Board: &MapGraph{
			Cities: []City{
				{
					ID:   0,
					Name: "TestCity",
					BuildSlots: [][]IndustryType{
						{CottonType},
					},
				},
			},
		},
		Players: []*PlayerState{
			&PlayerState{
				CurrentLevel: map[IndustryType]int{CottonType: 2},
			},
		},
	}

	playerID := PlayerId(0)
	cityID := CityID(0)

	// Place a level 1 industry
	gs.Industries = append(gs.Industries, &TokenState{
		Owner:     playerID,
		CityID:    cityID,
		SlotIndex: 0,
		Industry:  CottonType,
		Level:     1,
	})

	// Check if we can overbuild
	slotIdx, overbuild := gs.GetAvailableBuildSlot(cityID, CottonType, playerID)
	if slotIdx != 0 || !overbuild {
		t.Errorf("Expected to be able to overbuild own tile at slot 0, got slot %d, overbuild=%t", slotIdx, overbuild)
	}
}

func TestFirstBuildException(t *testing.T) {
	env := NewEnv(2)
	env.State.Board = &MapGraph{
		Cities: []City{
			{
				ID:   0,
				Name: "City0",
				BuildSlots: [][]IndustryType{{CottonType}},
			},
			{
				ID:   1,
				Name: "City1",
				BuildSlots: [][]IndustryType{{CottonType}},
			},
		},
	}
	BuildActionRegistry(env.State.Board)

	p := env.State.Players[env.State.Active]
	p.Money = 100
	p.CurrentLevel[CottonType] = 1

	// Hand has Industry card for Cotton
	p.Hand = []Card{{Type: IndustryCard, Industry: CottonType}}

	// Player has NO industries on board yet (first build)

	action0 := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionBuildIndustry && a.CityID == 0 && a.IndustryType == CottonType {
			action0 = a.ID
			break
		}
	}

	mask := env.GetActionMask()

	// Should be allowed to build in City 0 (or any city with slot) as first build!
	if !mask[action0] {
		t.Errorf("Expected can build in City 0 with Industry card as first build (exception), got false")
	}
}

func TestIronWorksCoal(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{
				ID:   0,
				Name: "City0",
				BuildSlots: [][]IndustryType{{IronWorksType}},
			},
			{
				ID:   1,
				Name: "City1",
				BuildSlots: [][]IndustryType{{CoalMineType}},
			},
		},
		Routes: []Route{
			{
				ID:      0,
				CityA:   0,
				CityB:   1,
				Type:    "canal",
				IsBuilt: true,
				Owner:   p.ID,
			},
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0},
		},
	}
	BuildActionRegistry(env.State.Board)

	p.Money = 100
	p.CurrentLevel[IronWorksType] = 1

	// Hand has Location card for City 0
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}

	// Empty coal market
	env.State.CoalMarket.CurrentCubes = []int{0, 0, 0, 0, 0, 0, 0}

	action0 := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionBuildIndustry && a.CityID == 0 && a.IndustryType == IronWorksType {
			action0 = a.ID
			break
		}
	}

	mask := env.GetActionMask()

	// Should NOT be allowed to build Iron Works because it costs coal and no coal is available!
	if mask[action0] {
		t.Errorf("Expected cannot build Iron Works without coal, got true")
	}

	// Now provide coal on board in City 1
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:     p.ID,
		CityID:    1,
		Industry:  CoalMineType,
		Level:     1,
		Coal:      1,
	})

	env.maskDirty = true
	mask = env.GetActionMask()

	// Should NOW be allowed to build Iron Works!
	if !mask[action0] {
		t.Errorf("Expected can build Iron Works with coal on board, got false")
	}
}

func TestResourcePriority(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, Name: "City0", BuildSlots: [][]IndustryType{{IronWorksType}}},
			{ID: 1, Name: "City1", BuildSlots: [][]IndustryType{{CoalMineType}}},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, Type: "canal", IsBuilt: true, Owner: p.ID},
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0},
		},
	}
	BuildActionRegistry(env.State.Board)

	p.Money = 5 // Exact cost of Iron Works
	p.CurrentLevel[IronWorksType] = 1
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}

	// Setup Coal Market with cubes at price 1
	env.State.CoalMarket.CurrentCubes = []int{1, 0, 0, 0, 0, 0, 0}

	// Provide coal on board in City 1
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:     p.ID,
		CityID:    1,
		Industry:  CoalMineType,
		Level:     1,
		Coal:      1,
	})

	action0 := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionBuildIndustry && a.CityID == 0 && a.IndustryType == IronWorksType {
			action0 = a.ID
			break
		}
	}

	mask := env.GetActionMask()

	// Should be allowed because it should use FREE board coal!
	if !mask[action0] {
		t.Errorf("Expected can build Iron Works using free board coal, got false")
	}

	// Now remove board coal
	env.State.Industries[0].Coal = 0
	env.maskDirty = true

	mask = env.GetActionMask()

	// Should NOT be allowed now because it must buy from market, costing £1, total £6 > £5!
	if mask[action0] {
		t.Errorf("Expected cannot build Iron Works when forced to buy unaffordable market coal, got true")
	}
}

func TestResourceProximity(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, Name: "City0"},
			{ID: 1, Name: "City1"},
			{ID: 2, Name: "City2"},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, Type: "canal", IsBuilt: true, Owner: p.ID},
			{ID: 1, CityA: 1, CityB: 2, Type: "canal", IsBuilt: true, Owner: p.ID},
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0, 1},
			2: {1},
		},
	}
	BuildActionRegistry(env.State.Board)

	// Provide coal in City 1 (distance 1)
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:     p.ID,
		CityID:    1,
		Industry:  CoalMineType,
		Level:     1,
		Coal:      1,
	})

	// Provide coal in City 2 (distance 2)
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:     p.ID,
		CityID:    2,
		Industry:  CoalMineType,
		Level:     1,
		Coal:      1,
	})

	// Call SourceCoal directly from City 0
	cost := env.State.SourceCoal(0, 1, p.ID)
	
	if cost != 0 {
		t.Errorf("Expected cost to be 0 (board coal used), got %d", cost)
	}

	// Verify that Coal Mine in City 1 (closer) was used!
	if env.State.Industries[0].Coal != 0 {
		t.Errorf("Expected Coal Mine at City 1 to be consumed (0 coal), got %d", env.State.Industries[0].Coal)
	}
	if env.State.Industries[1].Coal != 1 {
		t.Errorf("Expected Coal Mine at City 2 to be untouched (1 coal), got %d", env.State.Industries[1].Coal)
	}
}

func TestResourceOwnership(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	opponentID := PlayerId(1)
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, Name: "City0"},
			{ID: 1, Name: "City1"},
			{ID: 2, Name: "City2"},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, Type: "canal", IsBuilt: true, Owner: p.ID},
			{ID: 1, CityA: 0, CityB: 2, Type: "canal", IsBuilt: true, Owner: p.ID},
		},
		Adj: map[CityID][]int{
			0: {0, 1},
			1: {0},
			2: {1},
		},
	}
	BuildActionRegistry(env.State.Board)

	// Provide coal in City 1 (Opponent)
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:     opponentID,
		CityID:    1,
		Industry:  CoalMineType,
		Level:     1,
		Coal:      1,
	})

	// Provide coal in City 2 (Player)
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:     p.ID,
		CityID:    2,
		Industry:  CoalMineType,
		Level:     1,
		Coal:      1,
	})

	// Call SourceCoal directly from City 0
	cost := env.State.SourceCoal(0, 1, p.ID)
	
	if cost != 0 {
		t.Errorf("Expected cost to be 0 (board coal used), got %d", cost)
	}

	// Verify that Coal Mine in City 2 (Player's own) was used!
	if env.State.Industries[1].Coal != 0 {
		t.Errorf("Expected Player's Coal Mine at City 2 to be consumed (0 coal), got %d", env.State.Industries[1].Coal)
	}
	if env.State.Industries[0].Coal != 1 {
		t.Errorf("Expected Opponent's Coal Mine at City 1 to be untouched (1 coal), got %d", env.State.Industries[0].Coal)
	}
}

func TestRailEraLevel1Restriction(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{
				ID:   0,
				Name: "City0",
				BuildSlots: [][]IndustryType{{CottonType}},
			},
		},
	}
	BuildActionRegistry(env.State.Board)

	p.Money = 100
	p.CurrentLevel[CottonType] = 1 // Level 1 Cotton
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}

	// Set Epoch to RailEra
	env.State.Epoch = RailEra

	action0 := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionBuildIndustry && a.CityID == 0 && a.IndustryType == CottonType {
			action0 = a.ID
			break
		}
	}

	mask := env.GetActionMask()

	// Should NOT be allowed to build Level 1 industry in Rail Era!
	if mask[action0] {
		t.Errorf("Expected cannot build Level 1 industry in Rail Era, got true")
	}
}
