package engine

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed industry_tiles.json
var industryTilesData []byte

type IndustryStat struct {
	Level          int      `json:"level"`
	Count          int      `json:"count"`
	Income         int      `json:"income"`
	VP             int      `json:"vp"`
	LinkVP         int      `json:"link_vp"`
	CostMoney      int      `json:"cost_money"`
	CostCoal       int      `json:"cost_coal"`
	CostIron       int      `json:"cost_iron"`
	CostBeerToSell int      `json:"cost_beer_to_sell"`
	YieldCanal     int      `json:"yield_canal"`
	YieldRail      int      `json:"yield_rail"`
	IsDevelopable  bool     `json:"is_developable"`
	Eras           []string `json:"eras"`
}

// IndustryTilesJSON removed as we parse directly into map

var IndustryCatalog map[IndustryType]map[int]IndustryStat
var IndustryMaxLevel map[IndustryType]int

// Init called automatically when package loads
func init() {
	LoadIndustryCatalog()
}

func LoadIndustryCatalog() {
	var iJSON map[string][]IndustryStat
	if err := json.Unmarshal(industryTilesData, &iJSON); err != nil {
		fmt.Printf("Error unmarshaling industry_tiles.json: %v\n", err)
		return
	}

	IndustryCatalog = make(map[IndustryType]map[int]IndustryStat)
	IndustryMaxLevel = make(map[IndustryType]int)

	stringToInd := func(s string) IndustryType {
		switch s {
		case "cotton":
			return CottonType
		case "goods":
			return ManufacturedGoodsType
		case "coal":
			return CoalMineType
		case "iron":
			return IronWorksType
		case "pottery":
			return PotteryType
		case "brewery":
			return BreweryType
		default:
			return -1
		}
	}

	for k, stats := range iJSON {
		ind := stringToInd(k)
		if ind != -1 {
			IndustryCatalog[ind] = make(map[int]IndustryStat)
			maxLvl := 0
			for _, stat := range stats {
				IndustryCatalog[ind][stat.Level] = stat
				if stat.Level > maxLvl {
					maxLvl = stat.Level
				}
			}
			IndustryMaxLevel[ind] = maxLvl
		}
	}
}
