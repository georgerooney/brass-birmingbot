package engine

// ─── Slot & token queries ─────────────────────────────────────────────────────

// GetAvailableBuildSlot finds the first suitable index in a city for an industry.
// Handles era restrictions and overbuilding rules.
func (gs *GameState) GetAvailableBuildSlot(cityID CityID, ind IndustryType, playerID PlayerId) (slotIdx int, overbuild bool) {
	city := gs.Board.Cities[cityID]

	// Rule: Canal Era — each player may build at most 1 industry per city.
	// Opponents may still build in the same city (their own slot).
	if gs.Epoch == CanalEra {
		for _, tok := range gs.Industries {
			if tok.CityID == cityID && tok.Owner == playerID {
				return -1, false // This player already has an industry in this city
			}
		}
	}

	// 1. Look for an empty matching slot
	for i, slot := range city.BuildSlots {
		matches := false
		for _, sInd := range slot {
			if sInd == ind {
				matches = true
				break
			}
		}
		if !matches {
			continue
		}

		occupied := false
		for _, tok := range gs.Industries {
			if tok.CityID == cityID && tok.SlotIndex == i {
				occupied = true
				break
			}
		}
		if !occupied {
			return i, false
		}
	}

	// 2. Overbuilding Logic
	// Rule: You can overbuild your own tile or an opponent's Coal/Iron if supply exhausted.
	for _, tok := range gs.Industries {
		if tok.CityID != cityID || tok.Industry != ind {
			continue
		}
		if tok.Owner == playerID {
			// Own tile: Must be higher level
			nextLvl := gs.Players[playerID].CurrentLevel[ind]
			if nextLvl > tok.Level {
				return tok.SlotIndex, true
			}
		} else {
			// Opponent tile: Only Coal or Iron, only if board/market exhausted
			if ind == CoalMineType || ind == IronWorksType {
				if gs.IsResourceExhausted(Resource(ind)) {
					return tok.SlotIndex, true
				}
			}
		}
	}

	return -1, false
}

// GetTokenAtSlot returns the industry sitting at a specific slot in a city.
func (gs *GameState) GetTokenAtSlot(cityID CityID, slotIdx int) *TokenState {
	for _, tok := range gs.Industries {
		if tok.CityID == cityID && tok.SlotIndex == slotIdx {
			return tok
		}
	}
	return nil
}

// IsResourceExhausted checks if no cubes of this type exist on the entire board or market.
func (gs *GameState) IsResourceExhausted(res Resource) bool {
	if res == Coal {
		if !gs.IsCoalMarketEmpty() {
			return false
		}
	} else if res == Iron {
		if !gs.IsIronMarketEmpty() {
			return false
		}
	}

	for _, tok := range gs.Industries {
		if res == Coal && tok.Industry == CoalMineType && tok.Coal > 0 {
			return false
		}
		if res == Iron && tok.Industry == IronWorksType && tok.Iron > 0 {
			return false
		}
	}
	return true
}

// ─── Merchant sell ────────────────────────────────────────────────────────────

// CanSellToMerchant checks if an industry token can be sold to a specific merchant slot.
func (gs *GameState) CanSellToMerchant(token *TokenState, merchantIdx int) bool {
	slot := gs.Merchants[merchantIdx]

	// 1. Acceptance check
	accepted := false
	for _, ind := range slot.Tile.Accepts {
		if ind == token.Industry {
			accepted = true
			break
		}
	}
	if !accepted {
		return false
	}

	// 2. Connectivity check (zero-allocation BFS)
	return gs.HasConnectionFast(token.CityID, slot.CityID)
}

// PredictSellableIndustries finds all mills reachable and fulfillable.
func (gs *GameState) PredictSellableIndustries(playerID PlayerId) []int {
	var sellable []int
	for i, tok := range gs.Industries {
		if tok.Owner == playerID && !tok.Flipped &&
			(tok.Industry == CottonType || tok.Industry == ManufacturedGoodsType || tok.Industry == PotteryType) {
			isPossible := false
			for midx := range gs.Merchants {
				if gs.CanSellToMerchant(tok, midx) {
					if gs.Merchants[midx].AvailableBeer > 0 || gs.PredictBeerPossible(tok.CityID, playerID, true, false) {
						isPossible = true
						break
					}
				}
			}
			if isPossible {
				sellable = append(sellable, i)
			}
		}
	}
	return sellable
}
