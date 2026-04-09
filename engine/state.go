// Package engine — state.go has been split into focused modules.
//
// Former contents now live in:
//   game.go      — GameState struct, NewGameState, era/turn queries
//   market.go    — Market struct, buy/sell/predict operations
//   resources.go — SourceCoal, SourceIron, SourceBeer, cost predictions
//   network.go   — BFS (HasConnectionFast, IsMerchantConnected, IsInNetwork, IsAdjacentToNetwork, findBestCoalSource, CanBuildDoubleRail)
//   player.go    — PlayerState methods (income, token, develop)
//   cards.go     — Card discard/burn/return helpers
//   board.go     — Slot queries, CanSellToMerchant, IsResourceExhausted, PredictSellableIndustries
//   scoring.go   — FlipIndustry, ScoreEra, ProcessIncome, HandleShortfall, EndEraTransition
//   observation.go — [TODO] Tensor export for Python interop

package engine
