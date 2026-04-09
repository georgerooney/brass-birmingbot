import React from 'react';
import { motion } from 'framer-motion';
import { ChevronLeft, ChevronRight, TrendingUp, SkipBack, SkipForward, Flag } from 'lucide-react';
import type { Step, EngineState } from '../../types';
import { cn } from '../../utils';

interface StepFooterProps {
  replayTrace: Step[];
  currentStep: number;
  onStepChange: (step: number) => void;
  currentState: EngineState | null;
  onJumpToStart: () => void;
  onJumpToEndCanal: () => void;
  onJumpToEndGame: () => void;
}

export const StepFooter: React.FC<StepFooterProps> = ({ 
  replayTrace, 
  currentStep, 
  onStepChange, 
  currentState,
  onJumpToStart,
  onJumpToEndCanal,
  onJumpToEndGame
}) => {
  const step = replayTrace[currentStep];
  if (!step) return null;

  return (
    <motion.footer 
      initial={{ y: 200 }} animate={{ y: 0 }} exit={{ y: 200 }}
      className="p-6 bg-black/80 backdrop-blur-3xl border-t border-white/5 shadow-2xl overflow-x-auto custom-scrollbar"
    >
      <div className="min-w-max flex items-center gap-8 pb-2">
        <div className="flex gap-1.5 shrink-0 border-r border-white/5 pr-6">
          <button 
            onClick={onJumpToStart}
            title="Jump to Start"
            className="p-2 bg-white/5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors"
          >
            <SkipBack className="w-4 h-4" />
          </button>
          
          <div className="flex gap-1 bg-white/5 p-1 rounded-xl">
            <button 
              onClick={() => onStepChange(Math.max(0, currentStep - 1))}
              className="p-2.5 rounded-lg text-white disabled:opacity-20 hover:bg-white/10"
              disabled={currentStep === 0}
            >
              <ChevronLeft className="w-5 h-5" />
            </button>
            <button 
              onClick={() => onStepChange(Math.min(replayTrace.length - 1, currentStep + 1))}
              className="p-2.5 bg-violet-600 rounded-lg text-white disabled:opacity-20 hover:bg-violet-500 shadow-lg shadow-violet-500/20"
              disabled={currentStep === replayTrace.length - 1}
            >
              <ChevronRight className="w-5 h-5" />
            </button>
          </div>

          <button 
            onClick={onJumpToEndCanal}
            title="Jump to End of Canal"
            className="p-2 bg-white/5 rounded-lg text-violet-400 hover:text-violet-300 hover:bg-violet-500/10 transition-colors"
          >
            <Flag className="w-4 h-4" />
          </button>

          <button 
            onClick={onJumpToEndGame}
            title="Jump to End of Game"
            className="p-2 bg-white/5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors"
          >
            <SkipForward className="w-4 h-4" />
          </button>
        </div>

        <div className="w-64 space-y-3 shrink-0">
          <div className="flex justify-between items-end">
            <div className="flex flex-col">
              <span className="text-[10px] font-black text-slate-500 uppercase tracking-widest">Temporal Index</span>
              <span className="text-xl font-black text-white italic tracking-tighter">Step {currentStep + 1} <span className="text-xs text-slate-600 font-normal">/ {replayTrace.length}</span></span>
            </div>
            <div className="text-right flex flex-col items-end">
              <span className={cn(
                "text-[10px] font-black uppercase tracking-widest block",
                (currentState.epoch === 0) ? "text-violet-400" : "text-amber-400"
              )}>
                {(currentState.epoch === 0) ? 'CANAL ERA' : 'RAIL ERA'}
              </span>
              <span className="text-[7px] font-bold text-slate-600 uppercase">Interactive Trace</span>
            </div>
          </div>
          <div className="h-1.5 w-full bg-slate-800 rounded-full overflow-hidden">
            <motion.div 
              className="h-full bg-violet-500" 
              animate={{ width: `${((currentStep + 1) / replayTrace.length) * 100}%` }} 
            />
          </div>
        </div>

        {/* Mutation Card */}
        <div className="w-64 shrink-0">
          <div className="bg-white/5 border border-white/10 p-3 rounded-xl min-h-[140px] flex flex-col">
            <div className="flex justify-between items-center mb-1">
              <span className="text-[9px] font-bold text-slate-500 uppercase tracking-widest block">Action Mutation</span>
              <span className={cn(
                  "text-[8px] font-black px-1.5 py-0.5 rounded",
                  step.player === 0 ? "bg-violet-500/20 text-violet-400" : "bg-pink-500/20 text-pink-400"
              )}>
                  ACTOR: AGENT_{step.player !== undefined ? step.player + 1 : "?"}
              </span>
            </div>
            <p className="text-xs text-white font-black truncate mb-2">{step.action_name}</p>
            <div className="flex-1">
            {step && step.cards_spent && step.cards_spent.length > 0 && (
            <div className="flex flex-col items-center gap-1.5 h-full border-r border-white/5 pr-6">
              <span className="text-[8px] font-black text-slate-500 uppercase tracking-widest leading-none">Cards Spent</span>
              <div className="flex gap-1 items-center justify-center flex-1">
                {step.cards_spent.map((card, i) => {
                  const isWild = card.type === 2 || card.type === 3;
                  const isLoc = card.type === 1 || card.type === 3;
                  const INDUSTRY_NAMES = ["Cotton", "Coal", "Iron", "Pottery", "Goods", "Beer"];
                  const indName = INDUSTRY_NAMES[card.industry] || "Any";
                  const cityName = currentState?.board?.Cities?.find((c: any) => c.ID === card.city_id)?.Name || "Any";
                  const tooltip = isWild 
                    ? (isLoc ? "Wild Location" : "Wild Industry") 
                    : (isLoc ? `Location: ${cityName}` : `Industry: ${indName}`);

                  return (
                    <div 
                      key={i} 
                      title={tooltip}
                      className={cn(
                        "w-5 h-7 rounded border border-white/10 flex items-center justify-center text-[10px] font-black shadow-lg cursor-help transition-transform hover:scale-110",
                        isWild ? "bg-white text-black" : (step.player === 0 ? "bg-violet-600 text-white" : "bg-pink-600 text-white")
                      )}
                    >
                      {isLoc ? 'L' : 'R'}
                    </div>
                  );
                })}
              </div>
            </div>
          )}
            </div>
            {step.slot_idx !== undefined && step.slot_idx !== -1 && (
                <div className="mt-auto pt-2 border-t border-white/5">
                  <span className="text-[8px] font-mono text-slate-500">SLOT_POS_ID: {step.slot_idx}</span>
                </div>
            )}
          </div>
        </div>

        {/* Player Scoring Cards */}
        {[0, 1].map((pId) => {
          const events = (step.score_events || []).filter(e => e.player === pId);
          const projected = step.projected_vps?.[pId] || 0;
          const isActor = step.player === pId;

          return (
              <div key={pId} className="w-56 shrink-0">
                  <div className={cn(
                      "border p-3 rounded-xl min-h-[140px] flex flex-col transition-all",
                      pId === 0 
                          ? (isActor ? "bg-violet-500/10 border-violet-500/40" : "bg-violet-900/5 border-violet-900/10 grayscale-[0.5]") 
                          : (isActor ? "bg-pink-500/10 border-pink-500/40" : "bg-pink-900/5 border-pink-900/10 grayscale-[0.5]")
                  )}>
                      <div className="flex justify-between items-center mb-2">
                          <span className={cn("text-[8px] font-black uppercase tracking-widest", pId === 0 ? "text-violet-400" : "text-pink-400")}>AGENT_{pId + 1} PROJECTION</span>
                          <div className="flex items-center gap-1">
                              <TrendingUp className="w-2.5 h-2.5 opacity-50" />
                              <span className={cn("text-xs font-black", pId === 0 ? "text-violet-200" : "text-pink-200")}>{projected} VP</span>
                          </div>
                      </div>
                      <div className="flex-1 overflow-y-auto custom-scrollbar max-h-20 space-y-1">
                          {events.length > 0 ? (
                              events.map((ev, i) => (
                                  <div key={i} className="flex justify-between items-center bg-black/20 p-1.5 rounded border-l-2 border-amber-500/50">
                                      <div className="flex flex-col">
                                          <span className="text-[8px] text-white/90 font-bold truncate max-w-[120px]">{ev.source}</span>
                                          <span className="text-[6px] text-slate-500 uppercase font-black">{ev.type}</span>
                                      </div>
                                      <span className="text-[10px] text-amber-400 font-black">+{ev.vp}</span>
                                  </div>
                              ))
                          ) : (
                              <div className="h-full flex items-center justify-center opacity-20 italic text-[7px] text-slate-400">No bank events</div>
                          )}
                      </div>
                  </div>
              </div>
          );
        })}
      </div>
    </motion.footer>
  );
};
