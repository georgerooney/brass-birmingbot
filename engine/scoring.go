package engine

import (
	"fmt"
	"sort"
)

// ─── VP scoring ───────────────────────────────────────────────────────────────

// FlipIndustry handles the VP and Income gains when a tile is flipped.
func (gs *GameState) FlipIndustry(tokenIdx int) {
	tok := gs.Industries[tokenIdx]
	if tok.Flipped {
		return
	}
	tok.Flipped = true
	p := gs.Players[tok.Owner]
	stat := IndustryCatalog[tok.Industry][tok.Level]

	// VP only added to rule-score at end of era, but we track auditVP immediately
	// for RL rewards and diagnostic logs.
	p.VPAuditIndustries += stat.VP
	p.IncomeLevel += stat.Income
	if p.IncomeLevel > 99 {
		p.IncomeLevel = 99
	}
	p.SyncIncome()

	// Residual Reward: Grant immediate link VP to any link owners connected to this city
	gs.UpdateLinkBonuses(tok.CityID, stat.LinkVP)
}

func (gs *GameState) UpdateLinkBonuses(cityID CityID, bonus int) {
	if bonus == 0 {
		return
	}
	for _, routeID := range gs.Board.Adj[cityID] {
		if gs.RouteBuilt[routeID] && gs.RouteOwners[routeID] != -1 {
			gs.Players[gs.RouteOwners[routeID]].VPAuditLinks += bonus
		}
	}
}

// ScoreEra awards VPs for links and flipped industries at end of an era.
func (gs *GameState) ScoreEra(collectEvents bool) []ScoreEvent {
	var events []ScoreEvent

	// 1. Score Links
	for i := range gs.Board.Routes {
		if gs.RouteBuilt[i] {
			route := gs.Board.Routes[i]
			valA := gs.GetLinkValueForCity(route.CityA)
			valB := gs.GetLinkValueForCity(route.CityB)
			points := valA + valB

			owner := gs.RouteOwners[i]
			p := gs.Players[owner]
			p.VP += points
			p.ScoringBreakdown["Links"] += points

			if collectEvents && points > 0 {
				cityA := gs.Board.Cities[route.CityA].Name
				cityB := gs.Board.Cities[route.CityB].Name
				events = append(events, ScoreEvent{
					Source: fmt.Sprintf("%s <-> %s", cityA, cityB),
					Type:   "Link",
					VP:     points,
					Player: int(owner),
				})
			}
		}
	}

	// 2. Score Flipped Industries
	for _, tok := range gs.Industries {
		if tok.Flipped {
			stat := IndustryCatalog[tok.Industry][tok.Level]
			points := stat.VP

			p := gs.Players[tok.Owner]
			p.VP += points

			indName := IndustryNames[tok.Industry]
			p.ScoringBreakdown[indName] += points

			if collectEvents && points > 0 {
				cityName := gs.Board.Cities[tok.CityID].Name
				events = append(events, ScoreEvent{
					Source: fmt.Sprintf("%s (%s Lvl %d)", cityName, indName, tok.Level),
					Type:   "Industry",
					VP:     points,
					Player: int(tok.Owner),
				})
			}
		}
	}
	return events
}

var IndustryNames = map[IndustryType]string{
	CottonType:            "Cotton",
	CoalMineType:          "Coal",
	IronWorksType:         "Iron",
	PotteryType:           "Pottery",
	ManufacturedGoodsType: "Goods",
	BreweryType:           "Beer",
}

// GetLinkValueForCity calculates the link VP weight of a city (board icons + flipped industry icons).
func (gs *GameState) GetLinkValueForCity(cityID CityID) int {
	city := gs.Board.Cities[cityID]
	total := city.BoardLinkIcons

	for i := range gs.Industries {
		tok := gs.Industries[i]
		if tok.CityID == cityID && tok.Flipped {
			stat := IndustryCatalog[tok.Industry][tok.Level]
			total += stat.LinkVP
		}
	}
	return total
}

// ─── Round / Income ───────────────────────────────────────────────────────────

// ProcessTurnOrder reshuffles the turn order based on amount spent (ascending).
func (gs *GameState) ProcessTurnOrder() {
	sort.SliceStable(gs.TurnOrder, func(i, j int) bool {
		p1 := gs.Players[gs.TurnOrder[i]]
		p2 := gs.Players[gs.TurnOrder[j]]
		return p1.AmountSpent < p2.AmountSpent
	})

	for _, p := range gs.Players {
		p.AmountSpent = 0
	}
}

// ProcessIncome grants income to all players and handles shortfalls.
// Rule: Income is NOT collected in the final round — the caller (env.go) must guard this.
func (gs *GameState) ProcessIncome() {
	for _, pID := range gs.TurnOrder {
		player := gs.Players[pID]
		income := IncomeTrackMap[player.IncomeLevel]
		player.Money += income
		if player.Money < 0 {
			gs.HandleShortfall(pID, -player.Money)
		}
	}
}

// HandleShortfall forces a player to liquidate industries to cover debt.
func (gs *GameState) HandleShortfall(playerID PlayerId, debt int) {
	player := gs.Players[playerID]

	for debt > 0 {
		var candidates []int
		for i, tok := range gs.Industries {
			if tok.Owner == playerID {
				candidates = append(candidates, i)
			}
		}

		if len(candidates) == 0 {
			// Rule: Lose 1 VP per £1 shortfall if no tiles left
			player.VP -= debt
			if player.VP < 0 {
				player.VP = 0
			}
			player.Money = 0
			return
		}

		// Sort by low level first, then low connectivity
		sort.Slice(candidates, func(i, j int) bool {
			tok1 := gs.Industries[candidates[i]]
			tok2 := gs.Industries[candidates[j]]
			if tok1.Level != tok2.Level {
				return tok1.Level < tok2.Level
			}
			conn1 := len(gs.Board.Adj[tok1.CityID])
			conn2 := len(gs.Board.Adj[tok2.CityID])
			return conn1 < conn2
		})

		bestIdx := candidates[0]
		tok := gs.Industries[bestIdx]
		stat := IndustryCatalog[tok.Industry][tok.Level]
		value := stat.CostMoney / 2

		gs.Industries = append(gs.Industries[:bestIdx], gs.Industries[bestIdx+1:]...)
		player.Money += value
		debt -= value

		if player.Money >= 0 {
			return
		}
	}
}

// ─── Era transition ───────────────────────────────────────────────────────────

// EndEraTransition handles the scoring and board wipe between eras.
func (gs *GameState) EndEraTransition() {
	// (Note: Scoring must be handled by caller before transition to capture metadata)

	// 2. Board Wipe — remove all links
	for i := range gs.RouteBuilt {
		gs.RouteBuilt[i] = false
		gs.RouteOwners[i] = -1
	}

	// 3. Remove all Level 1 industries and score surviving Level 2+ flipped tiles AGAIN for Rail Era
	var remaining []*TokenState
	for _, tok := range gs.Industries {
		if tok.Level > 1 {
			remaining = append(remaining, tok)
			// Rule: Flipped industries that survive the transition are scored AGAIN in the Rail Era.
			// We credit this to the audit field now so the RL agent sees the jump.
			if tok.Flipped {
				stat := IndustryCatalog[tok.Industry][tok.Level]
				gs.Players[tok.Owner].VPAuditIndustries += stat.VP
			}
		}
	}
	gs.Industries = remaining

	// 4. Score all surviving Links for the transition bonus?
	// Actually links are REMOVED (Board Wipe), but you get their points at the end of Canal.
	// The VPAuditLinks already has those points from when they were built/adjacent-flipped.

	// 4. Canal Era specific reset → Rail Era
	if gs.Epoch == CanalEra {
		// Reset Merchant Beer
		for i := range gs.Merchants {
			m := &gs.Merchants[i]
			if len(m.Tile.Accepts) > 0 {
				m.AvailableBeer = 1
			}
		}

		gs.Epoch = RailEra
		gs.InitializeDeck() // Distribution is the same for Rail

		gs.RoundCounter = 1
		gs.ActionsRemaining = 2 // Always 2 in Rail
	} else {
		// Rail Era is over -> Game Over
		gs.GameOver = true
	}
}
