package engine

// CoalSource represents a potential board coal source found during BFS.
type CoalSource struct {
	CityID   CityID
	TokenIdx int // Index in gs.Industries
	Distance int
	Owner    PlayerId
}

// ─── Zero-allocation BFS primitives ──────────────────────────────────────────

// HasConnectionFast is a zero-allocation BFS connectivity check.
// Uses the GameState's pre-allocated scratch (generation counter + bfsQueue).
// Not goroutine-safe — each Env/GameState must call this on its own instance only.
func (gs *GameState) HasConnectionFast(start, target CityID) bool {
	if start == target {
		return true
	}

	gs.bfsGen++
	gs.bfsQueue = gs.bfsQueue[:0]
	gs.bfsQueue = append(gs.bfsQueue, start)
	gs.bfsVisited[start] = gs.bfsGen

	head := 0
	for head < len(gs.bfsQueue) {
		curr := gs.bfsQueue[head]
		head++

		for _, routeID := range gs.Board.Adj[curr] {
			route := gs.Board.Routes[routeID]
			if !route.IsBuilt {
				continue
			}
			next := route.CityA
			if next == curr {
				next = route.CityB
			}
			if next == target {
				return true
			}
			if gs.bfsVisited[next] != gs.bfsGen {
				gs.bfsVisited[next] = gs.bfsGen
				gs.bfsQueue = append(gs.bfsQueue, next)
			}
		}
	}
	return false
}

// IsMerchantConnected checks if any Merchant city is reachable from start via built routes.
// Uses the generation-counter BFS scratch to avoid allocating a map per call.
func (gs *GameState) IsMerchantConnected(start CityID) bool {
	gs.bfsGen++
	gs.bfsQueue = gs.bfsQueue[:0]
	gs.bfsQueue = append(gs.bfsQueue, start)
	gs.bfsVisited[start] = gs.bfsGen

	head := 0
	for head < len(gs.bfsQueue) {
		curr := gs.bfsQueue[head]
		head++

		if gs.Board.Cities[curr].Type == "Merchant" {
			return true
		}

		for _, routeID := range gs.Board.Adj[curr] {
			route := gs.Board.Routes[routeID]
			if !route.IsBuilt {
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

// ─── Network membership ───────────────────────────────────────────────────────

// IsInNetwork checks if a player has a physical presence inside a specific routing network.
func (gs *GameState) IsInNetwork(playerID PlayerId, city CityID) bool {
	// Rule 1: Do they own an industry here?
	for _, ind := range gs.Industries {
		if ind.CityID == city && ind.Owner == playerID {
			return true
		}
	}
	// Rule 2: Do they have a built route adjacent to here?
	for _, routeID := range gs.Board.Adj[city] {
		r := gs.Board.Routes[routeID]
		if r.IsBuilt && r.Owner == playerID {
			return true
		}
	}
	return false
}

// IsFirstBuild checks if the player has no industries and no routes built.
func (gs *GameState) IsFirstBuild(playerID PlayerId) bool {
	for _, ind := range gs.Industries {
		if ind.Owner == playerID {
			return false
		}
	}
	for _, r := range gs.Board.Routes {
		if r.IsBuilt && r.Owner == playerID {
			return false
		}
	}
	return true
}

// IsAdjacentToNetwork checks if a route is adjacent to any player presence.
// Special Rule: If player has NO presence on board, ANY route is valid (first-build exception).
func (gs *GameState) IsAdjacentToNetwork(routeID int, playerID PlayerId) bool {
	route := gs.Board.Routes[routeID]

	hasPresence := false
	for _, tok := range gs.Industries {
		if tok.Owner == playerID {
			hasPresence = true
			break
		}
	}
	if !hasPresence {
		for _, r := range gs.Board.Routes {
			if r.IsBuilt && r.Owner == playerID {
				hasPresence = true
				break
			}
		}
	}
	if !hasPresence {
		return true // First build exception
	}

	return gs.IsInNetwork(playerID, route.CityA) || gs.IsInNetwork(playerID, route.CityB)
}

// ─── Coal BFS ─────────────────────────────────────────────────────────────────

// findBestCoalSource BFS-searches for the nearest coal mine(s) and applies
// proximity → own-mine → lowest-VP-opponent priority rules.
func (gs *GameState) findBestCoalSource(start CityID, activePlayer PlayerId) *CoalSource {
	type step struct {
		city CityID
		dist int
	}
	// Use generation-counter scratch for O(1) visited clear; maintain local dist-aware queue.
	gs.bfsGen++
	gs.bfsVisited[start] = gs.bfsGen

	queue := []step{{start, 0}}
	var candidates []CoalSource
	head := 0
	for head < len(queue) {
		curr := queue[head]
		head++

		for idx, tok := range gs.Industries {
			if tok.CityID == curr.city && tok.Industry == CoalMineType && tok.Coal > 0 {
				candidates = append(candidates, CoalSource{
					CityID:   curr.city,
					TokenIdx: idx,
					Distance: curr.dist,
					Owner:    tok.Owner,
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

// CanBuildDoubleRail performs a sequence of checks for the £15 double rail action.
// IMPORTANT: This function is non-mutating and goroutine-safe. It does NOT write to board state.
// Adjacency for R2 is checked by treating R1's endpoints as "in network" without physically building R1.
// Coal/beer prediction for R2 is conservative (uses current network, not post-R1 network) — this
// may produce rare false negatives (masking a valid double-rail) but never false positives.
func (gs *GameState) CanBuildDoubleRail(r1, r2 int, playerID PlayerId) bool {
	p := gs.Players[playerID]
	r1Route := gs.Board.Routes[r1]
	r2Route := gs.Board.Routes[r2]

	if !gs.IsAdjacentToNetwork(r1, playerID) {
		return false
	}

	// Adjacency check for R2: must connect to (existing network ∪ {r1.CityA, r1.CityB}).
	inNetworkWithR1 := func(city CityID) bool {
		return gs.IsInNetwork(playerID, city) ||
			city == r1Route.CityA || city == r1Route.CityB
	}
	if !inNetworkWithR1(r2Route.CityA) && !inNetworkWithR1(r2Route.CityB) {
		return false
	}

	// Find which end of r1 is in network
	startCity := r1Route.CityA
	if !gs.IsInNetwork(playerID, startCity) {
		startCity = r1Route.CityB
	}

	// Check if we can source 2 coal from startCity
	cost, possible := gs.PredictCoalCost(startCity, 2, playerID)
	if !possible {
		return false
	}
	
	// Check if we can source 1 beer from startCity
	if !gs.PredictBeerPossible(startCity, playerID, true, false, true) {
		return false
	}
	
	if p.Money < (15 + cost) {
		return false
	}
	return true
}
