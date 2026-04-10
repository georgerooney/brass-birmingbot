package engine

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed mercantile_tiles.json
var mercantileTilesData []byte

type MercantileTilesJSON struct {
	MerchantPools map[string][]struct {
		ID      string   `json:"id"`
		Accepts []string `json:"accepts"`
	} `json:"merchant_pools"`
}

type MerchantTile struct {
	ID      string
	Accepts []IndustryType
}

var MerchantPools map[int][]MerchantTile

func init() {
	LoadMerchantPools()
}

func LoadMerchantPools() {
	var mJSON MercantileTilesJSON
	if err := json.Unmarshal(mercantileTilesData, &mJSON); err != nil {
		fmt.Printf("Error unmarshaling mercantile_tiles.json: %v\n", err)
		return
	}

	MerchantPools = make(map[int][]MerchantTile)

	for pCountStr, tiles := range mJSON.MerchantPools {
		pCount := 0
		fmt.Sscanf(pCountStr, "%d", &pCount)

		var parsedTiles []MerchantTile
		for _, t := range tiles {
			var accepts []IndustryType
			for _, a := range t.Accepts {
				switch a {
				case "cotton":
					accepts = append(accepts, CottonType)
				case "goods":
					accepts = append(accepts, ManufacturedGoodsType)
				case "pottery":
					accepts = append(accepts, PotteryType)
				}
			}
			parsedTiles = append(parsedTiles, MerchantTile{ID: t.ID, Accepts: accepts})
		}
		MerchantPools[pCount] = parsedTiles
	}
}

// EvaluateMerchantBeerBonus processes the flat bonuses for drinking a merchant's beer.
// Modifies the player state directly.
func (p *PlayerState) EvaluateMerchantBeerBonus(cityStr string) *ScoreEvent {
	var ev *ScoreEvent
	switch cityStr {
	case "Shrewsbury":
		p.VP += 4
		p.ScoringBreakdown["Merchant Bonus"] += 4
		ev = &ScoreEvent{Source: "Shrewsbury", Type: "Merchant", VP: 4, Player: int(p.ID)}
	case "Nottingham":
		p.VP += 3
		p.ScoringBreakdown["Merchant Bonus"] += 3
		ev = &ScoreEvent{Source: "Nottingham", Type: "Merchant", VP: 3, Player: int(p.ID)}
	case "Warrington":
		p.Money += 5
	case "Oxford":
		p.IncomeLevel += 2
	case "Gloucester":
		p.FreeDevelopments += 1
	}
	p.SyncIncome()
	return ev
}
