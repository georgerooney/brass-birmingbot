package engine

import (
	"testing"
)

func TestIronMarketLeak(t *testing.T) {
	gs := &GameState{}
	gs.IronMarket = Market{
		Prices:        []int{1, 2, 3, 4, 5},
		Capacity:      []int{2, 2, 2, 2, 2},
		CurrentCubes:  []int{2, 2, 2, 2, 2},
		ExternalPrice: 6,
	}

	// Sourcing 2 iron from market
	cost := gs.SourceIron(2, 0)

	if cost != 2 {
		t.Errorf("Expected cost £2, got £%d", cost)
	}

	if gs.IronMarket.CurrentCubes[0] != 0 {
		t.Errorf("Expected 0 cubes in £1 slot, got %d", gs.IronMarket.CurrentCubes[0])
	}
}
