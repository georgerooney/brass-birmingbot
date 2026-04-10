package engine

import (
	"testing"
)

// TestObservationRouteEncoding verifies that routes are correctly encoded
// in the observation vector, and that the egocentric POV (chair rotation)
// logic works as expected for route owners.
func TestObservationRouteEncoding(t *testing.T) {
	env := NewEnv(2) // Create a 2-player game
	
	// Initially, no routes should be built.
	obs := BuildObservation(env.State)
	
	// Check that all routes are zeroed in the observation.
	for i := 0; i < ObsMaxRoutes; i++ {
		base := ObsMerchEnd + i*ObsRouteWidth
		if obs[base+0] != 0 {
			t.Errorf("Expected route %d to be unbuilt, but got %f", i, obs[base+0])
		}
	}
	
	// Build a route for player 0
	routeID := 0
	env.BuildRoute(routeID, 0)
	
	// Ensure active player is 0
	env.State.Active = 0
	
	obs = BuildObservation(env.State)
	
	// Check that route 0 is built and owned by player 0 (relative ID 0)
	base := ObsMerchEnd + routeID*ObsRouteWidth
	if obs[base+0] != 1 {
		t.Errorf("Expected route %d to be built, but got %f", routeID, obs[base+0])
	}
	if obs[base+1] != 1 {
		t.Errorf("Expected route %d owner to be relative player 0 (active player), but got %f", routeID, obs[base+1])
	}
	
	// Now change active player to 1 and check POV
	env.State.Active = 1
	obs = BuildObservation(env.State)
	
	// Route 0 is owned by player 0. Relative to player 1, player 0 is index (0 - 1 + 2) % 2 = 1.
	if obs[base+0] != 1 {
		t.Errorf("Expected route %d to be built, but got %f", routeID, obs[base+0])
	}
	if obs[base+2] != 1 { // base + 1 + relOwner, where relOwner = 1
		t.Errorf("Expected route %d owner to be relative player 1 (owner is player 0), but got %f", routeID, obs[base+2])
	}
}
