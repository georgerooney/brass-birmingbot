package engine

// ─── Card discard / burn helpers ──────────────────────────────────────────────

// DiscardCardFromPlayer removes a card by priority and returns it.
func (gs *GameState) DiscardCardFromPlayer(playerID PlayerId, priority []CardType) ([]Card, bool) {
	return gs.DiscardMultipleCardsFromPlayer(playerID, 1, priority)
}

// DiscardMultipleCardsFromPlayer removes N cards by priority order.
func (gs *GameState) DiscardMultipleCardsFromPlayer(playerID PlayerId, count int, priority []CardType) ([]Card, bool) {
	p := gs.Players[playerID]
	var removedCards []Card
	foundTotal := 0

	for foundTotal < count {
		foundThisIter := false
		for _, targetType := range priority {
			for i, c := range p.Hand {
				if c.Type == targetType {
					removed := p.Hand[i]
					p.Hand = append(p.Hand[:i], p.Hand[i+1:]...)

					if removed.Type == WildLocationCard {
						gs.WildLocationSupply++
					} else if removed.Type == WildIndustryCard {
						gs.WildIndustrySupply++
					} else {
						gs.Discard = append(gs.Discard, removed)
					}
					removedCards = append(removedCards, removed)
					foundTotal++
					foundThisIter = true
					break
				}
			}
			if foundThisIter {
				break
			}
		}
		if !foundThisIter {
			break
		}
	}

	return removedCards, foundTotal == count
}

// DiscardCardFromPlayerWithType removes the first card of the given type and returns it.
func (gs *GameState) DiscardCardFromPlayerWithType(playerID PlayerId, t CardType) (Card, bool) {
	p := gs.Players[playerID]
	for i, c := range p.Hand {
		if c.Type == t {
			card := p.Hand[i]
			gs.ReturnCard(playerID, i)
			return card, true
		}
	}
	return Card{}, false
}

// ReturnCard removes the i-th card from player hand and returns it to supply if Wild.
func (gs *GameState) ReturnCard(activePlayer PlayerId, cardIdx int) {
	p := gs.Players[activePlayer]
	card := p.Hand[cardIdx]

	p.Hand = append(p.Hand[:cardIdx], p.Hand[cardIdx+1:]...)

	if card.Type == WildLocationCard {
		gs.WildLocationSupply++
	} else if card.Type == WildIndustryCard {
		gs.WildIndustrySupply++
	} else {
		gs.Discard = append(gs.Discard, card)
	}
}

// BurnCardForBuild selects and discards the most appropriate card for a build action.
// Priority: Specific Location Card > Industry Card (in-network) > Wild Location > Wild Industry.
func (gs *GameState) BurnCardForBuild(cityID CityID, ind IndustryType, playerID PlayerId) (Card, bool) {
	p := gs.Players[playerID]
	city := gs.Board.Cities[cityID]

	// 1. Specific Location Card
	for i, c := range p.Hand {
		if c.Type == LocationCard && c.CityID == int(city.ID) {
			card := p.Hand[i]
			gs.ReturnCard(playerID, i)
			return card, true
		}
	}

	// 2. Matching Industry Card (Must be in network, or first build)
	inNetwork := gs.IsInNetwork(playerID, cityID) || len(gs.IndustriesForPlayer(playerID)) == 0
	if inNetwork {
		for i, c := range p.Hand {
			if c.Type == IndustryCard && c.Industry == ind {
				card := p.Hand[i]
				gs.ReturnCard(playerID, i)
				return card, true
			}
		}
	}

	// 3. Wild Location Card (Cannot build Farm Breweries)
	if city.Type != "FarmBrewery" {
		if card, ok := gs.DiscardCardFromPlayerWithType(playerID, WildLocationCard); ok {
			return card, true
		}
	}

	// 4. Wild Industry Card
	return gs.DiscardCardFromPlayerWithType(playerID, WildIndustryCard)
}

// CanBurnCardForBuild checks if any card in hand can fulfill the build.
func (gs *GameState) CanBurnCardForBuild(cityID CityID, ind IndustryType, playerID PlayerId) bool {
	p := gs.Players[playerID]
	city := gs.Board.Cities[cityID]

	for _, c := range p.Hand {
		if c.Type == LocationCard && c.CityID == int(city.ID) {
			return true
		}
		if c.Type == WildLocationCard && city.Type != "FarmBrewery" {
			return true
		}
		if c.Type == IndustryCard && c.Industry == ind {
			if gs.IsInNetwork(playerID, cityID) || len(gs.IndustriesForPlayer(playerID)) == 0 {
				return true
			}
		}
		if c.Type == WildIndustryCard {
			return true
		}
	}
	return false
}

// IndustriesForPlayer returns all industry tokens owned by the given player.
func (gs *GameState) IndustriesForPlayer(id PlayerId) []*TokenState {
	var res []*TokenState
	for _, tok := range gs.Industries {
		if tok.Owner == id {
			res = append(res, tok)
		}
	}
	return res
}

// CanCardBeUsedForBuild checks if a specific card in the player's hand (by index)
// is rule-legal for building a specific industry in a specific city.
func (gs *GameState) CanCardBeUsedForBuild(cityID CityID, ind IndustryType, playerID PlayerId, cardIdx int) bool {
	p := gs.Players[playerID]
	if cardIdx < 0 || cardIdx >= len(p.Hand) {
		return false
	}
	
	card := p.Hand[cardIdx]
	city := gs.Board.Cities[cityID]

	switch card.Type {
	case LocationCard:
		return card.CityID == int(cityID)
	case IndustryCard:
		if card.Industry != ind { return false }
		// Industry cards require connection (or first build)
		return gs.IsInNetwork(playerID, cityID) || len(gs.IndustriesForPlayer(playerID)) == 0
	case WildLocationCard:
		return city.Type != "FarmBrewery"
	case WildIndustryCard:
		return true
	}
	return false
}
