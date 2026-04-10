package engine

import "sort"

// ReturnCard removes the card at cardIdx from the player's Hand and adds it to the Discard pile.
func (gs *GameState) ReturnCard(pID PlayerId, cardIdx int) {
	p := gs.Players[pID]
	if cardIdx < 0 || cardIdx >= len(p.Hand) {
		return
	}
	card := p.Hand[cardIdx]
	gs.Discard = append(gs.Discard, card)
	
	// Remove from slice
	p.Hand = append(p.Hand[:cardIdx], p.Hand[cardIdx+1:]...)
}

// DiscardMultipleCardsFromPlayer removes n cards from the player's hand based on priority.
func (gs *GameState) DiscardMultipleCardsFromPlayer(pID PlayerId, count int, priority []CardType) ([]Card, bool) {
	p := gs.Players[pID]
	if len(p.Hand) < count {
		return nil, false
	}
	
	var discarded []Card
	for i := 0; i < count; i++ {
		// Greedy priority selection: try each priority type in order
		targetIdx := -1
		for _, pType := range priority {
			for idx, c := range p.Hand {
				if c.Type == pType {
					targetIdx = idx
					break
				}
			}
			if targetIdx != -1 { break }
		}
		
		// Fallback to any card if priority not found
		if targetIdx == -1 {
			targetIdx = 0
		}
		
		discarded = append(discarded, p.Hand[targetIdx])
		gs.ReturnCard(pID, targetIdx)
	}
	return discarded, true
}

// CanCardBeUsedForBuild checks if a SPECIFIC card index in hand can build the target.
func (gs *GameState) CanCardBeUsedForBuild(cityID CityID, ind IndustryType, pID PlayerId, cardIdx int) bool {
	p := gs.Players[pID]
	if cardIdx < 0 || cardIdx >= len(p.Hand) {
		return false
	}
	card := p.Hand[cardIdx]
	
	switch card.Type {
	case LocationCard:
		// Location Card: Build any industry in the city shown.
		return card.CityID == int(cityID)
	case IndustryCard:
		// Industry Card: Build that industry in a city you are CONNECTED to.
		// (Special first-build rule is handled by IsInNetwork or env caller)
		return card.Industry == ind && gs.IsInNetwork(pID, cityID)
	case WildLocationCard:
		// Wild Location: Build any industry in any city.
		return true
	case WildIndustryCard:
		// Wild Industry: Build that industry type in any city.
		return card.Industry == ind
	}
	return false
}

// CanBurnCardForBuild checks if ANY card in the hand can be used to build this industry/city combination.
func (gs *GameState) CanBurnCardForBuild(cityID CityID, ind IndustryType, pID PlayerId) bool {
	p := gs.Players[pID]
	for i := range p.Hand {
		if gs.CanCardBeUsedForBuild(cityID, ind, pID, i) {
			return true
		}
	}
	return false
}

// GetActualHandIndex converts a sorted observation index to the actual index in the player's Hand slice.
func (e *Env) GetActualHandIndex(sortedIdx int) (int, bool) {
	p := e.State.Players[e.State.Active]
	if sortedIdx < 0 || sortedIdx >= len(p.Hand) {
		return -1, false
	}

	type cardRef struct {
		card    Card
		origIdx int
	}
	refs := make([]cardRef, len(p.Hand))
	for i, c := range p.Hand {
		refs[i] = cardRef{c, i}
	}

	// Sort using the exact same logic as in observation.go
	sort.SliceStable(refs, func(i, j int) bool {
		ti, tj := refs[i].card.Type, refs[j].card.Type
		if ti != tj {
			return ti < tj // CardType enum order: Location=0, Industry=1, WildIndustry=2, WildLocation=3
		}
		return refs[i].card.CityID < refs[j].card.CityID
	})

	return refs[sortedIdx].origIdx, true
}

// GetCardAndBurn identifies the card at the sorted index and removes it from the player's hand.
func (e *Env) GetCardAndBurn(sortedIdx int) ([]Card, bool) {
	actualIdx, ok := e.GetActualHandIndex(sortedIdx)
	if !ok {
		return nil, false
	}
	card := e.State.Players[e.State.Active].Hand[actualIdx]
	e.State.ReturnCard(e.State.Active, actualIdx)
	return []Card{card}, true
}
