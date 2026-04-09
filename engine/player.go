package engine

// ─── PlayerState methods ──────────────────────────────────────────────────────

// GetCurrentIncome evaluates exactly what the player earns at the start of their round.
func (p *PlayerState) GetCurrentIncome() int {
	if p.IncomeLevel > 99 {
		return IncomeTrackMap[99]
	}
	if p.IncomeLevel < 0 {
		return IncomeTrackMap[0]
	}
	return IncomeTrackMap[p.IncomeLevel]
}

// SyncIncome updates the ready-to-display Income field based on current track position.
func (p *PlayerState) SyncIncome() {
	p.Income = p.GetCurrentIncome()
}

// HasWildCard checks if the player holds either a Wild Location or Wild Industry card.
func (p *PlayerState) HasWildCard() bool {
	for _, c := range p.Hand {
		if c.Type == WildLocationCard || c.Type == WildIndustryCard {
			return true
		}
	}
	return false
}

// ConsumeToken evaluates attempting to pull a token from the player board.
// Returns false if entirely sold out.
// Handles auto-advancing to the next level when a tier depletes.
func (p *PlayerState) ConsumeToken(ind IndustryType) bool {
	if p.CurrentLevel[ind] > IndustryMaxLevel[ind] {
		return false // No tokens left for this industry anywhere on board
	}

	p.TokensLeft[ind] -= 1

	if p.TokensLeft[ind] == 0 {
		p.CurrentLevel[ind] += 1
		nextLvl := p.CurrentLevel[ind]
		if nextLvl <= IndustryMaxLevel[ind] {
			p.TokensLeft[ind] = IndustryCatalog[ind][nextLvl].Count
		}
	}
	return true
}

// DevelopToken removes the current industry tile from the board if it is developable.
// Returns false if not developable or no tokens left.
func (p *PlayerState) DevelopToken(ind IndustryType) bool {
	currLvl := p.CurrentLevel[ind]
	if currLvl > IndustryMaxLevel[ind] {
		return false
	}
	stat := IndustryCatalog[ind][currLvl]
	if !stat.IsDevelopable {
		return false
	}
	return p.ConsumeToken(ind)
}

// isDevelopableAtCurrentLevel checks if the current token of this type can be developed.
func (p *PlayerState) isDevelopableAtCurrentLevel(ind IndustryType) bool {
	currLvl := p.CurrentLevel[ind]
	if currLvl > IndustryMaxLevel[ind] {
		return false
	}
	return IndustryCatalog[ind][currLvl].IsDevelopable
}

// canDevelopTwoOfSame checks if there are at least two tokens left and both are developable in sequence.
func (p *PlayerState) canDevelopTwoOfSame(ind IndustryType) bool {
	currLvl := p.CurrentLevel[ind]
	tokens := p.TokensLeft[ind]

	// Case 1: More than 1 token in current tier.
	if tokens >= 2 {
		return IndustryCatalog[ind][currLvl].IsDevelopable
	}

	// Case 2: Only 1 token in current tier — need both this and next to be developable.
	if tokens == 1 {
		if !IndustryCatalog[ind][currLvl].IsDevelopable {
			return false
		}
		nextLvl := currLvl + 1
		if nextLvl > IndustryMaxLevel[ind] {
			return false
		}
		return IndustryCatalog[ind][nextLvl].IsDevelopable
	}

	return false
}

// GetNextAvailableLevel returns the level of the next tile to be built.
func (p *PlayerState) GetNextAvailableLevel(ind IndustryType) int {
	if p.CurrentLevel[ind] > IndustryMaxLevel[ind] {
		return 0
	}
	return p.CurrentLevel[ind]
}

// GetStepsToNextIncomePound calculates distance to the next increment in actual cash income.
func (p *PlayerState) GetStepsToNextIncomePound() int {
	curr := p.GetCurrentIncome()
	for i := p.IncomeLevel + 1; i < 100; i++ {
		if IncomeTrackMap[i] > curr {
			return i - p.IncomeLevel
		}
	}
	return 0
}

// GetDevelopCostIron calculates the iron cost for the next available tier if it was developed.
func (p *PlayerState) GetDevelopCostIron() int {
	// Note: Logic needs to handle multiple industries or current priority.
	// For simplicity in observation, we look at the next available industry tile's development cost if it exists.
	// We'll iterate through all industries and pick the min non-zero dev cost as a heuristic.
	minIron := 9
	found := false
	for ind := CottonType; ind <= BreweryType; ind++ {
		lvl := p.CurrentLevel[ind]
		if lvl <= IndustryMaxLevel[ind] && IndustryCatalog[ind][lvl].IsDevelopable {
			cost := IndustryCatalog[ind][lvl].CostIron
			if cost < minIron {
				minIron = cost
				found = true
			}
		}
	}
	if !found {
		return 0
	}
	return minIron
}
