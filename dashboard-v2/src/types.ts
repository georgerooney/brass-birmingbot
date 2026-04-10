interface GameSummary {
  id: number;
  vps: [number, number];
  reward: number;
  trace_file: string;
}

interface Card {
  type: number;
  city_id: number;
  industry: number;
}

interface Market {
  prices: number[];
  capacity: number[];
  current_cubes: number[];
  external_price: number;
}

interface Player {
  id: number;
  money: number;
  income_level: number;
  income: number;
  amount_spent: number;
  hand: Card[];
  current_level: Record<number, number>;
  tokens_left: Record<number, number>;
  vp: number;
  vp_audit_industries: number;
  vp_audit_links: number;
  scoring_breakdown: Record<string, number>;
}

interface ScoreEvent {
  source: string;
  type: string;
  vp: number;
  player: number;
}

interface Industry {
  owner: number;
  city_id: number;
  slot_idx: number;
  industry: number;
  level: number;
  flipped: boolean;
  coal: number;
  iron: number;
  beer: number;
}

interface Route {
  ID: number;
  CityB: number;
  CityA: number;
  Type: string;
  IsSubRoute: boolean;
  SubRoutes: number[];
}

interface City {
  ID: number;
  Name: string;
  Type: string;
  BuildSlots: number[][] | null;
}

interface MerchantSlot {
  city_id: number;
  tile: {
    ID: string;
    Accepts: number[] | null;
  };
  available_beer: number;
}

interface EngineState {
  players: Player[];
  board: {
    Cities: City[];
    Routes: Route[];
    NameMap: Record<string, number>;
  };
  deck: Card[];
  discard: Card[];
  active: number;
  industries: Industry[];
  merchants: MerchantSlot[];
  coal_market: Market;
  iron_market: Market;
  epoch: number;
  round_counter: number;
  actions_remaining: number;
  route_built: boolean[];
  route_owners: number[];
  game_over: boolean;
}

interface Step {
  player: number;
  action_name: string;
  slot_idx: number;
  city_id?: number;
  route_id?: number;
  is_overbuild: boolean;
  era: string;
  state?: EngineState;
  cards_spent?: Card[];
  score_events?: ScoreEvent[];
  projected_vps?: number[];
}

interface AnalysisData {
  num_episodes: number;
  slots: Record<string, { built: number; win_built: number; industry: string; city_id: number; slot_idx: number }>;
  routes: Record<string, { built: number; win_built: number }>;
  moves: Record<string, { overall: number; win: number; lose: number }>;
  decomposition: {
    winner: { avg_ind: number; avg_link: number; avg_merc: number; avg_vp: number };
    loser: { avg_ind: number; avg_link: number; avg_merc: number; avg_vp: number };
  };
}

export type {
  GameSummary,
  Card,
  Market,
  Player,
  ScoreEvent,
  Industry,
  Route,
  City,
  MerchantSlot,
  EngineState,
  Step,
  AnalysisData
};
