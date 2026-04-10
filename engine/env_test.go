package engine

import (
	"testing"
)

func TestComputeTerminalReward(t *testing.T) {
	env := &Env{
		State: &GameState{
			NumPlayers: 2,
			Players: []*PlayerState{
				{VP: 100, IncomeLevel: 10, Money: 50},
				{VP: 150, IncomeLevel: 5, Money: 20},
			},
		},
	}

	// Total VP = 250. Scale should be 1.0.
	// Player 1 (index 1) has 150 VP, wins.
	// Player 0 (index 0) has 100 VP, loses.

	reward1 := env.ComputeTerminalReward(1)
	if reward1 != 1.0 {
		t.Errorf("Expected reward 1.0 for winner at total VP 250, got %f", reward1)
	}

	reward0 := env.ComputeTerminalReward(0)
	if reward0 != -1.0 {
		t.Errorf("Expected reward -1.0 for loser at total VP 250, got %f", reward0)
	}

	// Test with low score
	env.State.Players[0].VP = 0
	env.State.Players[1].VP = 0
	// Total VP = 0. Scale should be 0.1.
	// Ties broken by Income. Player 0 has higher income (10 > 5).
	// So Player 0 wins.

	reward0_low := env.ComputeTerminalReward(0)
	if reward0_low != 0.1 {
		t.Errorf("Expected reward 0.1 for winner at total VP 0, got %f", reward0_low)
	}

	reward1_low := env.ComputeTerminalReward(1)
	if reward1_low != -0.1 {
		t.Errorf("Expected reward -0.1 for loser at total VP 0, got %f", reward1_low)
	}

	// Test with intermediate score
	env.State.Players[0].VP = 50
	env.State.Players[1].VP = 75
	// Total VP = 125. Scale should be 0.1 + 0.9 * 125 / 250 = 0.1 + 0.45 = 0.55.
	// Player 1 wins.

	reward1_mid := env.ComputeTerminalReward(1)
	expected_mid := 0.55
	if reward1_mid != expected_mid {
		t.Errorf("Expected reward %f for winner at total VP 125, got %f", expected_mid, reward1_mid)
	}
}
