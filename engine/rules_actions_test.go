package engine

import (
	"testing"
)

func TestLoanCondition(t *testing.T) {
	p := &PlayerState{
		IncomeLevel: 10, // starting level, value 0
		Hand:        []Card{{Type: LocationCard}},
	}

	// Value at level 10 is 0. 0 - 3 = -3 >= -10. Should be allowed.
	if IncomeTrackMap[p.IncomeLevel]-3 < -10 {
		t.Errorf("Expected loan to be allowed at IncomeLevel 10")
	}

	// Index 0 is value -10. -10 - 3 = -13 < -10. Should be disallowed.
	p.IncomeLevel = 0
	if IncomeTrackMap[p.IncomeLevel]-3 >= -10 {
		t.Errorf("Expected loan to be disallowed at IncomeLevel 0")
	}

	// Index 1 is value -9. -9 - 3 = -12 < -10. Should be disallowed.
	p.IncomeLevel = 1
	if IncomeTrackMap[p.IncomeLevel]-3 >= -10 {
		t.Errorf("Expected loan to be disallowed at IncomeLevel 1")
	}

	// Index 2 is value -8. -8 - 3 = -11 < -10. Should be disallowed.
	p.IncomeLevel = 2
	if IncomeTrackMap[p.IncomeLevel]-3 >= -10 {
		t.Errorf("Expected loan to be disallowed at IncomeLevel 2")
	}

	// Index 3 is value -7. -7 - 3 = -10 >= -10. Should be allowed.
	p.IncomeLevel = 3
	if IncomeTrackMap[p.IncomeLevel]-3 < -10 {
		t.Errorf("Expected loan to be allowed at IncomeLevel 3")
	}
}

func TestDevelopLimitsAndIron(t *testing.T) {
	env := NewEnv(2)
	BuildActionRegistry(env.State.Board)
	p := env.State.Players[env.State.Active]
	p.Money = 100
	p.CurrentLevel[CottonType] = 1 // Level 1 Cotton is developable

	// Hand has 1 card
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}

	// Provide iron on board
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0,
		Industry: IronWorksType,
		Level:    1,
		Iron:     2, // Provide 2 iron
	})

	// Find Single Develop action for Cotton
	actionSingle := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionDevelop && a.IndustryType == CottonType && a.IndustryType2 == -1 {
			actionSingle = a.ID
			break
		}
	}

	if actionSingle == -1 {
		t.Logf("ActionRegistry size: %d", len(ActionRegistry))
		for _, a := range ActionRegistry {
			if a.Type == ActionDevelop {
				t.Logf("Found Develop action: ID=%d, Ind=%d, Ind2=%d", a.ID, a.IndustryType, a.IndustryType2)
			}
		}
		t.Fatal("Could not find Single Develop action for Cotton")
	}

	// Clone state to restore later or just use NewEnv again
	// Let's just do Single Develop first

	// Verify mask says it's allowed
	mask := env.GetActionMask()
	if !mask[actionSingle] {
		t.Errorf("Expected Single Develop to be allowed, got false")
	}

	// Perform action
	env.Step(actionSingle, false, 0)

	// Verify iron consumed
	if env.State.Industries[0].Iron != 1 {
		t.Errorf("Expected 1 iron remaining, got %d", env.State.Industries[0].Iron)
	}

	// Verify card spent in metadata
	if len(env.LastMetadata.CardsSpent) != 1 {
		t.Errorf("Expected 1 card spent, got %d", len(env.LastMetadata.CardsSpent))
	}
}

func TestDevelopNonDevelopable(t *testing.T) {
	env := NewEnv(2)
	BuildActionRegistry(env.State.Board)
	p := env.State.Players[env.State.Active]
	p.Money = 100
	
	// Set Pottery level to 1 (not developable)
	p.CurrentLevel[PotteryType] = 1
	
	// Hand has 1 card
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}

	// Find Single Develop action for Pottery
	actionPottery := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionDevelop && a.IndustryType == PotteryType && a.IndustryType2 == -1 {
			actionPottery = a.ID
			break
		}
	}

	if actionPottery == -1 {
		t.Fatal("Could not find Single Develop action for Pottery")
	}

	mask := env.GetActionMask()

	// Should NOT be allowed to develop Pottery level 1!
	if mask[actionPottery] {
		t.Errorf("Expected cannot develop Pottery level 1, got true")
	}
}

func TestCanalEraRailRestriction(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	p.Money = 100
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, Name: "City0"},
			{ID: 1, Name: "City1"},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, Type: "rail_only", IsBuilt: false},
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0},
		},
	}
	BuildActionRegistry(env.State.Board)

	// Give player a card to allow action
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}
	
	// Player needs network presence or first build exception!
	// Let's give them network presence in City 0 by placing an industry there!
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0,
		Industry: CottonType,
		Level:    1,
	})

	// Set Epoch to CanalEra
	env.State.Epoch = CanalEra

	action0 := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionBuildLink && a.RouteID == 0 {
			action0 = a.ID
			break
		}
	}

	if action0 == -1 {
		t.Fatal("Could not find ActionBuildLink for route 0")
	}

	mask := env.GetActionMask()

	// Should NOT be allowed to build rail_only link in Canal Era!
	if mask[action0] {
		t.Errorf("Expected cannot build rail_only link in Canal Era, got true")
	}
}

func TestRailEraCanalRestriction(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	p.Money = 100
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, Name: "City0"},
			{ID: 1, Name: "City1"},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, Type: "canal_only", IsBuilt: false},
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0},
		},
	}
	BuildActionRegistry(env.State.Board)

	// Give player a card to allow action
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}
	
	// Player needs network presence and COAL for Rail Era!
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0,
		Industry: CoalMineType, // Provides network presence AND coal!
		Level:    1,
		Coal:     1,
	})

	// Set Epoch to RailEra
	env.State.Epoch = RailEra

	action0 := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionBuildLink && a.RouteID == 0 {
			action0 = a.ID
			break
		}
	}

	if action0 == -1 {
		t.Fatal("Could not find ActionBuildLink for route 0")
	}

	mask := env.GetActionMask()

	// Should NOT be allowed to build canal_only link in Rail Era!
	if mask[action0] {
		t.Errorf("Expected cannot build canal_only link in Rail Era, got true")
	}
}

func TestDoubleRailBeerConnection(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	p.Money = 100
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, Name: "City0"},
			{ID: 1, Name: "City1"},
			{ID: 2, Name: "City2"},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, Type: "rail", IsBuilt: false},
			{ID: 1, CityA: 1, CityB: 2, Type: "rail", IsBuilt: false},
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0, 1},
			2: {1},
		},
	}
	BuildActionRegistry(env.State.Board)

	// Give player a card to allow action
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}
	
	// Player needs network presence in City 0
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0,
		Industry: CottonType,
		Level:    1,
	})

	// Provide beer in City 0 (Player's own brewery)
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0,
		Industry: BreweryType,
		Level:    1,
		Beer:     1,
	})

	// Provide coal in City 0 (so it's available for building links)
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0,
		Industry: CoalMineType,
		Level:    1,
		Coal:     2, // Need 2 coal for double rail
	})

	// Set Epoch to RailEra
	env.State.Epoch = RailEra

	// Find Double Rail action for routes 0 and 1
	actionDouble := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionBuildLinkDouble && 
			((a.RouteID == 0 && a.RouteID2 == 1) || (a.RouteID == 1 && a.RouteID2 == 0)) {
			actionDouble = a.ID
			break
		}
	}

	if actionDouble == -1 {
		t.Fatal("Could not find ActionBuildLinkDouble for routes 0 and 1")
	}

	mask := env.GetActionMask()

	// Should be allowed because beer is at City 0, and we build from City 0!
	// Wait, as analyzed, it might FAIL because it checks from City 1 (r2.CityA) and 0-1 is not built!
	// Let's see what the engine does!
	if !mask[actionDouble] {
		t.Errorf("Expected Double Rail to be allowed with connected beer at City 0, got false")
	}
}

func TestSellRequiresConnection(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, Name: "City0"},
			{ID: 1, Name: "City1"},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, Type: "canal", IsBuilt: false},
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0},
		},
	}
	BuildActionRegistry(env.State.Board)

	// Give player a card to allow action
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}
	
	// Player has Cotton Works at City 0
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0,
		Industry: CottonType,
		Level:    1,
		Flipped:  false,
	})

	// Merchant at City 1 accepts Cotton
	env.State.Merchants = []MerchantSlot{
		{
			CityID: 1,
			Tile: MerchantTile{
				Accepts: []IndustryType{CottonType},
			},
			AvailableBeer: 1,
		},
	}

	// Set Epoch to CanalEra
	env.State.Epoch = CanalEra

	// Find Sell action
	actionSell := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionSell {
			actionSell = a.ID
			break
		}
	}

	if actionSell == -1 {
		t.Fatal("Could not find ActionSell")
	}

	mask := env.GetActionMask()

	// Should NOT be allowed to sell because not connected!
	if mask[actionSell] {
		t.Errorf("Expected cannot sell when not connected to merchant, got true")
	}

	// Now build the route!
	env.State.Board.Routes[0].IsBuilt = true
	env.maskDirty = true

	mask = env.GetActionMask()

	// Should be allowed now!
	if !mask[actionSell] {
		t.Errorf("Expected can sell when connected to merchant, got false")
	}
}

func TestSellRequiresBeer(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, Name: "City0"},
			{ID: 1, Name: "City1"},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, Type: "canal", IsBuilt: true}, // Connected!
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0},
		},
	}
	BuildActionRegistry(env.State.Board)

	// Give player a card to allow action
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}
	
	// Player has Cotton Works at City 0
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0,
		Industry: CottonType,
		Level:    1,
		Flipped:  false,
	})

	// Merchant at City 1 accepts Cotton, but has NO BEER!
	env.State.Merchants = []MerchantSlot{
		{
			CityID: 1,
			Tile: MerchantTile{
				Accepts: []IndustryType{CottonType},
			},
			AvailableBeer: 0,
		},
	}

	// Set Epoch to CanalEra
	env.State.Epoch = CanalEra

	// Find Sell action
	actionSell := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionSell {
			actionSell = a.ID
			break
		}
	}

	if actionSell == -1 {
		t.Fatal("Could not find ActionSell")
	}

	mask := env.GetActionMask()

	// Should NOT be allowed to sell because no beer!
	if mask[actionSell] {
		t.Errorf("Expected cannot sell when no beer available, got true")
	}

	// Now provide beer in Player's own brewery (does not need connection!)
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0, // Same city, but connection doesn't matter for own beer anyway
		Industry: BreweryType,
		Level:    1,
		Beer:     1,
	})
	env.maskDirty = true

	mask = env.GetActionMask()

	// Should be allowed now!
	if !mask[actionSell] {
		t.Errorf("Expected can sell when own beer available, got false")
	}
}

func TestSellBeerPriority(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, Name: "City0"},
			{ID: 1, Name: "City1"},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, Type: "canal", IsBuilt: true}, // Connected!
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0},
		},
	}
	BuildActionRegistry(env.State.Board)

	// Give player a card to allow action
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}
	
	// Player has Cotton Works at City 0
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0,
		Industry: CottonType,
		Level:    1,
		Flipped:  false,
	})

	// Merchant at City 1 accepts Cotton, and HAS BEER!
	env.State.Merchants = []MerchantSlot{
		{
			CityID: 1,
			Tile: MerchantTile{
				Accepts: []IndustryType{CottonType},
			},
			AvailableBeer: 1,
		},
	}

	// Player also has own Brewery with beer at City 0!
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0,
		Industry: BreweryType,
		Level:    1,
		Beer:     1,
	})

	// Set Epoch to CanalEra
	env.State.Epoch = CanalEra

	// Find Sell action
	actionSell := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionSell {
			actionSell = a.ID
			break
		}
	}

	if actionSell == -1 {
		t.Fatal("Could not find ActionSell")
	}

	mask := env.GetActionMask()
	if !mask[actionSell] {
		t.Fatal("Expected Sell action to be allowed")
	}

	// Perform action
	env.Step(actionSell, false, 0)

	// Verify Merchant beer was consumed (should be 0)
	if env.State.Merchants[0].AvailableBeer != 0 {
		t.Errorf("Expected Merchant beer to be consumed (0), got %d", env.State.Merchants[0].AvailableBeer)
	}

	// Verify Brewery beer was NOT consumed (should be 1)
	if env.State.Industries[1].Beer != 1 {
		t.Errorf("Expected Brewery beer to be preserved (1), got %d", env.State.Industries[1].Beer)
	}
}

func TestSellIncreasesIncome(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	
	env.State.Board = &MapGraph{
		Cities: []City{
			{ID: 0, Name: "City0"},
			{ID: 1, Name: "City1"},
		},
		Routes: []Route{
			{ID: 0, CityA: 0, CityB: 1, Type: "canal", IsBuilt: true}, // Connected!
		},
		Adj: map[CityID][]int{
			0: {0},
			1: {0},
		},
	}
	BuildActionRegistry(env.State.Board)

	// Give player a card to allow action
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}
	
	// Player has Cotton Works at City 0
	env.State.Industries = append(env.State.Industries, &TokenState{
		Owner:    p.ID,
		CityID:   0,
		Industry: CottonType,
		Level:    1,
		Flipped:  false,
	})

	// Merchant at City 1 accepts Cotton, and HAS BEER!
	env.State.Merchants = []MerchantSlot{
		{
			CityID: 1,
			Tile: MerchantTile{
				Accepts: []IndustryType{CottonType},
			},
			AvailableBeer: 1,
		},
	}

	// Set Epoch to CanalEra
	env.State.Epoch = CanalEra

	// Find Sell action
	actionSell := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionSell {
			actionSell = a.ID
			break
		}
	}

	if actionSell == -1 {
		t.Fatal("Could not find ActionSell")
	}

	initialIncomeLevel := p.IncomeLevel

	// Perform action
	env.Step(actionSell, false, 0)

	// Verify income level increased!
	if p.IncomeLevel <= initialIncomeLevel {
		t.Errorf("Expected income level to increase, started at %d, ended at %d", initialIncomeLevel, p.IncomeLevel)
	}
}

func TestLoanIncomeLimit(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	
	// Give player a card to allow action
	p.Hand = []Card{{Type: LocationCard, CityID: 0}}
	
	// Find Loan action
	actionLoan := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionLoan {
			actionLoan = a.ID
			break
		}
	}

	if actionLoan == -1 {
		t.Fatal("Could not find ActionLoan")
	}

	// Case 1: IncomeLevel maps to -7 cash (index 3)
	// -7 - 3 = -10 >= -10. Should be ALLOWED!
	p.IncomeLevel = 3
	env.maskDirty = true
	mask := env.GetActionMask()
	if !mask[actionLoan] {
		t.Errorf("Expected Loan to be allowed at income level 3 (cash -7), got false")
	}

	// Case 2: IncomeLevel maps to -8 cash (index 2)
	// -8 - 3 = -11 < -10. Should be BLOCKED!
	p.IncomeLevel = 2
	env.maskDirty = true
	mask = env.GetActionMask()
	if mask[actionLoan] {
		t.Errorf("Expected Loan to be blocked at income level 2 (cash -8), got true")
	}
}

func TestScoutRules(t *testing.T) {
	env := NewEnv(2)
	p := env.State.Players[env.State.Active]
	
	// Find Scout action
	actionScout := -1
	for _, a := range ActionRegistry {
		if a.Type == ActionScout {
			actionScout = a.ID
			break
		}
	}

	if actionScout == -1 {
		t.Fatal("Could not find ActionScout")
	}

	// Case 1: Hand size 2, no wild card -> BLOCKED!
	p.Hand = []Card{
		{Type: LocationCard, CityID: 0},
		{Type: LocationCard, CityID: 1},
	}
	env.maskDirty = true
	mask := env.GetActionMask()
	if mask[actionScout] {
		t.Errorf("Expected Scout to be blocked with hand size 2, got true")
	}

	// Case 2: Hand size 3, but HAS wild card -> BLOCKED!
	p.Hand = []Card{
		{Type: LocationCard, CityID: 0},
		{Type: LocationCard, CityID: 1},
		{Type: WildLocationCard},
	}
	env.maskDirty = true
	mask = env.GetActionMask()
	if mask[actionScout] {
		t.Errorf("Expected Scout to be blocked when already holding a wild card, got true")
	}

	// Case 3: Hand size 3, NO wild card -> ALLOWED!
	p.Hand = []Card{
		{Type: LocationCard, CityID: 0},
		{Type: LocationCard, CityID: 1},
		{Type: LocationCard, CityID: 2},
	}
	env.maskDirty = true
	mask = env.GetActionMask()
	if !mask[actionScout] {
		t.Errorf("Expected Scout to be allowed with hand size 3 and no wild cards, got false")
	}
}
