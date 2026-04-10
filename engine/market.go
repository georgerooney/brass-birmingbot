package engine

// Market holds the resource market state for coal or iron.
type Market struct {
	Resource      Resource `json:"resource"`
	MaxPrice      int      `json:"max_price"`
	Prices        []int    `json:"prices"`        // [0] = £1, [1] = £2...
	Capacity      []int    `json:"capacity"`      // Max cubes at this price
	CurrentCubes  []int    `json:"current_cubes"` // Current cubes at this price
	ExternalPrice int      `json:"external_price"`
}

// BuyFromMarket consumes resources from the market using least-expensive-first logic.
func (m *Market) GetCurrentPrice() int {
	for i, cubes := range m.CurrentCubes {
		if cubes > 0 {
			return m.Prices[i]
		}
	}
	return m.ExternalPrice
}

func (m *Market) BuyFromMarket(count int) (cost int) {
	totalCost := 0
	toBuy := count

	for i := 0; i < len(m.Prices); i++ {
		if toBuy <= 0 {
			break
		}
		available := m.CurrentCubes[i]
		taking := toBuy
		if taking > available {
			taking = available
		}
		totalCost += taking * m.Prices[i]
		m.CurrentCubes[i] -= taking
		toBuy -= taking
	}

	// External market for remainder
	if toBuy > 0 {
		totalCost += toBuy * m.ExternalPrice
	}

	return totalCost
}

// PredictCost returns the cost of buying N cubes without modifying state.
func (m *Market) PredictCost(count int) int {
	totalCost := 0
	toBuy := count

	tempCubes := make([]int, len(m.CurrentCubes))
	copy(tempCubes, m.CurrentCubes)

	for i := 0; i < len(m.Prices); i++ {
		if toBuy <= 0 {
			break
		}
		available := tempCubes[i]
		taking := toBuy
		if taking > available {
			taking = available
		}
		totalCost += taking * m.Prices[i]
		toBuy -= taking
	}
	if toBuy > 0 {
		totalCost += toBuy * m.ExternalPrice
	}
	return totalCost
}

// PredictNextCubeCost returns the cost of the next single cube from a temp cube slice.
func (m *Market) PredictNextCubeCost(tempCubes []int) int {
	for i := 0; i < len(m.Prices); i++ {
		if tempCubes[i] > 0 {
			return m.Prices[i]
		}
	}
	return m.ExternalPrice
}

// SellToMarket fills empty market slots using most-expensive-first logic.
func (m *Market) SellToMarket(count int) (moneyGained int, leftover int) {
	toSell := count
	totalMoney := 0

	for i := len(m.Prices) - 1; i >= 0; i-- {
		if toSell <= 0 {
			break
		}
		emptySlots := m.Capacity[i] - m.CurrentCubes[i]
		filling := toSell
		if filling > emptySlots {
			filling = emptySlots
		}
		totalMoney += filling * m.Prices[i]
		m.CurrentCubes[i] += filling
		toSell -= filling
	}

	return totalMoney, toSell
}

// ─── GameState market accessors ───────────────────────────────────────────────

// SellToMarket moves resources from a builder's tile to the market and returns (sold, money).
func (gs *GameState) SellToMarket(res Resource, count int) (int, int) {
	var m *Market
	if res == Coal {
		m = &gs.CoalMarket
	} else {
		m = &gs.IronMarket
	}

	totalMoney := 0
	cubesSold := 0

	for i := len(m.Prices) - 1; i >= 0 && cubesSold < count; i-- {
		available := m.Capacity[i] - m.CurrentCubes[i]
		toSell := count - cubesSold
		if toSell > available {
			toSell = available
		}
		m.CurrentCubes[i] += toSell
		totalMoney += toSell * m.Prices[i]
		cubesSold += toSell
	}

	return cubesSold, totalMoney
}

// IsCoalMarketEmpty checks if there is any coal in the market to buy.
func (gs *GameState) IsCoalMarketEmpty() bool {
	for _, count := range gs.CoalMarket.CurrentCubes {
		if count > 0 {
			return false
		}
	}
	return true
}

// IsIronMarketEmpty checks if there is any iron in the market to buy.
func (gs *GameState) IsIronMarketEmpty() bool {
	for _, count := range gs.IronMarket.CurrentCubes {
		if count > 0 {
			return false
		}
	}
	return true
}

// IsMarketFull checks if a market can accept any more cubes.
func (gs *GameState) IsMarketFull(res Resource) bool {
	var m *Market
	if res == Coal {
		m = &gs.CoalMarket
	} else {
		m = &gs.IronMarket
	}
	for i, count := range m.CurrentCubes {
		if count < m.Capacity[i] {
			return false
		}
	}
	return true
}

// consumeOneCubeFromTemp removes one cube from a temporary cube-count slice (cheapest first).
func (gs *GameState) consumeOneCubeFromTemp(tempCubes []int) {
	for i := 0; i < len(tempCubes); i++ {
		if tempCubes[i] > 0 {
			tempCubes[i]--
			return
		}
	}
}
