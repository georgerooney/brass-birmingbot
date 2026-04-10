package engine

// DeckDistribution holds the specific card counts based on player count.
type DeckDistribution struct {
	NumPlayers int
	Locations  map[string]int
	Industries map[IndustryType]int
}

// GetDeckDistribution returns the card counts mapped exactly to the image provided.
func GetDeckDistribution(numPlayers int) DeckDistribution {
	dist := DeckDistribution{
		NumPlayers: numPlayers,
		Locations:  make(map[string]int),
		Industries: make(map[IndustryType]int),
	}

	if numPlayers < 2 || numPlayers > 4 {
		return dist // Unsupported defaults to empty
	}

	// Light Blue Region
	if numPlayers == 4 {
		dist.Locations["Belper"] = 2
		dist.Locations["Derby"] = 3
	}

	// Blue Region
	if numPlayers >= 3 {
		dist.Locations["Leek"] = 2
		dist.Locations["Stoke-on-Trent"] = 3
		dist.Locations["Stone"] = 2
		if numPlayers == 4 {
			dist.Locations["Uttoxeter"] = 2
		} else {
			dist.Locations["Uttoxeter"] = 1
		}
	}

	// Pink Region
	dist.Locations["Stafford"] = 2
	dist.Locations["Burton-upon-Trent"] = 2
	dist.Locations["Cannock"] = 2
	dist.Locations["Tamworth"] = 1
	dist.Locations["Walsall"] = 1

	// Yellow Region
	dist.Locations["Coalbrookdale"] = 3
	dist.Locations["Dudley"] = 2
	dist.Locations["Kidderminster"] = 2
	dist.Locations["Wolverhampton"] = 2
	dist.Locations["Worcester"] = 2

	// Purple Region
	dist.Locations["Birmingham"] = 3
	dist.Locations["Coventry"] = 3
	dist.Locations["Nuneaton"] = 1
	dist.Locations["Redditch"] = 1

	// Industry Cards
	dist.Industries[IronWorksType] = 4
	dist.Industries[BreweryType] = 5

	if numPlayers == 4 {
		dist.Industries[CoalMineType] = 3
		dist.Industries[PotteryType] = 3
		// Note: The image shows "MAN. GOODS / COTTON MILL" as one type of card
		dist.Industries[ManufacturedGoodsType] = 8
		dist.Industries[CottonType] = 8
	} else if numPlayers == 3 {
		dist.Industries[CoalMineType] = 2
		dist.Industries[PotteryType] = 2
		dist.Industries[ManufacturedGoodsType] = 6
		dist.Industries[CottonType] = 6
	} else if numPlayers == 2 {
		dist.Industries[CoalMineType] = 2
		dist.Industries[PotteryType] = 2
		// The image shows '-' for MAN. GOODS / COTTON MILL for 2 players
		dist.Industries[ManufacturedGoodsType] = 0
		dist.Industries[CottonType] = 0
	}

	return dist
}

// InitializeDeck populates the deck using distribution, shuffles it, and deals 8 initial cards.
func (gs *GameState) InitializeDeck() {
	dist := GetDeckDistribution(gs.NumPlayers)
	var deck []Card

	for city, count := range dist.Locations {
		if cityID, exists := gs.Board.NameMap[city]; exists {
			for i := 0; i < count; i++ {
				deck = append(deck, Card{
					Type:   LocationCard,
					CityID: int(cityID),
				})
			}
		}
	}

	for ind, count := range dist.Industries {
		for i := 0; i < count; i++ {
			deck = append(deck, Card{
				Type:     IndustryCard,
				Industry: ind,
			})
		}
	}

	// Each env has its own Rng; shuffle through it to avoid global rand contention
	// across thousands of parallel environments.
	if gs.Rng != nil {
		gs.Rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	}

	gs.Deck = deck

	// Remove 1 card to discard pile at start of game
	if len(gs.Deck) > 0 {
		gs.Discard = append(gs.Discard, gs.Deck[len(gs.Deck)-1])
		gs.Deck = gs.Deck[:len(gs.Deck)-1]
	}

	for p := 0; p < gs.NumPlayers; p++ {
		// Allocate a fresh slice per player — assigning gs.Deck[n:] directly would give all
		// players the same backing array, so mutating one hand would corrupt the others.
		hand := make([]Card, 8)
		copy(hand, gs.Deck[len(gs.Deck)-8:])
		gs.Players[p].Hand = hand
		gs.Deck = gs.Deck[:len(gs.Deck)-8]
	}
}
