import React from 'react';
import { Coins, TrendingUp } from 'lucide-react';
import type { EngineState, Market } from '../../types';
import { cn } from '../../utils';

interface GameHUDProps {
  currentState: EngineState | null;
}

export const GameHUD: React.FC<GameHUDProps> = ({ currentState }) => {
  if (!currentState) return null;

  const { players, coal_market, iron_market, round_counter, epoch, deck } = currentState;
  const INDUSTRY_NAMES = ["Cotton", "Coal", "Iron", "Pottery", "Goods", "Beer"];

  return (
    <div className="absolute top-4 inset-x-4 z-30 pointer-events-none">
      <div className="flex justify-between items-start gap-4">

        {/* Left Control Column: Clock & Markets */}
        <div className="flex flex-col gap-3 pointer-events-auto">
          <div className="flex gap-3">
            {/* Global Clock */}
            <div className="p-3 rounded-2xl border border-white/5 bg-slate-900/60 backdrop-blur-xl shadow-2xl flex flex-col justify-center items-center min-w-[100px]">
              <span className="text-[8px] font-black text-slate-500 uppercase tracking-widest mb-1">Global Clock</span>
              <div className="text-xl font-black text-white italic leading-none mb-1">{round_counter}</div>
              <div className="flex flex-col items-center">
                <span className={cn("text-[9px] font-black uppercase tracking-widest", (epoch === 0) ? "text-violet-400" : "text-amber-400")}>
                  {(epoch === 0) ? 'CANAL ERA' : 'RAIL ERA'}
                </span>
                <span className="text-[7px] font-bold text-slate-500 uppercase mt-0.5">
                   {deck?.length || 0} CARDS IN DECK
                </span>
              </div>
            </div>

            {/* Iron Market */}
            <MarketCard label="Iron" market={iron_market} color="bg-orange-950/40" icon="🟧" />
          </div>

          {/* Coal Market - Spanning below */}
          <MarketCard label="Coal" market={coal_market} color="bg-slate-900/40" icon="⚫" isWide />
        </div>

        {/* Right Side: Agent Grid */}
        <div className="grid grid-cols-2 gap-3 pointer-events-auto items-start">
          {players.map((p, pId) => (
            <div key={pId} className={cn(
              "p-3 rounded-2xl border backdrop-blur-xl shadow-2xl min-w-[200px] transition-all duration-500",
              pId === 0 ? "bg-violet-600/10 border-violet-500/20" : "bg-pink-600/10 border-pink-500/20"
            )}>
              <div className="flex justify-between items-center mb-2">
                <div className="flex flex-col">
                  <span className={cn("text-[9px] font-black uppercase tracking-[0.2em]", pId === 0 ? "text-violet-400" : "text-pink-400")}>AGENT_{pId+1}</span>
                  <span className="text-[7px] font-bold text-white/30 uppercase tracking-tighter">Track Pos: {p.income_level}</span>
                </div>
                <div className={cn("w-2 h-2 rounded-full animate-pulse", pId === 0 ? "bg-violet-500" : "bg-pink-500")} />
              </div>

              <div className="grid grid-cols-2 gap-x-3 gap-y-2 mb-3">
                <div className="space-y-0.5">
                  <div className="flex items-center gap-1 text-slate-500">
                    <Coins className="w-2.5 h-2.5" />
                    <span className="text-[7px] font-bold uppercase tracking-tighter">Money</span>
                  </div>
                  <div className="text-sm font-black text-white italic tracking-tighter leading-none px-0.5">£{p.money}</div>
                </div>
                <div className="space-y-0.5">
                  <div className="flex items-center gap-1 text-slate-500">
                    <TrendingUp className="w-2.5 h-2.5" />
                    <span className="text-[7px] font-bold uppercase tracking-tighter">Income</span>
                  </div>
                  <div className={cn("text-sm font-black italic tracking-tighter leading-none", (p.income || 0) >= 0 ? "text-emerald-400" : "text-rose-400")}>
                    {(p.income || 0) > 0 ? '+' : ''}{p.income || 0}
                  </div>
                </div>
              </div>

              {/* Fixed-Height Scoring Audit */}
              <div className="space-y-1 mb-3 border-t border-white/5 pt-2 min-h-[110px]">
                 <span className="text-[7px] font-black text-slate-500 uppercase tracking-widest block mb-1 opacity-50">Projected metrics</span>
                 <div className="space-y-0.5">
                    {/* Standard Audit Sources (Live Projections) */}
                    <div className="flex justify-between items-center text-[8px]">
                      <span className="text-white/40 uppercase tracking-tighter">Links</span>
                      <span className="text-white font-black">{p.vp_audit_links || 0}</span>
                    </div>

                    {INDUSTRY_NAMES.map(ind => {
                      // Actually, let's use the actual breakdown if it exists, but the user specifically wants projected.
                      // If p.scoring_breakdown is empty, we only have aggregate 'vp_audit_industries'.
                      // We'll show the aggregate under 'Industries' and 0 for specific types unless finalized.
                      const val = p.scoring_breakdown?.[ind] || 0;
                      return (
                        <div key={ind} className="flex justify-between items-center text-[8px]">
                          <span className="text-white/40 uppercase tracking-tighter">{ind}</span>
                          <span className="text-white font-black">{val}</span>
                        </div>
                      );
                    })}

                    {/* Aggregate for Industries Projection */}
                    {(!p.scoring_breakdown || Object.keys(p.scoring_breakdown).length === 0) && (
                       <div className="flex justify-between items-center text-[8px] bg-white/5 px-1 rounded">
                          <span className="text-violet-400 uppercase tracking-tighter font-bold">Unflushed Ind.</span>
                          <span className="text-violet-400 font-black">{p.vp_audit_industries || 0}</span>
                       </div>
                    )}

                    <div className="flex justify-between items-center text-[9px] pt-1.5 border-t border-white/10 mt-1">
                        <span className="text-amber-500/80 font-black uppercase tracking-widest">Total VP</span>
                        <span className="text-amber-400 font-black text-xs">{(p.vp_audit_industries || 0) + (p.vp_audit_links || 0)}</span>
                    </div>
                 </div>
              </div>

              {/* Player Hand (Shrunk cards) */}
              <div className="border-t border-white/5 pt-2">
                <div className="flex gap-1 justify-between">
                  {Array.from({ length: 8 }).map((_, i) => {
                    const card = p.hand[i];
                    if (!card) return (
                      <div key={i} className="w-5 h-7 rounded shrink-0 border border-white/5 border-dashed" />
                    );

                    const isWild = card.type === 2 || card.type === 3;
                    const isLoc = card.type === 1 || card.type === 3;

                    const indName = INDUSTRY_NAMES[card.industry] || "Any";
                    const cityName = currentState.board.Cities.find(c => c.ID === card.city_id)?.Name || "Any";
                    const tooltip = isWild
                      ? (isLoc ? "Wild Location" : "Wild Industry")
                      : (isLoc ? `Location: ${cityName}` : `Industry: ${indName}`);

                    return (
                      <div
                        key={i} title={tooltip}
                        className={cn(
                          "w-5 h-7 rounded shrink-0 flex items-center justify-center text-[9px] font-black border border-white/10 shadow-lg cursor-help transition-transform hover:scale-110",
                          isWild ? "bg-white text-black" : (pId === 0 ? "bg-violet-600 text-white border-violet-400/30" : "bg-pink-600 text-white border-pink-400/30")
                        )}
                      >
                        {isLoc ? 'L' : 'R'}
                      </div>
                    );
                  })}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

const MarketCard = ({ label, market, color, icon, isWide }: { label: string, market: Market, color: string, icon: string, isWide?: boolean }) => (
  <div className={cn("p-2.5 rounded-2xl border border-white/5 backdrop-blur-xl shadow-2xl", color, isWide ? "w-full" : "min-w-[140px]")}>
    <div className="flex items-center gap-1.5 mb-1.5 opacity-60">
      <span className="text-[9px] font-black text-white/50 uppercase tracking-widest">{label}</span>
      <span className="text-[10px]">{icon}</span>
    </div>
    <div className="flex gap-1">
      {market.current_cubes.map((count: number, idx: number) => (
        <div key={idx} className="flex flex-col items-center gap-0.5">
          <div className="w-6 h-8 rounded bg-black/40 border border-white/5 flex flex-col items-center justify-center relative overflow-hidden">
             <div className="absolute inset-x-0 bottom-0 bg-white/5" style={{ height: `${(count / market.capacity[idx]) * 100}%` }} />
             <span className="text-[9px] font-black text-white z-10">{count}</span>
          </div>
          <span className="text-[7px] font-black text-white/20">£{market.prices[idx]}</span>
        </div>
      ))}
    </div>
  </div>
);
