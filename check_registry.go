package main

import (
	"fmt"
	"brass_engine/engine"
)

func main() {
	mg := engine.NewMapGraph()
	engine.BuildActionRegistry(mg)
	
	fmt.Printf("Total Action Registry Size: %d\n", len(engine.ActionRegistry))
	fmt.Printf("Total Routes: %d\n", len(mg.Routes))
	fmt.Printf("Total Cities: %d\n", len(mg.Cities))

	counts := make(map[engine.ActionType]int)
	for _, a := range engine.ActionRegistry {
		counts[a.Type]++
	}

	fmt.Println("Breakdown:")
	fmt.Printf("  Build Industry: %d\n", counts[engine.ActionBuildIndustry])
	fmt.Printf("  Build Link:     %d\n", counts[engine.ActionBuildLink])
	fmt.Printf("  Build Double:   %d\n", counts[engine.ActionBuildLinkDouble])
	fmt.Printf("  Develop:        %d\n", counts[engine.ActionDevelop])
	fmt.Printf("  Sell:           %d\n", counts[engine.ActionSell])
	fmt.Printf("  Loan:           %d\n", counts[engine.ActionLoan])
	fmt.Printf("  Scout:          %d\n", counts[engine.ActionScout])
	fmt.Printf("  Pass:           %d\n", counts[engine.ActionPass])
}
