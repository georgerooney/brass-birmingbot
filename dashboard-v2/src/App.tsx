import React, { useState, useEffect, useMemo } from 'react';
import { AnimatePresence } from 'framer-motion';
import {
  Activity,
  Zap,
  History,
  Map as MapIcon,
  Search,
  Loader2,
  AlertCircle,
  Play
} from 'lucide-react';
import type { GameSummary, Step, AnalysisData } from './types';
import { cn } from './utils';

// Components
import { GameBoard } from './components/Map/GameBoard';
import { StrategyHub } from './components/Analytics/StrategyHub';
import { StepFooter } from './components/Navigation/StepFooter';
import { GameHUD } from './components/Navigation/GameHUD';

export default function App() {
  const [games, setGames] = useState<GameSummary[]>([]);
  const [selectedGame, setSelectedGame] = useState<GameSummary | null>(null);
  const [replayTrace, setReplayTrace] = useState<Step[]>([]);
  const [currentStep, setCurrentStep] = useState(0);
  const [analysisData, setAnalysisData] = useState<AnalysisData | null>(null);
  const [viewMode, setViewMode] = useState<'replay' | 'heatmap'>('replay');
  const [showCityLabels, setShowCityLabels] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Initialize Data
  useEffect(() => {
    async function loadData() {
      try {
        setLoading(true);
        const idxRes = await fetch('/data/eval_index.json');
        if (!idxRes.ok) throw new Error("Could not find eval_index.json");
        setGames(await idxRes.json());

        try {
          const analysisRes = await fetch('/data/analysis_data.json');
          if (analysisRes.ok) {
            setAnalysisData(await analysisRes.json());
          }
        } catch (e) {
          console.warn("Failed to parse analysis data:", e);
        }

        setLoading(false);
      } catch (err) {
        console.error("Dashboard Init Error:", err);
        setError((err as Error).message || "Unknown error");
        setLoading(false);
      }
    }
    loadData();
  }, []);

  // Load Trace
  const loadTrace = async (game: GameSummary) => {
    try {
      setLoading(true);
      setError(null);
      const res = await fetch(`/data/traces/${game.trace_file}`);
      if (!res.ok) throw new Error(`Engine trace not found: ${game.trace_file}`);
      const data = await res.json();
      setReplayTrace(data);
      setSelectedGame(game);
      setCurrentStep(0);
      setViewMode('replay');
      setLoading(false);
    } catch {
      setError("Failed to load game trace");
      setLoading(false);
    }
  };

  const currentState = useMemo(() => {
    if (viewMode === 'replay' && replayTrace.length > 0) {
      return replayTrace[currentStep]?.state || null;
    }
    return (replayTrace.length > 0 ? replayTrace[0]?.state : null) || null;
  }, [replayTrace, currentStep, viewMode]);

  // Navigation Jumps
  const onJumpToStart = () => setCurrentStep(0);
  const onJumpToEndCanal = () => {
    // Find the last index where the state is still in Canal Era (epoch 0)
    const idx = [...replayTrace].reverse().findIndex(s => s.state?.epoch === 0);
    if (idx !== -1) {
      setCurrentStep(replayTrace.length - 1 - idx);
    }
  };
  const onJumpToEndGame = () => setCurrentStep(replayTrace.length - 1);

  if (loading && games.length === 0) {
    return (
      <div className="h-screen w-screen bg-[#020617] flex flex-col items-center justify-center space-y-4">
        <Loader2 className="w-12 h-12 text-violet-500 animate-spin" />
        <p className="text-slate-500 font-black uppercase tracking-widest text-xs">Initializing Brass Vision...</p>
      </div>
    );
  }

  return (
    <div className="h-screen w-screen bg-[#020617] text-slate-200 overflow-hidden flex flex-col font-sans selection:bg-violet-500/30">
      <header className="h-16 border-b border-white/5 bg-black/40 backdrop-blur-md flex items-center justify-between px-8 z-50">
        <div className="flex items-center gap-4">
          <div className="w-10 h-10 bg-gradient-to-br from-violet-600 to-indigo-700 rounded-xl flex items-center justify-center shadow-lg shadow-violet-500/20">
            <Activity className="w-6 h-6 text-white" />
          </div>
          <div>
            <h1 className="text-lg font-black tracking-tighter text-white uppercase italic">Brass Vision <span className="text-violet-500">Birmingham</span></h1>
            <div className="flex items-center gap-2">
              <span className="w-1.5 h-1.5 rounded-full bg-green-500 animate-pulse" />
              <span className="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Engine Diagnostics : 2.0.4-RL</span>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-6">
          <button
            onClick={() => setShowCityLabels(!showCityLabels)}
            className={cn(
              "flex items-center gap-2 px-3 py-1.5 rounded-lg text-[9px] font-black transition-all uppercase tracking-widest border",
              showCityLabels ? "bg-amber-500/20 border-amber-500/50 text-amber-500" : "bg-white/5 border-white/10 text-slate-500 hover:text-white"
            )}
          >
            <MapIcon className="w-3.5 h-3.5" /> {showCityLabels ? "HIDE LABELS" : "SHOW LABELS"}
          </button>

          <div className="flex items-center gap-3 bg-white/5 p-1 rounded-xl border border-white/10">
            <button
              onClick={() => setViewMode('replay')}
              className={cn(
                "flex items-center gap-2 px-4 py-2 rounded-lg text-xs font-black transition-all uppercase tracking-widest",
                viewMode === 'replay' ? "bg-violet-600 text-white shadow-lg" : "text-slate-400 hover:text-white"
              )}
            >
              <History className="w-4 h-4" /> REPLAY VCR
            </button>
            <button
              onClick={() => setViewMode('heatmap')}
              className={cn(
                "flex items-center gap-2 px-4 py-2 rounded-lg text-xs font-black transition-all uppercase tracking-widest",
                viewMode === 'heatmap' ? "bg-amber-600 text-white shadow-lg" : "text-slate-400 hover:text-white"
              )}
            >
              <Zap className="w-4 h-4" /> STRATEGY HUB
            </button>
          </div>
        </div>
      </header>

      <div className="flex-1 flex overflow-hidden">
        <aside className="w-80 border-r border-white/5 bg-black/20 backdrop-blur-sm flex flex-col">
          <div className="p-6 border-b border-white/5">
            <div className="relative group">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500 group-focus-within:text-violet-400 transition-colors" />
              <input
                type="text" placeholder="FILTER SIMULATIONS..."
                className="w-full bg-white/5 border border-white/10 rounded-xl py-2.5 pl-10 pr-4 text-[10px] font-black tracking-widest uppercase focus:outline-none focus:border-violet-500/50 transition-all"
              />
            </div>
          </div>
          <div className="flex-1 overflow-y-auto custom-scrollbar p-4 space-y-2">
            {games.map(game => (
              <button
                key={game.id} onClick={() => loadTrace(game)}
                className={cn(
                  "w-full p-4 rounded-xl border transition-all flex flex-col gap-2 group",
                  selectedGame?.id === game.id ? "bg-violet-600/10 border-violet-500/50" : "bg-white/20 border-white/5 hover:bg-white/5 hover:border-white/10"
                )}
              >
                <div className="flex justify-between items-start w-full">
                  <span className="text-[10px] font-black text-slate-500 uppercase tracking-widest group-hover:text-violet-400 transition-colors">Simulation #{game.id}</span>
                </div>
                <div className="flex justify-between items-end w-full">
                  <div className="text-left">
                    <p className="text-lg font-black text-white italic tracking-tighter">{game.vps[0]} - {game.vps[1]}</p>
                    <p className="text-[9px] font-bold text-slate-500 uppercase tracking-widest mt-0.5">Final Scoreline</p>
                  </div>
                </div>
              </button>
            ))}
          </div>
        </aside>

        <main className="flex-1 relative overflow-hidden flex flex-col">
          {error && (
            <div className="absolute top-4 right-4 z-[100] animate-in slide-in-from-top duration-300">
              <div className="bg-red-500/10 border border-red-500/50 backdrop-blur-xl p-4 rounded-2xl flex items-center gap-4 shadow-2xl shadow-red-500/20">
                <AlertCircle className="w-6 h-6 text-red-500" />
                <p className="text-sm font-bold text-white tracking-tight">{error}</p>
              </div>
            </div>
          )}

          {!selectedGame && !loading && (
            <div className="absolute inset-0 z-[40] flex flex-col items-center justify-center bg-black/40 backdrop-blur-sm">
              <div className="p-12 rounded-3xl bg-slate-900/50 border border-white/5 flex flex-col items-center text-center space-y-6 max-w-md">
                <Play className="w-10 h-10 text-violet-400" />
                <h2 className="text-2xl font-black italic tracking-tighter text-white uppercase mb-2">Initialize VCR</h2>
              </div>
            </div>
          )}

          {viewMode === 'replay' && <GameHUD currentState={currentState} />}

          <GameBoard
            currentState={currentState}
            analysisData={analysisData}
            viewMode={viewMode}
            showCityLabels={showCityLabels}
          />

          {viewMode === 'heatmap' && analysisData && (
            <StrategyHub analysisData={analysisData} />
          )}

          <AnimatePresence>
            {viewMode === 'replay' && replayTrace.length > 0 && (
              <StepFooter
                replayTrace={replayTrace}
                currentStep={currentStep}
                onStepChange={setCurrentStep}
                currentState={currentState}
                onJumpToStart={onJumpToStart}
                onJumpToEndCanal={onJumpToEndCanal}
                onJumpToEndGame={onJumpToEndGame}
              />
            )}
          </AnimatePresence>
        </main>
      </div>
    </div>
  );
}
