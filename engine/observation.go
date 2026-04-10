package engine

import "sort"

// ─── Observation Tensor ───────────────────────────────────────────────────────
//
// BuildObservation encodes the full game state as a flat float32 slice for the
// policy network. The layout is fixed-width and position-semantically consistent
// across all time steps, which is a hard requirement for gradient-based RL.
//
// Layout (total = ObsTotalSize):
//
//  [0 .. ObsMetaEnd)               Game metadata (era, round, active player, …)
//  [ObsMetaEnd .. ObsPlayerEnd)    One block per player (VP, money, income, …)
//  [ObsPlayerEnd .. ObsMarketEnd)  Coal market (7 slots) + Iron market (5 slots)
//  [ObsMarketEnd .. ObsMerchEnd)   Merchant slots (9 × 5 floats each)
//  [ObsMerchEnd .. ObsRouteEnd)    Route states (ObsMaxRoutes × ObsRouteWidth)
//  [ObsRouteEnd .. ObsSlotEnd)     Industry slot states (ObsMaxSlots × ObsSlotWidth)
//  [ObsSlotEnd .. ObsTotalSize)    Active player hand (ObsHandSize × ObsCardWidth)
//
// Values are normalized where possible so that all floats are ≈[0, 1].
// One-hot encodings use 0.0 / 1.0. Unknown / absent entries are all zeros.

// ─── Dimension constants ──────────────────────────────────────────────────────

const (
	ObsMaxPlayers = 4
	ObsMaxRoutes  = 48
	ObsMaxSlots   = 56
	ObsMaxCities  = 32
	ObsHandSize   = 8
	ObsMerchants  = 9

	// Per-element widths
	ObsRouteWidth  = 6  // built(1) + owner_ohe(4) + subRoute(1)
	ObsSlotWidth   = 24 // present(1) + owner_ohe(4) + industry_ohe(8) + level(1) + flipped(1) + res(3) + pad(6)
	ObsCityWidth   = 12 // connectivity(1) + market_access(3) + proximity_res(3) + network_pres(1) + pad(4)
	ObsCardWidth   = 24 // present(1) + type_ohe(4) + city_norm(1) + industry_ohe(8) + validity_mask(6) + expansion_val(1) + pad(3)
	ObsPlayerWidth = 28 // money_norm(1) + income_norm(1) + vp_norm(1) + spent_norm(1) + active(1) + tokens(7) + next_tiers(6) + wipe_vuln(6) + delta_income(1) + dev_cost(1) + audit_ind(1) + audit_link(1) + pad(1)
	ObsMerchWidth  = 5

	// Coal: 7 price slots; Iron: 5 price slots
	ObsCoalSlots = 7
	ObsIronSlots = 5

	// Metadata block: era(1) + round(1) + active(4) + actions(1) + wildcards(2) + cards_left(1) + prices(2) + global_supply(5) + demand(2) = 19
	ObsMetaWidth = 19

	// Derived totals
	ObsMetaEnd   = ObsMetaWidth
	ObsPlayerEnd = ObsMetaEnd + ObsMaxPlayers*ObsPlayerWidth
	ObsMarketEnd = ObsPlayerEnd + ObsCoalSlots + ObsIronSlots
	ObsMerchEnd  = ObsMarketEnd + ObsMerchants*ObsMerchWidth
	ObsRouteEnd  = ObsMerchEnd + ObsMaxRoutes*ObsRouteWidth
	ObsCityEnd   = ObsRouteEnd + ObsMaxCities*ObsCityWidth
	ObsSlotEnd   = ObsCityEnd + ObsMaxSlots*ObsSlotWidth
	ObsTotalSize = ObsSlotEnd + ObsHandSize*ObsCardWidth
)

// IndustryTypeToIdx maps IndustryType → [0,7] for one-hot encoding.
// Order matches the IndustryType enum declared in types.go.
func industryIdx(ind IndustryType) int {
	// CoalMineType=0, IronWorksType=1, CottonType=2, ManufacturedGoodsType=3,
	// PotteryType=4, BreweryType=5, (pad to 8 with zeros)
	return int(ind)
}

// ─── Builder ──────────────────────────────────────────────────────────────────

// BuildObservation returns a fixed-size float32 slice encoding gs from the
// perspective of the active player. The layout is documented above.
// The slice is freshly allocated; callers that need to avoid allocation should
// reuse a buffer and call FillObservation instead.
func BuildObservation(gs *GameState) []float32 {
	buf := make([]float32, ObsTotalSize)
	FillObservation(gs, buf)
	return buf
}

// FillObservation fills a caller-owned buffer of length >= ObsTotalSize.
// Panics if buf is too short.
func FillObservation(gs *GameState, buf []float32) {
	if len(buf) < ObsTotalSize {
		panic("observation buffer too small")
	}
	// Zero the buffer first (handles padding and absent entities).
	for i := range buf {
		buf[i] = 0
	}

	off := 0 // running offset into buf

	// ── Metadata ─────────────────────────────────────────────────────────────
	// era (0=canal, 1=rail)
	if gs.Epoch == RailEra {
		buf[off] = 1
	}
	off++

	// round (normalised; max ~12 rounds per era)
	buf[off] = clampNorm(float32(gs.RoundCounter), 20)
	off++

	// active player one-hot (ego-centric: me taking the turn is always bit 0)
	buf[off] = 1
	off += ObsMaxPlayers

	// actions remaining (0,1,2 → 0, 0.5, 1.0)
	buf[off] = clampNorm(float32(gs.ActionsRemaining), 2)
	off++

	// total cards left (normalised; full deck ≈ 60-80)
	buf[off] = clampNorm(float32(gs.TotalCardsLeft()), 80)
	off++

	// prices
	buf[off] = clampNorm(float32(gs.CoalMarket.GetCurrentPrice()), 8)
	off++
	buf[off] = clampNorm(float32(gs.IronMarket.GetCurrentPrice()), 6)
	off++

	// global supply
	buf[off] = clampNorm(float32(gs.TotalCoalOnBoard()), 20)
	off++
	buf[off] = clampNorm(float32(gs.TotalIronOnBoard()), 20)
	off++

	myBeer, oppBeer, mercBeer := gs.GetBeerSplit(gs.Active)
	buf[off] = clampNorm(float32(myBeer), 5)
	off++
	buf[off] = clampNorm(float32(oppBeer), 10)
	off++
	buf[off] = clampNorm(float32(mercBeer), 5)
	off++

	// demand
	coalDemand, ironDemand := gs.CalculateUncappedDemand(gs.Active)
	buf[off] = clampNorm(float32(coalDemand), 10)
	off++
	buf[off] = clampNorm(float32(ironDemand), 10)
	off++

	off = ObsMetaEnd // align to boundary

	// ── Player blocks ─────────────────────────────────────────────────────────
	// Ego-centric remapping: The active player block is always at index 0.
	// Remaining players are filled in turn-sequence order relative to the active player.
	for relPid := 0; relPid < ObsMaxPlayers; relPid++ {
		base := ObsMetaEnd + relPid*ObsPlayerWidth

		// Map relative block index back to absolute player ID
		absPid := (int(gs.Active) + relPid) % gs.NumPlayers

		if relPid >= gs.NumPlayers {
			// Absent player — leave zeros
			continue
		}
		p := gs.Players[absPid]

		buf[base+0] = clampNorm(float32(p.Money), 50)
		buf[base+1] = clampNorm(float32(p.IncomeLevel), 99)
		buf[base+2] = clampNorm(float32(p.VP), 100)
		buf[base+3] = clampNorm(float32(p.AmountSpent), 50)
		// Tokens left per industry type (5 standard types, normalised to max 5)
		for ind := IndustryType(0); ind <= BreweryType; ind++ {
			tokIdx := 5 + int(ind)
			buf[base+tokIdx] = clampNorm(float32(p.TokensLeft[ind]), 5)
		}

		// v2.0 Supply Side
		for ind := IndustryType(0); ind <= BreweryType; ind++ {
			buf[base+12+int(ind)] = clampNorm(float32(p.GetNextAvailableLevel(ind)), 8)
		}
		for ind := IndustryType(0); ind <= BreweryType; ind++ {
			if gs.HasWipeVulnerability(PlayerId(absPid), ind) {
				buf[base+18+int(ind)] = 1
			}
		}
		buf[base+24] = clampNorm(float32(p.GetStepsToNextIncomePound()), 4)
		buf[base+25] = clampNorm(float32(p.GetDevelopCostIron()), 4)
		buf[base+26] = clampNorm(float32(p.VPAuditIndustries), 100)
		buf[base+27] = clampNorm(float32(p.VPAuditLinks), 50)
	}
	off = ObsPlayerEnd

	// ── Markets ───────────────────────────────────────────────────────────────
	// Coal market (7 price slots, normalised by capacity=2)
	for i, cubes := range gs.CoalMarket.CurrentCubes {
		if i < ObsCoalSlots {
			buf[off+i] = clampNorm(float32(cubes), 2)
		}
	}
	off += ObsCoalSlots

	// Iron market (5 price slots)
	for i, cubes := range gs.IronMarket.CurrentCubes {
		if i < ObsIronSlots {
			buf[off+i] = clampNorm(float32(cubes), 2)
		}
	}
	off += ObsIronSlots

	off = ObsMarketEnd

	// ── Merchant slots ────────────────────────────────────────────────────────
	numCities := float32(len(gs.Board.Cities))
	for i, m := range gs.Merchants {
		if i >= ObsMerchants {
			break
		}
		base := ObsMarketEnd + i*ObsMerchWidth

		// Available beer
		if m.AvailableBeer > 0 {
			buf[base+0] = 1
		}
		// Accepted goods (cotton=1, manufactured=2, pottery=4)
		for _, acc := range m.Tile.Accepts {
			switch acc {
			case CottonType:
				buf[base+1] = 1
			case ManufacturedGoodsType:
				buf[base+2] = 1
			case PotteryType:
				buf[base+3] = 1
			}
		}
		// City normalised ID
		if numCities > 0 {
			buf[base+4] = float32(m.CityID) / numCities
		}
	}
	off = ObsMerchEnd
	for i := 0; i < len(gs.Board.Routes) && i < ObsMaxRoutes; i++ {
		r := gs.Board.Routes[i]
		base := ObsMerchEnd + i*ObsRouteWidth

		if gs.RouteBuilt[i] {
			buf[base+0] = 1 // built
			// Owner one-hot (mapped to relative PID)
			relOwner := (int(gs.RouteOwners[i]) - int(gs.Active) + gs.NumPlayers) % gs.NumPlayers
			if relOwner < ObsMaxPlayers {
				buf[base+1+relOwner] = 1
			}
		}
		if r.IsSubRoute {
			buf[base+5] = 1 // is sub-route
		}
	}
	off = ObsRouteEnd

	// ── Cities ────────────────────────────────────────────────────────────────
	for i := range gs.Board.Cities {
		if i >= ObsMaxCities {
			break
		}
		base := ObsRouteEnd + i*ObsCityWidth

		buf[base+0] = clampNorm(float32(len(gs.Board.Adj[CityID(i)])), 6)
		if gs.IsMerchantConnectedForIndustry(CityID(i), CottonType) {
			buf[base+1] = 1
		}
		if gs.IsMerchantConnectedForIndustry(CityID(i), ManufacturedGoodsType) {
			buf[base+2] = 1
		}
		if gs.IsMerchantConnectedForIndustry(CityID(i), BreweryType) {
			buf[base+3] = 1
		}

		distMyCoal, distAnyCoal, distAnyIron := gs.CalculateEconomicDistances(CityID(i), gs.Active)
		buf[base+4] = clampNorm(float32(distMyCoal), 10)
		buf[base+5] = clampNorm(float32(distAnyCoal), 10)
		buf[base+6] = clampNorm(float32(distAnyIron), 1) // Iron is binary availability

		if gs.IsInNetwork(gs.Active, CityID(i)) {
			buf[base+7] = 1
		}
	}
	off = ObsCityEnd

	// ── Industry build slots ──────────────────────────────────────────────────
	// Iterate over cities × slots in a stable order (same iteration every step).
	slotIdx := 0
	for cityIdx, city := range gs.Board.Cities {
		if slotIdx >= ObsMaxSlots {
			break
		}
		for slotInCity := 0; slotInCity < len(city.BuildSlots); slotInCity++ {
			if slotIdx >= ObsMaxSlots {
				break
			}
			base := ObsCityEnd + slotIdx*ObsSlotWidth

			tok := gs.GetTokenAtSlot(CityID(cityIdx), slotInCity)
			if tok != nil {
				buf[base+0] = 1 // present
				// Owner one-hot (mapped to relative PID)
				relOwner := (int(tok.Owner) - int(gs.Active) + gs.NumPlayers) % gs.NumPlayers
				if relOwner < ObsMaxPlayers {
					buf[base+1+relOwner] = 1
				}
				// Industry type one-hot (8 slots, 6 types used)
				indIdx := industryIdx(tok.Industry)
				if 5+indIdx < ObsSlotWidth {
					buf[base+5+indIdx] = 1
				}
				// Level (normalised; max ~8)
				buf[base+13] = clampNorm(float32(tok.Level), 8)
				// Flipped
				if tok.Flipped {
					buf[base+14] = 1
				}
				// Resources (normalised; coal/iron up to 6, beer up to 2)
				buf[base+15] = clampNorm(float32(tok.Coal), 6)
				buf[base+16] = clampNorm(float32(tok.Iron), 6)
				buf[base+17] = clampNorm(float32(tok.Beer), 2)
			}
			slotIdx++
		}
	}
	off = ObsSlotEnd

	// ── Active player hand ────────────────────────────────────────────────────
	hand := gs.Players[gs.Active].Hand
	sortedHand := make([]Card, len(hand))
	copy(sortedHand, hand)

	// Sort: Location > Industry > WildIndustry > WildLocation
	sort.SliceStable(sortedHand, func(i, j int) bool {
		ti, tj := sortedHand[i].Type, sortedHand[j].Type
		if ti != tj {
			return ti < tj // CardType enum order: Location=0, Industry=1, WildIndustry=2, WildLocation=3
		}
		return sortedHand[i].CityID < sortedHand[j].CityID
	})

	for i := 0; i < ObsHandSize; i++ {
		base := ObsSlotEnd + i*ObsCardWidth
		if i >= len(sortedHand) {
			continue
		}
		c := sortedHand[i]

		buf[base+0] = 1 // card present
		switch c.Type {
		case LocationCard:
			buf[base+1] = 1
		case IndustryCard:
			buf[base+2] = 1
		case WildIndustryCard:
			buf[base+3] = 1
		case WildLocationCard:
			buf[base+4] = 1
		}

		if c.Type == LocationCard || c.Type == WildLocationCard {
			buf[base+5] = float32(c.CityID) / numCities
		}
		if c.Type == IndustryCard || c.Type == WildIndustryCard {
			buf[base+6+industryIdx(c.Industry)] = 1
		}

		// Validity Mask (Build, Network, Develop, Sell, Loan, Pass)
		if gs.CanCardAction(c, ActionBuildIndustry) {
			buf[base+14] = 1
		}
		if gs.CanCardAction(c, ActionNetwork) {
			buf[base+15] = 1
		}
		if gs.CanCardAction(c, ActionDevelop) {
			buf[base+16] = 1
		}
		if gs.CanCardAction(c, ActionSell) {
			buf[base+17] = 1
		}
		if gs.CanCardAction(c, ActionLoan) {
			buf[base+18] = 1
		}
		buf[base+19] = 1 // Always can pass

		buf[base+20] = clampNorm(float32(gs.GetNetworkExpansionCount(c, gs.Active)), 10)
	}
	off = ObsTotalSize
	_ = off // align marker
}

// clampNorm normalises v ∈ [0, max] to [0, 1], clamping at the bounds.
func clampNorm(v, max float32) float32 {
	if max == 0 {
		return 0
	}
	n := v / max
	if n < 0 {
		return 0
	}
	if n > 1 {
		return 1
	}
	return n
}

// ─── Observation v2.0 Helpers ──────────────────────────────────────────────────

func (gs *GameState) GetBeerSplit(pID PlayerId) (my, opp, merc int) {
	for _, tok := range gs.Industries {
		if tok.Industry == BreweryType && tok.Beer > 0 {
			if tok.Owner == pID {
				my += tok.Beer
			} else {
				opp += tok.Beer
			}
		}
	}
	for _, m := range gs.Merchants {
		merc += m.AvailableBeer
	}
	return
}

func (gs *GameState) CalculateUncappedDemand(pID PlayerId) (coal, iron int) {
	p := gs.Players[pID]
	for ind := CottonType; ind <= BreweryType; ind++ {
		lvl := p.CurrentLevel[ind]
		if lvl <= IndustryMaxLevel[ind] {
			stat := IndustryCatalog[ind][lvl]
			iron += stat.CostIron
			coal += stat.CostCoal
		}
	}
	return
}

func (gs *GameState) HasWipeVulnerability(pID PlayerId, ind IndustryType) bool {
	if gs.Epoch != CanalEra {
		return false
	}
	for _, tok := range gs.Industries {
		if tok.Owner == pID && tok.Industry == ind && tok.Level == 1 {
			return true
		}
	}
	return false
}

func (gs *GameState) IsMerchantConnectedForIndustry(city CityID, ind IndustryType) bool {
	gs.bfsGen++
	gs.bfsQueue = gs.bfsQueue[:0]
	gs.bfsQueue = append(gs.bfsQueue, city)
	gs.bfsVisited[city] = gs.bfsGen

	head := 0
	for head < len(gs.bfsQueue) {
		curr := gs.bfsQueue[head]
		head++

		// Check if this city is a merchant that accepts the industry
		for _, m := range gs.Merchants {
			if m.CityID == curr {
				for _, acc := range m.Tile.Accepts {
					if acc == ind {
						return true
					}
				}
			}
		}

		for _, routeID := range gs.Board.Adj[curr] {
			route := gs.Board.Routes[routeID]
			if !gs.RouteBuilt[routeID] {
				continue
			}
			next := route.CityA
			if next == curr {
				next = route.CityB
			}
			if gs.bfsVisited[next] != gs.bfsGen {
				gs.bfsVisited[next] = gs.bfsGen
				gs.bfsQueue = append(gs.bfsQueue, next)
			}
		}
	}
	return false
}

func (gs *GameState) CalculateEconomicDistances(city CityID, pID PlayerId) (myCoal, anyCoal, anyIron int) {
	// 1. Coal Distances
	gs.bfsGen++
	gs.bfsQueue = gs.bfsQueue[:0]
	type step struct {
		city CityID
		dist int
	}
	queue := []step{{city, 0}}
	gs.bfsVisited[city] = gs.bfsGen

	myCoal = 10
	anyCoal = 10

	head := 0
	for head < len(queue) {
		curr := queue[head]
		head++

		if curr.dist >= 10 {
			continue
		}

		for _, tok := range gs.Industries {
			if tok.CityID == curr.city && tok.Industry == CoalMineType && tok.Coal > 0 {
				if tok.Owner == pID && curr.dist < myCoal {
					myCoal = curr.dist
				}
				if curr.dist < anyCoal {
					anyCoal = curr.dist
				}
			}
		}

		if myCoal != 10 && anyCoal != 10 {
			break
		}

		for _, routeID := range gs.Board.Adj[curr.city] {
			r := gs.Board.Routes[routeID]
			if !gs.RouteBuilt[routeID] {
				continue
			}
			next := r.CityA
			if next == curr.city {
				next = r.CityB
			}
			if gs.bfsVisited[next] != gs.bfsGen {
				gs.bfsVisited[next] = gs.bfsGen
				queue = append(queue, step{next, curr.dist + 1})
			}
		}
	}

	// 2. Iron Availability (Binary)
	for _, tok := range gs.Industries {
		if tok.Industry == IronWorksType && tok.Iron > 0 {
			anyIron = 1
			break
		}
	}

	return
}

func (gs *GameState) CanCardAction(c Card, actionType ActionType) bool {
	p := gs.Players[gs.Active]
	switch actionType {
	case ActionBuildIndustry:
		// Simplified validation for observation validity mask
		return gs.CanBurnCardForBuild(CityID(c.CityID), c.Industry, gs.Active)
	case ActionNetwork:
		return len(p.Hand) >= 1 // Any card can be used for a link
	case ActionDevelop:
		return len(p.Hand) >= 1
	case ActionSell:
		return len(p.Hand) >= 1
	case ActionLoan:
		return len(p.Hand) >= 1
	}
	return false
}

func (gs *GameState) GetNetworkExpansionCount(c Card, pID PlayerId) int {
	if c.Type != IndustryCard {
		return 0
	}
	count := 0
	// Count how many cities in our current network (or adjunct) accept this industry
	for i, city := range gs.Board.Cities {
		if gs.IsInNetwork(pID, CityID(i)) {
			// Check if city has an available slot for this industry
			allowed := false
			for _, slot := range city.BuildSlots {
				for _, ind := range slot {
					if ind == c.Industry {
						allowed = true
						break
					}
				}
			}
			if allowed {
				// Check if slot is empty or could be overbuilt
				empty := true
				for j := range city.BuildSlots {
					if gs.GetTokenAtSlot(CityID(i), j) != nil {
						empty = false
					}
				}
				if empty {
					count++
				}
			}
		}
	}
	return count
}
