package engine

// ─── Coal ─────────────────────────────────────────────────────────────────────

// SourceCoal attempts to consume N coal from the board or market.
// Prioritizes: Board Coal (Closest → Own → Lowest VP Opponent) > Market (if connected to Merchant).
// Returns the total cost in pounds.
func (gs *GameState) SourceCoal(startCity CityID, count int, activePlayer PlayerId) int {
	needed := count
	totalCost := 0

	for i := 0; i < count; i++ {
		source := gs.findBestCoalSource(startCity, activePlayer)
		if source == nil {
			if gs.IsMerchantConnected(startCity) {
				totalCost += gs.CoalMarket.BuyFromMarket(1)
				needed--
			} else {
				// Failed to source — should be masked out upstream
				return 999999
			}
		} else {
			if source.Owner != activePlayer {
				gs.Players[activePlayer].ConsumedOpponentCoal += 1
			}
			gs.Industries[source.TokenIdx].Coal -= 1
			if gs.Industries[source.TokenIdx].Coal == 0 {
				gs.FlipIndustry(source.TokenIdx)
			}
			needed--
		}
	}
	_ = needed
	return totalCost
}

// PredictCoalCost performs a dry-run of coal consumption.
// Returns the cost and true if successful.
func (gs *GameState) PredictCoalCost(startCity CityID, count int, activePlayer PlayerId) (int, bool) {
	// Temp coal tracking (must be separate from board since we're doing a dry-run)
	tempCoal := make(map[int]int) // TokenIdx -> CoalRemaining
	for i, tok := range gs.Industries {
		if tok.Industry == CoalMineType {
			tempCoal[i] = tok.Coal
		}
	}

	totalCost := 0
	needed := count

	// findBest BFS: uses the generation-counter scratch to avoid map allocation per call.
	findBest := func() *CoalSource {
		type step struct {
			city CityID
			dist int
		}
		gs.bfsGen++
		gs.bfsVisited[startCity] = gs.bfsGen

		queue := []step{{startCity, 0}}
		var candidates []CoalSource
		head := 0
		for head < len(queue) {
			curr := queue[head]
			head++

			for idx, remaining := range tempCoal {
				if gs.Industries[idx].CityID == curr.city && remaining > 0 {
					candidates = append(candidates, CoalSource{
						CityID:   curr.city,
						TokenIdx: idx,
						Distance: curr.dist,
						Owner:    gs.Industries[idx].Owner,
					})
				}
			}

			for _, routeID := range gs.Board.Adj[curr.city] {
				route := gs.Board.Routes[routeID]
				if !route.IsBuilt {
					continue
				}
				next := route.CityA
				if next == curr.city {
					next = route.CityB
				}
				if gs.bfsVisited[next] != gs.bfsGen {
					gs.bfsVisited[next] = gs.bfsGen
					queue = append(queue, step{next, curr.dist + 1})
				}
			}
		}

		if len(candidates) == 0 {
			return nil
		}
		minDist := 999
		for _, c := range candidates {
			if c.Distance < minDist {
				minDist = c.Distance
			}
		}
		var closest []CoalSource
		for _, c := range candidates {
			if c.Distance == minDist {
				closest = append(closest, c)
			}
		}
		for _, c := range closest {
			if c.Owner == activePlayer {
				return &c
			}
		}
		bestOpponentIdx := -1
		minVP := 9999
		for i, c := range closest {
			vp := gs.Players[c.Owner].VP
			if vp < minVP {
				minVP = vp
				bestOpponentIdx = i
			} else if vp == minVP {
				if bestOpponentIdx == -1 || c.Owner < closest[bestOpponentIdx].Owner {
					bestOpponentIdx = i
				}
			}
		}
		if bestOpponentIdx != -1 {
			return &closest[bestOpponentIdx]
		}
		return nil
	}

	tempMarketCubes := make([]int, len(gs.CoalMarket.CurrentCubes))
	copy(tempMarketCubes, gs.CoalMarket.CurrentCubes)

	for i := 0; i < count; i++ {
		source := findBest()
		if source != nil {
			tempCoal[source.TokenIdx]--
			needed--
		} else {
			if gs.IsMerchantConnected(startCity) {
				cost := gs.CoalMarket.PredictNextCubeCost(tempMarketCubes)
				if cost == -1 {
					return 0, false
				}
				totalCost += cost
				needed--
				gs.consumeOneCubeFromTemp(tempMarketCubes)
			} else {
				return 0, false
			}
		}
	}
	_ = needed
	return totalCost, true
}

// ─── Iron ─────────────────────────────────────────────────────────────────────

// SourceIron attempts to consume N iron from the board or market.
// Prioritizes: Active Player Works > Other Player Works > Market > External (£6).
// Returns the total cost in pounds.
func (gs *GameState) SourceIron(count int, activePlayer PlayerId) int {
	needed := count
	totalCost := 0

	// 1. Consume from Active Player's Iron Works first
	for i := range gs.Industries {
		if needed == 0 {
			break
		}
		tok := gs.Industries[i]
		if tok.Industry == IronWorksType && tok.Owner == activePlayer && tok.Iron > 0 {
			taking := needed
			if taking > tok.Iron {
				taking = tok.Iron
			}
			gs.Industries[i].Iron -= taking
			if gs.Industries[i].Iron == 0 {
				gs.FlipIndustry(i)
			}
			needed -= taking
		}
	}

	// 2. Consume from Opponent's Iron Works
	for i := range gs.Industries {
		if needed == 0 {
			break
		}
		tok := gs.Industries[i]
		if tok.Industry == IronWorksType && tok.Owner != activePlayer && tok.Iron > 0 {
			taking := needed
			if taking > tok.Iron {
				taking = tok.Iron
			}
			gs.Players[activePlayer].ConsumedOpponentIron += taking
			gs.Industries[i].Iron -= taking
			if gs.Industries[i].Iron == 0 {
				gs.FlipIndustry(i)
			}
			needed -= taking
		}
	}

	// 3. Iron Market for any remainder
	if needed > 0 {
		totalCost += gs.IronMarket.BuyFromMarket(needed)
	}

	return totalCost
}

// CalculateIronCost performs a dry-run of iron consumption to check for affordability.
func (gs *GameState) CalculateIronCost(count int, activePlayer PlayerId) int {
	needed := count

	for _, tok := range gs.Industries {
		if tok.Industry == IronWorksType && tok.Iron > 0 {
			taking := needed
			if taking > tok.Iron {
				taking = tok.Iron
			}
			needed -= taking
			if needed == 0 {
				return 0
			}
		}
	}

	if needed > 0 {
		return gs.IronMarket.PredictCost(needed)
	}
	return 0
}

// PredictIronCost estimates the cost of sourcing iron without mutating state.
func (gs *GameState) PredictIronCost(count int, playerID PlayerId) int {
	totalCost := 0
	ironLeft := count

	for _, tok := range gs.Industries {
		if tok.Iron > 0 {
			take := tok.Iron
			if take > ironLeft {
				take = ironLeft
			}
			ironLeft -= take
		}
		if ironLeft == 0 {
			return 0
		}
	}

	m := &gs.IronMarket
	for i := 0; i < len(m.CurrentCubes) && ironLeft > 0; i++ {
		available := m.CurrentCubes[i]
		toBuy := available
		if toBuy > ironLeft {
			toBuy = ironLeft
		}
		totalCost += toBuy * m.Prices[i]
		ironLeft -= toBuy
	}

	if ironLeft > 0 {
		totalCost += ironLeft * m.ExternalPrice
	}
	return totalCost
}

// ─── Beer ─────────────────────────────────────────────────────────────────────

// SourceBeer attempts to consume 1 beer according to the priority:
// 1. Merchant Beer (only if atCity is the merchant city, and includeMerchants is true)
// 2. Opponent's Brewery (Connected)
// 3. Active Player's Brewery (No connection required)
func (gs *GameState) SourceBeer(atCity CityID, playerID PlayerId, requireConnection bool, includeMerchants bool) bool {
	if includeMerchants {
		for i, m := range gs.Merchants {
			if m.CityID == atCity && m.AvailableBeer > 0 {
				gs.Merchants[i].AvailableBeer--
				return true
			}
		}
	}

	// Opponent breweries (connection required)
	for i := range gs.Industries {
		tok := gs.Industries[i]
		if tok.Industry == BreweryType && tok.Owner != playerID && tok.Beer > 0 {
			if gs.HasConnectionFast(atCity, tok.CityID) {
				gs.Industries[i].Beer -= 1
				if gs.Industries[i].Beer == 0 {
					gs.FlipIndustry(i)
				}
				return true
			}
		}
	}

	// Own breweries (no connection required)
	for i := range gs.Industries {
		tok := gs.Industries[i]
		if tok.Industry == BreweryType && tok.Owner == playerID && tok.Beer > 0 {
			gs.Industries[i].Beer -= 1
			if gs.Industries[i].Beer == 0 {
				gs.FlipIndustry(i)
			}
			return true
		}
	}

	return false
}

// PredictBeerPossible checks if a beer is available for consumption without state change.
func (gs *GameState) PredictBeerPossible(atCity CityID, playerID PlayerId, requireConnection bool, includeMerchants bool) bool {
	if includeMerchants {
		for _, m := range gs.Merchants {
			if m.CityID == atCity && m.AvailableBeer > 0 {
				return true
			}
		}
	}

	for _, tok := range gs.Industries {
		if tok.Industry == BreweryType && tok.Owner != playerID && tok.Beer > 0 {
			if gs.HasConnectionFast(atCity, tok.CityID) {
				return true
			}
		}
	}

	for _, tok := range gs.Industries {
		if tok.Industry == BreweryType && tok.Owner == playerID && tok.Beer > 0 {
			return true
		}
	}

	return false
}

// ─── Board resource totals ────────────────────────────────────────────────────

// TotalCoalOnBoard counts all coal currently sitting on mines.
func (gs *GameState) TotalCoalOnBoard() int {
	total := 0
	for _, tok := range gs.Industries {
		if tok.Industry == CoalMineType {
			total += tok.Coal
		}
	}
	return total
}

// TotalIronOnBoard counts all iron currently sitting on works.
func (gs *GameState) TotalIronOnBoard() int {
	total := 0
	for _, tok := range gs.Industries {
		if tok.Industry == IronWorksType {
			total += tok.Iron
		}
	}
	return total
}
