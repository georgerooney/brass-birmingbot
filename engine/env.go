package engine

import (
	"fmt"
	"math/rand"
	"sort"
	"sync/atomic"
	"time"
)

// envCounter ensures each Env gets a unique RNG seed even when many are created simultaneously.
var envCounter int64

type Env struct {
	State      *GameState
	maskDirty  bool   // true after any Step; cleared after GetActionMask recomputes
	cachedMask []bool // reused buffer to avoid allocation on every mask request

	LastAuditVPIndustries [ObsMaxPlayers]int
	LastAuditVPLinks      [ObsMaxPlayers]int
	LastIncome            [ObsMaxPlayers]int

	// Diagnostic metadata from the last Step
	LastMetadata StepMetadata
}

func NewEnv(numPlayers int) *Env {
	// XOR wall-clock with an atomic counter so concurrent NewEnv calls produce distinct seeds.
	seed := time.Now().UnixNano() ^ atomic.AddInt64(&envCounter, 1337)
	rng := rand.New(rand.NewSource(seed))
	env := &Env{
		State:     NewGameState(numPlayers, rng),
		maskDirty: true,
	}
	env.syncLastState()
	return env
}

func (e *Env) syncLastState() {
	for i := 0; i < e.State.NumPlayers; i++ {
		p := e.State.Players[i]
		p.SyncIncome()
		e.LastAuditVPIndustries[i] = p.VPAuditIndustries
		e.LastAuditVPLinks[i] = p.VPAuditLinks
		e.LastIncome[i] = p.IncomeLevel
	}
}

func (e *Env) Reset() {
	// Preserve the per-env RNG so the random sequence continues without re-seeding.
	rng := e.State.Rng
	e.State = NewGameState(e.State.NumPlayers, rng)
	e.maskDirty = true
	e.syncLastState()
}

// InvalidateMask forces the action mask to be recomputed on the next get.
// Primarily used for testing when mutating GameState directly.
func (e *Env) InvalidateMask() {
	e.maskDirty = true
}

const ActionSpaceSize = 886 // Simplified strategic action space (Basics v4)

func (e *Env) Step(actionID int, includeMetadata bool, denseRewardScale float64) (reward float64, done bool) {
	e.maskDirty = true // Any state change invalidates the cached mask

	// If the game is already over, return terminal signal immediately.
	if e.State.GameOver {
		return 0.0, true
	}

	if actionID == -1 {
		return 0, true
	}

	EnsureActionRegistry(e.State.Board)

	// In Version 3.0 (Basics), actionID is purely the index in ActionRegistry.
	// We use a heuristic to choose the best card from the player's hand.
	baseActionID := actionID
	if baseActionID < 0 || baseActionID >= len(ActionRegistry) {
		return 0.0, true
	}
	action := ActionRegistry[baseActionID]
	active := e.State.Active
	player := e.State.Players[active]

	cardSlotIdx := e.ChooseBestCardForAction(player, action)
	if cardSlotIdx == -1 {
		if action.Type == ActionPass && len(player.Hand) == 0 {
			// Allow Pass with empty hand
		} else {
			fmt.Println("DEBUG: Invalid action-card pairing in Step! Action:", action.Type, "ActionIdx:", baseActionID)
			return 0.0, false // Invalid action-card pairing (should be masked)
		}
	}

	// Capture player state at beginning for reward (will use delta since last turn)
	prevAuditIndustries := e.LastAuditVPIndustries[active]
	prevAuditLinks := e.LastAuditVPLinks[active]
	prevIncome := e.LastIncome[active]

	// Reset Metadata for this step
	e.LastMetadata = StepMetadata{
		ActivePlayer: int(active),
		ActionName:   action.Name(e.State.Board),
		SlotIndex:    -1,
		CityID:       -1,
		RouteID:      -1,
		Era:          "Canal",
	}
	if e.State.Epoch == RailEra { e.LastMetadata.Era = "Rail" }

	switch action.Type {
	case ActionLoan:
		// Execute Loan
		currentInc := player.GetCurrentIncome()
		targetInc := currentInc - 3
		if targetInc < -10 {
			targetInc = -10
		}

		// Rule: Move to the highest space for that income level
		newLevel := 0
		for i := 99; i >= 0; i-- {
			if IncomeTrackMap[i] == targetInc {
				newLevel = i
				break
			}
		}

		player.Money += 30
		player.IncomeLevel = newLevel
		player.SyncIncome()

		// Reward: Small positive reward for action
		reward += 0.05 * denseRewardScale

		// Heuristic: Discard least flexible cards for loan (Industry > Location > Wild)
		// Spend chosen card
		actualCardIdx, ok := e.GetActualHandIndex(cardSlotIdx)
		if ok {
			card := e.State.Players[e.State.Active].Hand[actualCardIdx]
			e.LastMetadata.CardsSpent = []Card{card}
			e.State.ReturnCard(e.State.Active, actualCardIdx)
			
			// v2.4 Clarity: Small penalty for burning a LocationCard for a Loan
			if card.Type == LocationCard {
				reward -= 0.01 * denseRewardScale
			}
		}

	case ActionPass:
		player := e.State.Players[e.State.Active]
		if cardSlotIdx != -1 {
			actualCardIdx, ok := e.GetActualHandIndex(cardSlotIdx)
			if ok {
				card := player.Hand[actualCardIdx]
				e.LastMetadata.CardsSpent = []Card{card}
				e.State.ReturnCard(e.State.Active, actualCardIdx)
				
				// v2.4 Clarity: Small penalty for burning a LocationCard for a Pass
				if card.Type == LocationCard {
					reward -= 0.01 * denseRewardScale
				}
			}
		} else if len(player.Hand) == 0 {
			// Allowed to pass with empty hand, no card spent
			e.LastMetadata.CardsSpent = []Card{}
		}
		
		reward -= 0.1 * denseRewardScale

	case ActionScout:
		// Discard 3
		priority := []CardType{IndustryCard, LocationCard}
		e.LastMetadata.CardsSpent, _ = e.State.DiscardMultipleCardsFromPlayer(e.State.Active, 3, priority)

		// Gain Wilds
		player.Hand = append(player.Hand, Card{Type: WildLocationCard})
		player.Hand = append(player.Hand, Card{Type: WildIndustryCard})
		
		reward += 0.025 * denseRewardScale

	case ActionDevelop:
		count := 1
		if action.IndustryType2 != -1 {
			count = 2
		}

		// Calculate total cost and update market/board
		// Iron is distance-invariant, SourceIron handles priority automatically.
		cost := e.State.SourceIron(count, e.State.Active)
		player.Money -= cost
		e.LastMetadata.IronConsumed = count

		// Update Player Board
		player.DevelopToken(action.IndustryType)
		if count == 2 {
			player.DevelopToken(action.IndustryType2)
		}

		// Discard chosen card
		e.LastMetadata.CardsSpent, _ = e.GetCardAndBurn(cardSlotIdx)
		reward += 0.025 * denseRewardScale

	case ActionBuildIndustry:
		moneyBefore := player.Money
		// 1. Find Slot and Overbuild status
		// Version 4: We use the specific SlotIndex from the action choice.
		slotIdx := action.SlotIndex
		isOverbuild := e.State.IsOverbuild(action.CityID, slotIdx, action.IndustryType, e.State.Active)

		// Store diagnostic meta
		if includeMetadata {
			e.LastMetadata.CityID = int(action.CityID)
			e.LastMetadata.SlotIndex = slotIdx
			e.LastMetadata.IsOverbuild = isOverbuild
		}

		// 2. Identify and Burn the specific card from the Sorted Hand
		// We use CardIdx as provided by the model (referring to the Sorted view)
		actualCardIdx, ok := e.GetActualHandIndex(cardSlotIdx)
		if !ok {
			return 0.0, false
		}
		
		card := player.Hand[actualCardIdx]
		e.State.ReturnCard(e.State.Active, actualCardIdx)
		e.LastMetadata.CardsSpent = []Card{card}

		// 3. Stats and Costs
		currLvl := player.CurrentLevel[action.IndustryType]
		stat := IndustryCatalog[action.IndustryType][currLvl]
		
		coalCost := e.State.SourceCoal(action.CityID, stat.CostCoal, e.State.Active)
		ironCost := e.State.SourceIron(stat.CostIron, e.State.Active)
		
		e.LastMetadata.CoalConsumed = stat.CostCoal
		e.LastMetadata.IronConsumed = stat.CostIron
		
		player.Money -= (stat.CostMoney + coalCost + ironCost)

		// 4. Overbuild Cleanup
		if isOverbuild {
			// Remove the existing token from this slot
			for idx, tok := range e.State.Industries {
				if tok.CityID == action.CityID && tok.SlotIndex == slotIdx {
					e.State.Industries = append(e.State.Industries[:idx], e.State.Industries[idx+1:]...)
					break
				}
			}
		}

		// 5. Place industry on board
		token := &TokenState{
			Owner:     e.State.Active,
			CityID:    action.CityID,
			SlotIndex: slotIdx,
			Industry:  action.IndustryType,
			Level:     currLvl,
		}

		// Yield logic
		if action.IndustryType == CoalMineType || action.IndustryType == IronWorksType {
			yield := stat.YieldCanal
			if e.State.Epoch == RailEra { yield = stat.YieldRail }
			
			if action.IndustryType == CoalMineType {
				token.Coal = yield
				// Sell to Market immediately if connected to any merchant slot city
				connectedToMerchant := false
				for _, m := range e.State.Merchants {
					if e.State.Board.HasConnection(action.CityID, m.CityID) {
						connectedToMerchant = true
						break
					}
				}
				if connectedToMerchant {
					sold, earned := e.State.SellToMarket(Coal, token.Coal)
					player.Money += earned
					token.Coal -= sold
				}
			} else {
				token.Iron = yield
				// Iron always sells regardless of connection
				sold, earned := e.State.SellToMarket(Iron, token.Iron)
				player.Money += earned
				token.Iron -= sold
			}
		} else if action.IndustryType == BreweryType {
			token.Beer = 1
			if e.State.Epoch == RailEra { token.Beer = 2 }
		}

		e.State.Industries = append(e.State.Industries, token)

		// 6. Finalize Token Consumption
		player.ConsumeToken(action.IndustryType)

		// 7. Auto-Flip if yield exhausted (Market sell-off)
		if (action.IndustryType == CoalMineType && token.Coal == 0) || 
		   (action.IndustryType == IronWorksType && token.Iron == 0) {
			e.State.FlipIndustry(len(e.State.Industries) - 1)
		}

		// Reward: Stronger positive reward for building to encourage exploration
		reward += 0.15 * denseRewardScale
		moneySpent := moneyBefore - player.Money
		if moneySpent > 0 {
			reward += (float64(moneySpent) * 0.0) * denseRewardScale
		}

	case ActionBuildLink:
		route := &e.State.Board.Routes[action.RouteID]
		moneyBefore := player.Money
		wasConnectedA := e.State.IsMerchantConnected(route.CityA)
		wasConnectedB := e.State.IsMerchantConnected(route.CityB)

		if includeMetadata { e.LastMetadata.RouteID = action.RouteID }
		if e.State.Epoch == CanalEra {
			player.Money -= 3
		} else {
			player.Money -= 5
			cost := e.State.SourceCoal(route.CityA, 1, e.State.Active) 
			player.Money -= cost
			e.LastMetadata.CoalConsumed = 1
		}

		e.BuildRoute(action.RouteID, e.State.Active)
		e.LastMetadata.CardsSpent, _ = e.GetCardAndBurn(cardSlotIdx)

		// Link Reward: Grant immediate link VP for the points secured by this link right now
		p := e.State.Players[e.State.Active]
		valA := e.State.GetLinkValueForCity(route.CityA)
		valB := e.State.GetLinkValueForCity(route.CityB)
		p.VPAuditLinks += (valA + valB)
		// Reward: Stronger positive reward for building links to encourage network building
		reward += 0.15 * denseRewardScale

		// Reward: Removed for purification
		reward += 0.0 * denseRewardScale

		// Reward: Positive reward for connecting to a merchant!
		if (!wasConnectedA && e.State.IsMerchantConnected(route.CityA)) || 
		   (!wasConnectedB && e.State.IsMerchantConnected(route.CityB)) {
			reward += 0.15 * denseRewardScale
		}

		// Reward: Removed for purification
		moneySpent := moneyBefore - player.Money
		if moneySpent > 0 {
			reward += (float64(moneySpent) * 0.0) * denseRewardScale
		}

	case ActionBuildLinkDouble:
		r1ID := action.RouteID
		r2ID := action.RouteID2
		
		// 1. Pay Money
		player.Money -= 15

		// 2. Build Link 1 and Source Coal
		e.BuildRoute(r1ID, e.State.Active)
		c1cost := e.State.SourceCoal(e.State.Board.Routes[r1ID].CityA, 1, e.State.Active)

		// 3. Build Link 2 and Source Coal (Re-evaluating network)
		e.BuildRoute(r2ID, e.State.Active)
		c2cost := e.State.SourceCoal(e.State.Board.Routes[r2ID].CityA, 1, e.State.Active)

		// 4. Source Beer (Breweries ONLY for links)
		e.State.SourceBeer(e.State.Board.Routes[r2ID].CityA, e.State.Active, true, false, true)
		
		player.Money -= (c1cost + c2cost)
		e.LastMetadata.CoalConsumed = 2
		e.LastMetadata.BeerConsumed = 1

		// 5. Discard optimal card
		e.LastMetadata.CardsSpent, _ = e.GetCardAndBurn(cardSlotIdx)

		// Link Reward: Double Rail grants points for both routes immediately
		p := e.State.Players[e.State.Active]
		r1 := e.State.Board.Routes[r1ID]
		r2 := e.State.Board.Routes[r2ID]
		p.VPAuditLinks += (e.State.GetLinkValueForCity(r1.CityA) + e.State.GetLinkValueForCity(r1.CityB))
		p.VPAuditLinks += (e.State.GetLinkValueForCity(r2.CityA) + e.State.GetLinkValueForCity(r2.CityB))

		// Reward: Stronger positive reward for building double links
		reward += 0.10 * denseRewardScale



	case ActionSell:
		// Version 4: Greedy Sell Heuristic
		// Heuristic Priority: Merchant Beer > Other Player's Beer (Network) > Own Beer.
		// We sell EVERY possible reachable industry in one action.
		
		for {
			flippedAny := false
			for i, tok := range e.State.Industries {
				if tok.Owner != e.State.Active || tok.Flipped {
					continue
				}
				if tok.Industry != CottonType && tok.Industry != ManufacturedGoodsType && tok.Industry != PotteryType {
					continue
				}

				// Find best merchant and beer source
				bestMerchantIdx := -1
				beerSource := -1 // 0=Merchant, 1=Other, 2=Own

				for midx, m := range e.State.Merchants {
					if !e.State.CanSellToMerchant(tok, midx) {
						continue
					}
					
					// Priority 1: Merchant Beer
					if m.AvailableBeer > 0 {
						if bestMerchantIdx == -1 || beerSource > 0 {
							bestMerchantIdx = midx
							beerSource = 0
						}
					}
					
					// Priority 2: Other Player's Beer (Network)
					if beerSource > 1 || bestMerchantIdx == -1 {
						if e.State.HasNetworkBeer(tok.CityID, e.State.Active, false) {
							bestMerchantIdx = midx
							beerSource = 1
						}
					}

					// Priority 3: Own Beer
					if beerSource > 2 || bestMerchantIdx == -1 {
						if e.State.HasNetworkBeer(tok.CityID, e.State.Active, true) {
							bestMerchantIdx = midx
							beerSource = 2
						}
					}
				}

				if bestMerchantIdx != -1 {
					// Perform the sell
					m := e.State.Merchants[bestMerchantIdx]
					if beerSource == 0 {
						e.State.Merchants[bestMerchantIdx].AvailableBeer--
						if ev := player.EvaluateMerchantBeerBonus(e.State.Board.Cities[m.CityID].Name); ev != nil {
							if includeMetadata {
								e.LastMetadata.ScoreEvents = append(e.LastMetadata.ScoreEvents, *ev)
							}
						}
					} else {
						// SourceBeer handled internally by identifying network/owner beer
						e.State.SourceBeer(tok.CityID, e.State.Active, true, true, false)
					}
					e.State.FlipIndustry(i)
					flippedAny = true
				}
			}
			if !flippedAny {
				break
			}
		}

		e.LastMetadata.CardsSpent, _ = e.GetCardAndBurn(cardSlotIdx)
		reward += 0.025 * denseRewardScale
	}

	// ── Turn sequence ─────────────────────────────────────────────────────────
	e.State.ActionsRemaining--
	if e.State.ActionsRemaining <= 0 {
		prevActive := e.State.Active
		e.State.CurrentTurnIdx++

		if e.State.CurrentTurnIdx >= e.State.NumPlayers {
			// --- END OF ROUND ---
			e.State.ProcessTurnOrder()

			// Refill the last player's hand FIRST so IsEraOver() has accurate hand counts.
			e.State.RefillHand(prevActive)

			// Rule: Income is NOT collected in the final round of each era.
			if !e.State.IsEraOver() {
				e.State.ProcessIncome()
			}

			e.State.RoundCounter++
			e.State.CurrentTurnIdx = 0
			e.State.Active = e.State.TurnOrder[0]
			e.State.ActionsRemaining = 2

			if e.State.IsEraOver() {
				if e.State.Epoch == RailEra {
					// Game fully complete — score and mark done
					evs := e.State.ScoreEra(includeMetadata)
					if includeMetadata { e.LastMetadata.ScoreEvents = append(e.LastMetadata.ScoreEvents, evs...) }
					e.State.GameOver = true
				} else {
					evs := e.State.ScoreEra(includeMetadata) // Canal → Rail transition
					if includeMetadata { e.LastMetadata.ScoreEvents = append(e.LastMetadata.ScoreEvents, evs...) }
					e.State.EndEraTransition() // This already does ScoreEra(false) internally, so we swap it
				}
			}
		} else {
			// Next player in turn order
			e.State.Active = e.State.TurnOrder[e.State.CurrentTurnIdx]

			// Round 1 (Canal) exception: 1 action per player
			if e.State.RoundCounter == 1 && e.State.Epoch == CanalEra {
				e.State.ActionsRemaining = 1
			} else {
				e.State.ActionsRemaining = 2
			}

			e.State.RefillHand(prevActive)

			if e.State.IsEraOver() {
				if e.State.Epoch == RailEra {
					evs := e.State.ScoreEra(includeMetadata)
					if includeMetadata { e.LastMetadata.ScoreEvents = append(e.LastMetadata.ScoreEvents, evs...) }
					e.State.GameOver = true
				} else {
					evs := e.State.ScoreEra(includeMetadata)
					if includeMetadata { e.LastMetadata.ScoreEvents = append(e.LastMetadata.ScoreEvents, evs...) }
					e.State.EndEraTransition()
				}
			}
		}
	}

	// ── Compute step reward ───────────────────────────────────────────────────
	pAfter := e.State.Players[active]
	
	// Dense reward uses AuditVP (immediate boosts for flips and links)
	// v2.7: High-Contrast Scoring Signal (1 VP = 0.5 reward)
	vpDelta := (pAfter.VPAuditIndustries - prevAuditIndustries) + 
               (pAfter.VPAuditLinks - prevAuditLinks)
	incomeDelta := pAfter.IncomeLevel - prevIncome

	reward += (float64(vpDelta)*0.05 + float64(incomeDelta)*0.01) * denseRewardScale

	// Safety Clamp: Ensure total reward per step is in [-1.0, 1.0]
	if reward > 1.0 {
		reward = 1.0
	} else if reward < -1.0 {
		reward = -1.0
	}

	// Update tracking for next turn
	e.LastAuditVPIndustries[active] = pAfter.VPAuditIndustries
	e.LastAuditVPLinks[active] = pAfter.VPAuditLinks
	e.LastIncome[active] = pAfter.IncomeLevel

	done = e.State.GameOver
	if done {
		reward += e.ComputeTerminalReward(active)
	}


	if includeMetadata {
		e.LastMetadata.ProjectedVPs = make([]int, e.State.NumPlayers)
		for i := 0; i < e.State.NumPlayers; i++ {
			p := e.State.Players[i]
			e.LastMetadata.ProjectedVPs[i] = p.VPAuditIndustries + p.VPAuditLinks
		}
	}

	e.syncLastState()
	return reward, done
}

func (e *Env) BuildRoute(routeID int, owner PlayerId) {
	e.State.Board.Routes[routeID].IsBuilt = true
	e.State.Board.Routes[routeID].Owner = owner

	for _, subID := range e.State.Board.Routes[routeID].SubRoutes {
		e.State.Board.Routes[subID].IsBuilt = true
		e.State.Board.Routes[subID].Owner = owner
	}
}

func (e *Env) ComputeTerminalReward(active PlayerId) float64 {
	type pScore struct {
		id PlayerId
		vp int
		inc int
		money int
	}
	var scores []pScore
	for i := 0; i < e.State.NumPlayers; i++ {
		p := e.State.Players[i]
		scores = append(scores, pScore{PlayerId(i), p.VP, p.IncomeLevel, p.Money})
	}

	totalVP := 0
	for _, s := range scores {
		totalVP += s.vp
	}
	scoreScale := 0.1 + 0.9*float64(totalVP)/250.0
	if scoreScale > 1.0 {
		scoreScale = 1.0
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].vp != scores[j].vp { return scores[i].vp > scores[j].vp }
		if scores[i].inc != scores[j].inc { return scores[i].inc > scores[j].inc }
		return scores[i].money > scores[j].money
	})

	getRawPayout := func(rank int, total int) float64 {
		switch total {
		case 2:
			if rank == 0 { return 1.0 } else { return -1.0 }
		case 3:
			if rank == 0 { return 1.0 } else if rank == 1 { return 0.0 } else { return -1.0 }
		case 4:
			if rank == 0 { return 1.0 } else if rank == 1 { return 0.33 } else if rank == 2 { return -0.33 } else { return -1.0 }
		default:
			return 0
		}
	}

	var rankResults = make(map[PlayerId]float64)
	i := 0
	for i < e.State.NumPlayers {
		j := i + 1
		for j < e.State.NumPlayers && 
			scores[j].vp == scores[i].vp && 
			scores[j].inc == scores[i].inc && 
			scores[j].money == scores[i].money {
			j++
		}
		
		sumPayout := 0.0
		for k := i; k < j; k++ {
			sumPayout += getRawPayout(k, e.State.NumPlayers)
		}
		avgPayout := sumPayout / float64(j - i)
		
		for k := i; k < j; k++ {
			rankResults[scores[k].id] = avgPayout
		}
		i = j
	}
	
	return rankResults[active] * scoreScale
}

// ChooseBestCardForAction implements the V3.0 heuristic for automated card selection.
// Heuristic: IndustryCard(matching) > LocationCard(matching) > WildIndustry > WildLocation.
// For non-build actions: Any Non-Wild > Any Wild.
func (e *Env) ChooseBestCardForAction(p *PlayerState, action Action) int {
	// First, identify which cards are actually valid for this specific action
	var validSlots []int
	for slotIdx := 0; slotIdx < len(p.Hand); slotIdx++ {
		actualIdx, ok := e.GetActualHandIndex(slotIdx)
		if !ok { continue }
		if e.isValidActionWithCard(p, action, actualIdx) {
			validSlots = append(validSlots, slotIdx)
		}
	}

	if len(validSlots) == 0 {
		return -1
	}

		// Priority scoring: lower is better (Location > Industry > Wild)
		scoreSlot := func(slotIdx int) int {
			actualIdx, _ := e.GetActualHandIndex(slotIdx)
			card := p.Hand[actualIdx]
			
			if action.Type == ActionBuildIndustry {
				if card.Type == LocationCard && card.CityID == int(action.CityID) { return 0 }
				if card.Type == IndustryCard && card.Industry == action.IndustryType { return 1 }
				if card.Type == WildLocationCard { return 2 }
				if card.Type == WildIndustryCard { return 3 }
				return 4 // Should not be reachable for Build if valid
			}
			
			// For non-build actions: Location > Industry > Wild.
			if card.Type == LocationCard { return 0 }
			if card.Type == IndustryCard { return 1 }
			if card.Type == WildLocationCard { return 2 }
			if card.Type == WildIndustryCard { return 3 }
			return 4
		}

	bestSlot := validSlots[0]
	bestScore := scoreSlot(bestSlot)

	for i := 1; i < len(validSlots); i++ {
		s := scoreSlot(validSlots[i])
		if s < bestScore {
			bestScore = s
			bestSlot = validSlots[i]
		}
	}

	return bestSlot
}


