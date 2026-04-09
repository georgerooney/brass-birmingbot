package engine

type PlayerId int

const (
	P1 PlayerId = iota
	P2
	P3
	P4
)

type Resource int

const (
	None Resource = iota
	Coal
	Iron
	Beer
)

type IndustryType int

const (
	CottonType IndustryType = iota
	CoalMineType
	IronWorksType
	PotteryType
	ManufacturedGoodsType
	BreweryType
)

type RouteType int

const (
	Canal RouteType = iota
	Rail
)

type CardType int

const (
	LocationCard CardType = iota
	IndustryCard
	WildLocationCard
	WildIndustryCard
)

type Card struct {
	Type     CardType	 	`json:"type"`
	CityID   int          	`json:"city_id"`   // For LocationCards
	Industry IndustryType 	`json:"industry"`  // For IndustryCards
}
