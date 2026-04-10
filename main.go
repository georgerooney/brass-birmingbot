package main

import (
	"fmt"
	"brass_engine/engine"
)

func main() {
	fmt.Println("=== BRASS BIRMINGHAM: REWARD & TIE-BREAK VERIFICATION ===")

	numPlayers := 2
	env := engine.NewEnv(numPlayers)
	gs := env.State
	
	engine.EnsureActionRegistry(gs.Board)
	actionPassID := -1
	for i, a := range engine.ActionRegistry {
		if a.Type == engine.ActionPass { actionPassID = i; break }
	}

	// 1. REWARD PERSISTENCE TEST
	// Player 0 is active.
	// We'll manually give Player 1 some VP and see if they get it when they become active.
	fmt.Println("\n[1] Reward Persistence Test")
	gs.Players[1].VP = 50
	
	// P0 Takes a turn
	reward0, _ := env.Step(actionPassID, false, 1.0)
	fmt.Printf("P0 Turn 1 Reward: %.2f (Expected: 0.00)\n", reward0)
	
	// P1 Takes a turn (should now receive the 50 VP reward!)
	reward1, _ := env.Step(actionPassID, false, 1.0)
	fmt.Printf("P1 Turn 1 Reward: %.2f (Expected: 0.50)\n", reward1)

	// 2. TIE-BREAK VERIFICATION
	fmt.Println("\n[2] Tie-Break Verification (VP > Income > Money)")
	env.Reset()
	gs = env.State
	
	// Force conditions so that P0's action TRIGGERS game over
	gs.Epoch = engine.RailEra
	gs.RoundCounter = 20 // High enough to be near end
	gs.ActionsRemaining = 1
	gs.CurrentTurnIdx = gs.NumPlayers - 1 // Last player in turn order
	
	// Force a tie in VP but P1 has higher income
	// P1 is NOT active, so the 'active' player (P0) will be the one receiving the reward
	gs.Active = engine.PlayerId(0)
	gs.Players[0].VP = 100
	gs.Players[0].IncomeLevel = 20 
	gs.Players[0].Money = 10
	
	gs.Players[1].VP = 100
	gs.Players[1].IncomeLevel = 30 // Higher income
	gs.Players[1].Money = 10
	
	// IsEraOver checks if deck is empty and hands are empty
	gs.Deck = []engine.Card{}
	gs.Players[0].Hand = []engine.Card{}
	gs.Players[1].Hand = []engine.Card{}
	
	// Step P0's last move. This will trigger the Round End -> Era End -> Game Over sequence.
	rewardT0, _ := env.Step(actionPassID, false, 1.0)
	// P1 is winner (+1.0), P0 is loser (-1.0)
	fmt.Printf("P0 Terminal Reward (Income Tie-break): %.2f (Expected: 0.10)\n", rewardT0)

	// Test Money Tie-break
	env.Reset()
	gs = env.State
	gs.Epoch = engine.RailEra
	gs.ActionsRemaining = 1
	gs.CurrentTurnIdx = gs.NumPlayers - 1
	gs.Active = engine.PlayerId(0)
	gs.Deck = []engine.Card{}
	gs.Players[0].Hand = []engine.Card{}
	gs.Players[1].Hand = []engine.Card{}

	gs.Players[0].VP = 100
	gs.Players[0].IncomeLevel = 30
	gs.Players[0].Money = 50 // More money
	
	gs.Players[1].VP = 100
	gs.Players[1].IncomeLevel = 30
	gs.Players[1].Money = 10
	
	rewardT0_money, _ := env.Step(actionPassID, false, 1.0)
	fmt.Printf("P0 Terminal Reward (Money Tie-break): %.2f (Expected: 2.20)\n", rewardT0_money)

	// Test Draw Tie-break
	env.Reset()
	gs = env.State
	gs.Epoch = engine.RailEra
	gs.ActionsRemaining = 1
	gs.CurrentTurnIdx = gs.NumPlayers - 1
	gs.Active = engine.PlayerId(0)
	gs.Deck = []engine.Card{}
	gs.Players[0].Hand = []engine.Card{}
	gs.Players[1].Hand = []engine.Card{}

	gs.Players[0].VP = 100
	gs.Players[0].IncomeLevel = 30
	gs.Players[0].Money = 50 
	
	gs.Players[1].VP = 100
	gs.Players[1].IncomeLevel = 30
	gs.Players[1].Money = 50 // Exactly tied
	
	rewardT0_draw, _ := env.Step(actionPassID, false, 1.0)
	fmt.Printf("P0 Terminal Reward (Draw): %.2f (Expected: 1.20)\n", rewardT0_draw)

	fmt.Println("\n=== VERIFICATION COMPLETE ===")
}
