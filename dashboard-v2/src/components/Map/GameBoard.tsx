import React from 'react';
import { motion } from 'framer-motion';
import { COORDS } from '../../constants';
import { cn } from '../../utils';
import type { EngineState, AnalysisData } from '../../types';

interface GameBoardProps {
  currentState: EngineState | null;
  analysisData: AnalysisData | null;
  viewMode: 'replay' | 'heatmap';
  showCityLabels?: boolean;
}

export const GameBoard: React.FC<GameBoardProps> = ({ currentState, analysisData, viewMode, showCityLabels }) => {
  return (
    <div className="flex-1 overflow-hidden relative group p-8">
      <svg 
        viewBox="0 0 1000 900" 
        className="w-full h-full drop-shadow-2xl"
        style={{ filter: 'drop-shadow(0 0 10px rgba(0,0,0,0.5))' }}
      >
        {/* Rails/Canals Connections */}
        {currentState?.board?.Routes?.map((route, idx) => {
          const cityA = currentState?.board?.Cities[route.CityA];
          const cityB = currentState?.board?.Cities[route.CityB];
          if (!cityA || !cityB) return null;
          
          const posA = COORDS[cityA.Name] || { x: 0, y: 0 };
          const posB = COORDS[cityB.Name] || { x: 0, y: 0 };
          
          const stats = analysisData?.routes[idx];
          const freq = stats ? stats.built / (analysisData?.num_episodes || 1) : 0;
          const winRate = stats?.built > 0 ? stats.win_built / stats.built : 0;
          
          return (
            <g key={idx} className="route-group">
              <line 
                x1={posA.x} y1={posA.y} x2={posB.x} y2={posB.y}
                stroke={viewMode === 'heatmap' ? `rgba(234, 179, 8, ${0.1 + freq * 0.9})` : (route.IsBuilt ? (route.Owner === 0 ? '#8b5cf6' : '#ec4899') : "rgba(255,255,255,0.12)")}
                strokeWidth={viewMode === 'heatmap' ? 4 + freq * 10 : (route.IsBuilt ? 4 : 2)}
                strokeDasharray={route.Type === "Canal" ? "8,4" : "0"}
                className="transition-all duration-500"
              />
              {viewMode === 'heatmap' && freq > 0 && (
                <title>{`${cityA.Name} <-> ${cityB.Name}\nBuilt: ${(freq*100).toFixed(1)}%\nWin Rate: ${(winRate*100).toFixed(1)}%`}</title>
              )}
            </g>
          );
        })}

        {/* Base City Nodes (Plates) */}
        {Object.entries(COORDS).map(([name, pos]) => (
          <circle key={`plate-${name}`} cx={pos.x} cy={pos.y} r={18} fill="rgba(255,255,255,0.02)" stroke="rgba(255,255,255,0.05)" strokeWidth={1} />
        ))}

        {/* Merchant Slots (NONE or Active) */}
        {viewMode === 'replay' && currentState?.board?.Cities?.filter(c => c.Type === "Merchant").map((city, cIdx) => {
          const pos = COORDS[city.Name];
          if (!pos) return null;
          
          const citySlots = currentState.merchants.filter(m => m.city_id === city.ID);
          const INDUSTRY_ABBRS = ["CT", "CL", "IR", "PT", "MG", "BY"];

          return (
            <g key={`merc-group-${city.ID}`} transform={`translate(${pos.x}, ${pos.y})`} className="group cursor-help">
              {citySlots.map((activeMerc, sIdx) => {
                // Calculate horizontal offset for multi-slot cities (Oxford, Gloucester, Warrington)
                const offset = (sIdx - (citySlots.length - 1) / 2) * 50;
                const isPlaceholder = activeMerc.tile.ID.startsWith('empty') || !activeMerc.tile.Accepts;

                return (
                  <g key={`${city.ID}-slot-${sIdx}`} transform={`translate(${offset}, 0)`}>
                    {/* Base Slot Badge */}
                    <rect x={-22} y={-22} width={44} height={44} rx={8} 
                      fill={isPlaceholder ? "#0f172a" : "#1e293b"} 
                      stroke={isPlaceholder ? "rgba(255,255,255,0.1)" : "#fbbf24"} 
                      strokeWidth={isPlaceholder ? 1 : 2} 
                    />
                    
                    {isPlaceholder ? (
                      <text y={4} textAnchor="middle" fill="rgba(255,255,255,0.15)" className="text-[10px] font-black uppercase tracking-tighter">NONE</text>
                    ) : (
                      <motion.g initial={{ scale: 0.8, opacity: 0 }} animate={{ scale: 1, opacity: 1 }}>
                        {activeMerc.tile.ID.includes('any') ? (
                          <text y={4} textAnchor="middle" fill="#fbbf24" className="text-[12px] font-black tracking-widest">ALL</text>
                        ) : (
                          <g transform="translate(0, 1)">
                             <text y={0} textAnchor="middle" fill="#fbbf24" className="text-[8px] font-black leading-tight">
                                {activeMerc.tile.Accepts?.map(t => INDUSTRY_ABBRS[t]).join('\n')}
                             </text>
                          </g>
                        )}

                        {activeMerc.available_beer > 0 && (
                          <g transform="translate(18, -18)">
                            <circle r={8} fill="#f59e0b" stroke="#000" strokeWidth={1} />
                            <text y={3} textAnchor="middle" fill="black" className="text-[9px] font-black">{activeMerc.available_beer}</text>
                          </g>
                        )}
                      </motion.g>
                    )}
                  </g>
                );
              })}
              <text y={38} textAnchor="middle" fill="white" className="text-[12px] font-bold opacity-0 group-hover:opacity-100 transition-opacity bg-black/80 px-2 rounded whitespace-nowrap pointer-events-none">{city.Name}</text>
            </g>
          );
        })}

        {/* Cities & Slots */}
        {viewMode === 'replay' ? (
          Object.entries(COORDS).map(([name, pos]) => {
            const city = currentState?.board?.Cities?.find(c => c.Name === name);
            if (!city || city.Type === "Merchant") return null;
            
            const slots = city.BuildSlots || [];
            const INDUSTRY_ABBRS = ["CT", "CL", "IR", "PT", "MG", "BY"];
            const INDUSTRY_NAMES = ["Cotton", "Coal", "Iron", "Pottery", "Manufactured Goods", "Brewery"];
            
            return (
              <g key={name} className="city-label">
                {/* Central Interaction Plate for City Name */}
                <g className="group cursor-help">
                  <circle 
                    cx={pos.x} cy={pos.y} r={28} 
                    fill="rgba(255,255,255,0.01)" 
                    className="pointer-events-auto" 
                  />
                  <text 
                    x={pos.x} y={pos.y + 45} textAnchor="middle" fill="white" 
                    className={cn(
                      "text-[12px] font-bold transition-opacity bg-black/80 px-2 rounded whitespace-nowrap pointer-events-none",
                      showCityLabels ? "opacity-100" : "opacity-0 group-hover:opacity-100"
                    )}
                  >
                    {name}
                  </text>
                </g>

                {slots.map((allowedTypes, sIdx) => {
                  const dx = (sIdx % 2 === 0 ? -22 : 22);
                  const dy = (sIdx < 2 ? -22 : 22);
                  const builtIndustry = currentState?.industries?.find(tok => tok.city_id === city.ID && tok.slot_idx === sIdx);
                  const slotAbbr = allowedTypes.map(t => INDUSTRY_ABBRS[t] || "?").join('/');
                  const slotFull = allowedTypes.map(t => INDUSTRY_NAMES[t] || "Unknown").join(' or ');
                  
                  if (builtIndustry) {
                    return (
                      <g key={sIdx} transform={`translate(${pos.x + dx}, ${pos.y + dy})`} className="cursor-help" title={`${INDUSTRY_NAMES[builtIndustry.industry]} (Level ${builtIndustry.level})`}>
                        <rect 
                           x={-16} y={-16} width={32} height={32} rx={6}
                           fill={builtIndustry.owner === 0 ? '#8b5cf6' : '#ec4899'}
                           stroke={builtIndustry.flipped ? '#fbbf24' : 'white'}
                           strokeWidth={builtIndustry.flipped ? 2.5 : 1.5}
                           className="transition-all duration-300"
                        />
                        <text y={3} textAnchor="middle" fill="white" className="text-[10px] font-black pointer-events-none">
                          { INDUSTRY_ABBRS[builtIndustry.industry] }-{builtIndustry.level}
                        </text>

                        {/* Resource Badges */}
                        <g transform="translate(12, 12)">
                          {builtIndustry.coal > 0 && (
                            <g transform="translate(-24, 0)">
                              <rect x={-4} y={-4} width={8} height={8} rx={1} fill="black" stroke="white" strokeWidth={0.5} />
                              <text y={2} textAnchor="middle" fill="white" className="text-[6px] font-black leading-none">{builtIndustry.coal}</text>
                            </g>
                          )}
                          {builtIndustry.iron > 0 && (
                            <g transform="translate(-12, 0)">
                              <rect x={-4} y={-4} width={8} height={8} rx={1} fill="#ea580c" stroke="white" strokeWidth={0.5} />
                              <text y={2} textAnchor="middle" fill="white" className="text-[6px] font-black leading-none">{builtIndustry.iron}</text>
                            </g>
                          )}
                          {builtIndustry.beer > 0 && (
                            <g transform="translate(0, 0)">
                              <circle r={4} fill="#f59e0b" stroke="white" strokeWidth={0.5} />
                              <text y={2} textAnchor="middle" fill="white" className="text-[6px] font-black leading-none">{builtIndustry.beer}</text>
                            </g>
                          )}
                        </g>
                      </g>
                    );
                  }

                  return (
                    <g key={sIdx} transform={`translate(${pos.x + dx}, ${pos.y + dy})`} className="cursor-help" title={`Empty Slot: ${slotFull}`}>
                      <rect x={-14} y={-14} width={28} height={28} rx={6} fill="#1e293b" stroke="rgba(255,255,255,0.15)" strokeWidth={0.8} />
                      <text y={3} textAnchor="middle" fill="rgba(255,255,255,0.5)" className="text-[10px] font-bold">{slotAbbr}</text>
                    </g>
                  );
                })}
              </g>
            );
          })
        ) : (
          Object.entries(COORDS).map(([name, pos]) => {
            const city = currentState?.board?.Cities?.find(c => c.Name === name);
            if (!city) return null;

            const slots = city.BuildSlots || [];
            const INDUSTRY_ABBRS = ["CT", "CL", "IR", "PT", "MG", "BY"];

            return (
              <g key={name} className="city-heatmap group">
                <circle cx={pos.x} cy={pos.y} r={25} fill="rgba(255,255,255,0.03)" stroke="rgba(255,255,255,0.1)" />
                {slots.map((allowedTypes, sIdx) => {
                  const dx = (sIdx % 2 === 0 ? -12 : 12);
                  const dy = (sIdx < 2 ? -12 : 12);
                  const key = `${city.ID}_${sIdx}`;
                  const stats = analysisData?.slots[key];
                  const freq = stats ? stats.built / (analysisData?.num_episodes || 1) : 0;
                  const winRate = stats?.built > 0 ? stats.win_built / stats.built : 0;

                  return (
                    <g key={sIdx} transform={`translate(${pos.x + dx}, ${pos.y + dy})`}>
                      <rect 
                        x={-8} y={-8} width={16} height={16} rx={3}
                        fill={`rgba(139, 92, 246, ${0.1 + freq * 0.9})`}
                        stroke="rgba(255,255,255,0.15)"
                        strokeWidth={0.5}
                      />
                      {freq > 0 && <title>{`${name} Slot ${sIdx} (${allowedTypes.map(t => INDUSTRY_ABBRS[t]).join('/')})\nBuilt: ${(freq*100).toFixed(1)}%\nWin Rate: ${(winRate*100).toFixed(1)}%`}</title>}
                    </g>
                  );
                })}
                <text x={pos.x} y={pos.y + 35} textAnchor="middle" fill="white" className="text-[8px] font-bold opacity-50">{name}</text>
              </g>
            );
          })
        )}
      </svg>
    </div>
  );
};
