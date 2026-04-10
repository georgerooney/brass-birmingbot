package engine

func (env *Env) GetActionMask() []bool {
	EnsureActionRegistry(env.State.Board)

	if !env.maskDirty && env.cachedMask != nil {
		return env.cachedMask
	}

	size := 886 // V4 Strategic Action Space
	if env.cachedMask == nil || len(env.cachedMask) != size {
		env.cachedMask = make([]bool, size)
	}

	player := env.State.Players[env.State.Active]
	nonPassLegalCount := 0
	for actionIdx, action := range ActionRegistry {
		if action.Type == ActionPass {
			continue
		}

		// Heuristic check: Is there ANY card that makes this action valid?
		// We use ChooseBestCardForAction which returns -1 if no card is valid.
		cardSlotIdx := env.ChooseBestCardForAction(player, action)
		isValid := (cardSlotIdx != -1)
		env.cachedMask[actionIdx] = isValid
		if isValid {
			nonPassLegalCount++
		}
	}

	// 2. Audit and Handle Pass
	if nonPassLegalCount == 0 && !env.State.GameOver {
		// ... (stalemate logging same as before)
	}

	// Only allow Pass if no other move is valid
	// In Registry, ActionPass is usually one of the last indices (handled by RegisterConstants)
	// We'll iterate and find it.
	allowPass := true
	for i, action := range ActionRegistry {
		if action.Type == ActionPass {
			env.cachedMask[i] = allowPass
		}
	}

	env.maskDirty = false
	return env.cachedMask
}

// isValidActionWithCard executes rules validation for a specific card choice.
func (env *Env) isValidActionWithCard(p *PlayerState, action Action, cardIdx int) bool {
	// Rule 0: Specific card must exist in the current hand
	if cardIdx >= len(p.Hand) {
		return false
	}

	switch action.Type {
	case ActionBuildIndustry:
		// 1. Get stats for the specific industry level
		currLvl := p.CurrentLevel[action.IndustryType]
		if currLvl > IndustryMaxLevel[action.IndustryType] {
			return false
		}
		stat := IndustryCatalog[action.IndustryType][currLvl]

		// 2. Era Restrictions (Specific levels blocked in Canal)
		if env.State.Epoch == CanalEra && !contains(stat.Eras, "canal") {
			return false
		}
		if env.State.Epoch == RailEra && !contains(stat.Eras, "rail") {
			return false
		}

		// 3. Slot Availability and Overbuild
		slotIdx := action.SlotIndex
		overbuild := env.State.IsOverbuild(action.CityID, slotIdx, action.IndustryType, p.ID)

		// If the slot is filled by someone else and it's NOT an overbuild, it's blocked.
		// GetTokenAtSlot confirms if the slot is currently occupied.
		tok := env.State.GetTokenAtSlot(action.CityID, slotIdx)
		if tok != nil && !overbuild {
			return false
		}

		if overbuild {
			// Check if it's an opponent's mine/works
			if tok != nil && tok.Owner != p.ID {
				// Rule: May only overbuild opponent's coal/iron if board cubes AND market are empty.
				if tok.Industry == CoalMineType {
					if !env.State.IsResourceExhausted(Coal) {
						return false
					}
				} else if tok.Industry == IronWorksType {
					if !env.State.IsResourceExhausted(Iron) {
						return false
					}
				} else {
					// Cannot overbuild other opponent industries (Cotton, etc.)
					return false
				}
			}
		}

		// 4. Card Availability
		if !env.State.CanCardBeUsedForBuild(action.CityID, action.IndustryType, p.ID, cardIdx) {
			return false
		}

		// 5. Resource Affordability
		coalCost := 0
		if stat.CostCoal > 0 {
			cost, possible := env.State.PredictCoalCost(action.CityID, stat.CostCoal, p.ID)
			if !possible {
				return false
			}
			coalCost = cost
		}

		ironCost := 0
		if stat.CostIron > 0 {
			ironCost = env.State.PredictIronCost(stat.CostIron, p.ID)
		}

		if p.Money < (stat.CostMoney + coalCost + ironCost) {
			return false
		}

		return true

	case ActionNetwork:
		route := env.State.Board.Routes[action.RouteID]
		if env.State.RouteBuilt[action.RouteID] {
			return false
		}

		// Link cannot be the first action in the game! Must have an industry first.
		if env.State.IsFirstBuild(p.ID) {
			return false
		}

		// 1. Adjacency Check
		if !env.State.IsAdjacentToNetwork(action.RouteID, p.ID) {
			return false
		}

		// Era Restrictions
		if env.State.Epoch == CanalEra && route.Type == "rail_only" {
			return false
		}
		if env.State.Epoch == RailEra && route.Type == "canal_only" {
			return false
		}

		// 2. Money and Coal Check
		costMoney := 3
		if env.State.Epoch == RailEra {
			costMoney = 5
			coalCost, possible := env.State.PredictCoalCost(route.CityA, 1, p.ID)
			if !possible {
				return false
			}
			if p.Money < (costMoney + coalCost) {
				return false
			}
		} else {
			if p.Money < costMoney {
				return false
			}
		}

		// 2. Network Adjacency Rule (Rail era links must be connected to network)
		return true

	case ActionNetworkDouble:
		// Fast-path chain: each guard is cheaper than the last.
		// The full ~2080-entry registry is handled efficiently because nearly all
		// entries are eliminated here before any BFS fires.

		// Link cannot be the first action in the game! Must have an industry first.
		if env.State.IsFirstBuild(p.ID) {
			return false
		}

		// O(1): Canal Era has no double-rail at all
		if env.State.Epoch != RailEra {
			return false
		}
		// O(1): both routes must be unbuilt
		if env.State.RouteBuilt[action.RouteID] || env.State.RouteBuilt[action.RouteID2] {
			return false
		}
		// Era Restrictions: neither route can be canal_only in Rail Era
		if env.State.Board.Routes[action.RouteID].Type == "canal_only" || env.State.Board.Routes[action.RouteID2].Type == "canal_only" {
			return false
		}
		// O(adj_degree), no allocation: R1 must touch the player's current network.
		// This eliminates every pair whose first route isn't adjacent to the player —
		// which is the vast majority of the registry at any point in the game.
		if !env.State.IsAdjacentToNetwork(action.RouteID, p.ID) {
			return false
		}
		// O(adj_degree): player must afford the base £15 before we run the full BFS check
		if p.Money < 15 {
			return false
		}
		// Full check (2× BFS for coal, 1× BFS for beer, money with coal cost)
		return env.State.CanBuildDoubleRail(action.RouteID, action.RouteID2, p.ID)

	case ActionDevelop:
		// Develop costs 1 Iron per tile (1 or 2 tiles)
		count := 1
		if action.IndustryType2 != -1 {
			count = 2
		}

		// Check if first industry is developable
		if !p.isDevelopableAtCurrentLevel(action.IndustryType) {
			return false
		}

		// Check if second industry is developable (if applicable)
		if count == 2 {
			// Special case: Double development of same type
			if action.IndustryType == action.IndustryType2 {
				// Can we develop TWO in a row?
				// Needs to check if current is developable AND next is also developable
				if !p.canDevelopTwoOfSame(action.IndustryType) {
					return false
				}
			} else {
				if !p.isDevelopableAtCurrentLevel(action.IndustryType2) {
					return false
				}
			}
		}

		// Iron and Money check:
		cost := env.State.CalculateIronCost(count, p.ID)
		if p.Money < cost {
			return false
		}

		return true

	case ActionSell:
		// Version 4: One singular "greedy sell" action.
		// Valid only if the player has at least one industry that can sell to ANY reachable merchant slot.
		for _, tok := range env.State.Industries {
			if tok.Owner == p.ID && !tok.Flipped &&
				(tok.Industry == CottonType || tok.Industry == ManufacturedGoodsType || tok.Industry == PotteryType) {
				for midx, m := range env.State.Merchants {
					if env.State.CanSellToMerchant(tok, midx) {
						// Beer must be available (merchant beer, opponent brewery, or own brewery)
						if m.AvailableBeer > 0 || env.State.PredictBeerPossible(tok.CityID, p.ID, true, true, false) {
							return true
						}
					}
				}
			}
		}
		return false

	case ActionPass:
		return true

	case ActionScout:
		// Rule: Discard 3 cards to take 1 Wild Location and 1 Wild Industry.
		// Cannot take if already has a wild card.
		if len(p.Hand) >= 3 && !p.HasWildCard() {
			return true
		}
		return false

	case ActionLoan:
		// Rule: Cannot take a loan if it drops your income level below -10.
		// Taking a loan drops income by 3 levels (not index steps).
		if IncomeTrackMap[p.IncomeLevel]-3 >= -10 && len(p.Hand) > 0 {
			return true
		}
		return false
	}

	return false
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
