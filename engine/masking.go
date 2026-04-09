package engine

func (env *Env) GetActionMask() []bool {
	EnsureActionRegistry(env.State.Board)

	if !env.maskDirty && env.cachedMask != nil {
		return env.cachedMask
	}

	size := 12000 // Expert Action Space: 8 slots * 1500 base actions
	if env.cachedMask == nil || len(env.cachedMask) != size {
		env.cachedMask = make([]bool, size)
	}

	player := env.State.Players[env.State.Active]
	for slotIdx := 0; slotIdx < ObsHandSize; slotIdx++ {
		for actionIdx, action := range ActionRegistry {
			if actionIdx >= 1500 {
				break
			}
			totalIdx := actionIdx + (slotIdx * 1500)
			env.cachedMask[totalIdx] = env.isValidActionWithCard(player, action, slotIdx)
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
		slotIdx, overbuild := env.State.GetAvailableBuildSlot(action.CityID, action.IndustryType, p.ID)
		if slotIdx == -1 {
			return false
		}
        if overbuild {
            // Check if it's an opponent's mine/works
            tok := env.State.GetTokenAtSlot(action.CityID, slotIdx)
            if tok != nil && tok.Owner != p.ID {
                // Rule: May only overbuild opponent's coal/iron if board cubes AND market are empty.
                if tok.Industry == CoalMineType {
                     if !env.State.IsResourceExhausted(Coal) { return false }
                } else if tok.Industry == IronWorksType {
                     if !env.State.IsResourceExhausted(Iron) { return false }
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
			if !possible { return false }
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

	case ActionBuildLink:
		route := env.State.Board.Routes[action.RouteID]
		if route.IsBuilt {
			return false
		}

		// 1. Adjacency Check
		if !env.State.IsAdjacentToNetwork(action.RouteID, p.ID) {
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

	case ActionBuildLinkDouble:
		// Fast-path chain: each guard is cheaper than the last.
		// The full ~2080-entry registry is handled efficiently because nearly all
		// entries are eliminated here before any BFS fires.

		// O(1): Canal Era has no double-rail at all
		if env.State.Epoch != RailEra {
			return false
		}
		// O(1): both routes must be unbuilt
		if env.State.Board.Routes[action.RouteID].IsBuilt || env.State.Board.Routes[action.RouteID2].IsBuilt {
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
		// Valid only if the player has at least one industry that can sell to this specific merchant slot.
		if action.MerchantIdx < 0 || action.MerchantIdx >= len(env.State.Merchants) {
			return false
		}
		slot := env.State.Merchants[action.MerchantIdx]
		for _, tok := range env.State.Industries {
			if tok.Owner == p.ID && !tok.Flipped &&
				(tok.Industry == CottonType || tok.Industry == ManufacturedGoodsType || tok.Industry == PotteryType) {
				if env.State.CanSellToMerchant(tok, action.MerchantIdx) {
					// Beer must be available (merchant beer, opponent brewery, or own brewery)
					if slot.AvailableBeer > 0 || env.State.PredictBeerPossible(tok.CityID, p.ID, true, false) {
						return true
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
		// Rule: Can take loan if income level index permits (cannot drop below -10 which is index 0)
		if p.IncomeLevel >= 13 && len(p.Hand) > 0 {
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
