import React from 'react';
import { motion } from 'framer-motion';
import type { AnalysisData } from '../../types';
import { cn } from '../../utils';

interface StrategyHubProps {
  analysisData: AnalysisData;
}

export const StrategyHub: React.FC<StrategyHubProps> = ({ analysisData }) => {
  return (
    <motion.div
      initial={{ opacity: 0, x: 20 }} animate={{ opacity: 1, x: 0 }}
      className="absolute top-8 right-8 w-96 bg-black/80 backdrop-blur-xl rounded-2xl p-6 border border-white/10 space-y-6 max-h-[80%] overflow-y-auto custom-scrollbar shadow-2xl"
    >
      <div className="space-y-1">
          <h3 className="text-sm font-black text-amber-400 uppercase tracking-tighter">Strategic Insights</h3>
          <p className="text-[10px] text-slate-500 font-bold">Aggregated over {analysisData.num_episodes} games</p>
      </div>

      <div className="space-y-2">
          <span className="text-[9px] font-black uppercase text-slate-400">Yield Comparison (Avg VP)</span>
          <div className="grid grid-cols-2 gap-4">
              {['winner', 'loser'].map(role => {
                  const data = analysisData.decomposition[role as 'winner' | 'loser'];
                  return (
                    <div key={role} className="space-y-1">
                        <span className={cn("text-[8px] font-black uppercase", role === 'winner' ? 'text-violet-400' : 'text-pink-400')}>{role}</span>
                        <div className="bg-white/5 p-2 rounded space-y-1">
                            <div className="flex justify-between text-[8px] font-bold"><span className="text-slate-500">IND</span><span className="text-white">{data?.avg_ind.toFixed(1)}</span></div>
                            <div className="flex justify-between text-[8px] font-bold"><span className="text-slate-500">LNK</span><span className="text-white">{data?.avg_link.toFixed(1)}</span></div>
                            <div className="flex justify-between text-[8px] font-bold"><span className="text-slate-500">MERC</span><span className="text-white">{data?.avg_merc.toFixed(1)}</span></div>
                            <div className="pt-1 mt-1 border-t border-white/10 flex justify-between text-[10px] font-black"><span className="text-slate-400">TOTAL</span><span className="text-white">{data?.avg_vp.toFixed(1)}</span></div>
                        </div>
                    </div>
                  );
              })}
          </div>
      </div>

      <div className="space-y-3">
          <div className="flex justify-between items-center">
              <span className="text-[9px] font-black uppercase text-slate-400">Move Predictive Value</span>
              <span className="text-[7px] text-violet-400 font-black px-1.5 py-0.5 bg-violet-500/10 rounded tracking-widest">W_FREQ / L_FREQ</span>
          </div>
          <div className="space-y-1.5">
              {Object.entries(analysisData.moves)
                  .sort((a,b) => (b[1].win / (b[1].lose || 1)) - (a[1].win / (a[1].lose || 1)))
                  .slice(0, 10).map(([move, stats]) => {
                      const ratio = stats.win / (stats.lose || 1);
                      return (
                          <div key={move} className="flex flex-col bg-white/5 p-2 rounded border-l-2 border-violet-500">
                              <div className="flex justify-between items-start">
                                  <span className="text-[9px] text-white font-black truncate w-48" title={move}>{move}</span>
                                  <span className="text-[10px] text-violet-400 font-black">{ratio.toFixed(2)}x</span>
                              </div>
                              <div className="flex gap-3 text-[7px] font-bold text-slate-500 mt-1">
                                  <span>WIN: {stats.win}</span>
                                  <span>LOSE: {stats.lose}</span>
                                  <span>USAGE: {stats.overall}</span>
                              </div>
                          </div>
                      );
                  })
              }
          </div>
      </div>
    </motion.div>
  );
};
