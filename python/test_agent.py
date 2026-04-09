import argparse
from pathlib import Path
import numpy as np
from sb3_contrib import MaskablePPO
from brass_env import BrassEnv
from stable_baselines3.common.env_checker import check_env
from collections import Counter
import pandas as pd
import json
import subprocess
from brass_env.server import ensure_server

ROOT = Path(__file__).resolve().parent.parent

def run_eval(model_path: str, num_episodes: int = 20, output_dir_path: str = "eval_data", render_sample: bool = True, deterministic: bool = True, trace_limit: int = 20):
    print(f"Loading model: {model_path}")
    env = BrassEnv(num_players=2)
    model = MaskablePPO.load(model_path, env=env)
    
    action_names = env.get_action_names()
    
    # File Output Setup
    output_dir = Path(output_dir_path)
    traces_dir = output_dir / "traces"
    traces_dir.mkdir(parents=True, exist_ok=True)
    
    all_scores = []
    all_final_vps = []
    all_ind_vps = []
    all_link_vps = []
    all_parasitism_coal = []
    all_parasitism_iron = []
    
    game_summaries = []
    
    # Advanced Analytics tracking
    slots_stats = {} # city_slot -> { built: 0, win_built: 0, industry: "" }
    routes_stats = {} # route_id -> { built: 0, win_built: 0 }
    move_stats = {} # action_name -> { overall: 0, win: 0, lose: 0 }
    win_counts = Counter() # player_idx -> win_count
    
    # Decomposition (Winner vs Loser)
    decomp = {
        "winner": {"ind": [], "link": [], "merc": [], "vp": []},
        "loser": {"ind": [], "link": [], "merc": [], "vp": []}
    }
    
    print(f"\nEvaluating over {num_episodes} episodes...")
    
    for ep_idx in range(num_episodes):
        obs, info = env.reset(include_state=True)
        trace = [{"player": -1, "action_name": "Game Start", "state": info.get("state")}]
        done = False
        ep_reward = 0
        
        while not done:
            action_masks = env.action_masks()
            action, _ = model.predict(obs, action_masks=action_masks, deterministic=deterministic)
            action = int(action)
            
            obs, reward, terminated, truncated, info = env.step(action, include_state=True)
            meta = info.get("step_metadata", {})
            meta["state"] = info.get("state") # For replayability
            trace.append(meta)
            
            ep_reward += reward
            done = terminated or truncated
            
            if done:
                vps = info["vps"]
                winner_idx = np.argmax(vps) if vps[0] != vps[1] else -1
                if winner_idx != -1:
                    win_counts[winner_idx] += 1
                
                # Points Decomposition
                for i in range(2):
                    role = "winner" if i == winner_idx else "loser"
                    if winner_idx == -1: role = "winner" # Handle draws as winner for baseline
                    
                    vps_ind = info.get("vps_industries") or [0, 0]
                    vps_lnk = info.get("vps_links") or [0, 0]
                    vps_mrc = info.get("vps_merchant") or [0, 0]
                    
                    decomp[role]["ind"].append(vps_ind[i] if i < len(vps_ind) else 0)
                    decomp[role]["link"].append(vps_lnk[i] if i < len(vps_lnk) else 0)
                    decomp[role]["merc"].append(vps_mrc[i] if i < len(vps_mrc) else 0)
                    decomp[role]["vp"].append(vps[i] if i < len(vps) else 0)

                # Process moves and spatial data from trace
                for step in trace:
                    p_id = step.get("player", -1)
                    if p_id == -1: continue
                    
                    is_winner = (p_id == winner_idx)
                    act_name = step.get("action_name", "")
                    
                    # 1. Global Move Usage
                    if act_name not in move_stats:
                        move_stats[act_name] = {"overall": 0, "win": 0, "lose": 0}
                    move_stats[act_name]["overall"] += 1
                    if is_winner: move_stats[act_name]["win"] += 1
                    else: move_stats[act_name]["lose"] += 1
                    
                    # 2. Spatial - Slots
                    city_id = step.get("city_id", -1)
                    slot_idx = step.get("slot_idx", -1)
                    if city_id != -1 and slot_idx != -1:
                        key = f"{city_id}_{slot_idx}"
                        if key not in slots_stats:
                            # Extract industry name from action string if possible
                            industry = act_name.split("Build ")[1].split(" in ")[0] if "Build " in act_name else "Unknown"
                            slots_stats[key] = {"built": 0, "win_built": 0, "industry": industry, "city_id": city_id, "slot_idx": slot_idx}
                        slots_stats[key]["built"] += 1
                        if is_winner: slots_stats[key]["win_built"] += 1
                    
                    # 3. Spatial - Routes
                    route_id = step.get("route_id", -1)
                    if route_id != -1:
                        r_key = str(route_id)
                        if r_key not in routes_stats:
                            routes_stats[r_key] = {"built": 0, "win_built": 0}
                        routes_stats[r_key]["built"] += 1
                        if is_winner: routes_stats[r_key]["win_built"] += 1

                # Save Trace (limited by trace_limit to save disk space)
                if ep_idx < trace_limit:
                    trace_path = traces_dir / f"game_{ep_idx:02d}.json"
                    with open(trace_path, "w") as f:
                        json.dump(trace, f)
                
                game_summaries.append({
                    "id": ep_idx,
                    "vps": vps,
                    "reward": round(ep_reward, 2),
                    "trace_file": f"game_{ep_idx:02d}.json" if ep_idx < trace_limit else None
                })
        
    # Final Export
    analysis_data = {
        "num_episodes": num_episodes,
        "slots": slots_stats,
        "routes": routes_stats,
        "moves": move_stats,
        "decomposition": {
            k: {
                "avg_ind": float(np.mean(v["ind"])) if v["ind"] else 0,
                "avg_link": float(np.mean(v["link"])) if v["link"] else 0,
                "avg_merc": float(np.mean(v["merc"])) if v["merc"] else 0,
                "avg_vp": float(np.mean(v["vp"])) if v["vp"] else 0
            } for k, v in decomp.items()
        },
        "win_rates": {f"Player {i+1}": win_counts[i] / num_episodes for i in range(2)}
    }
    
    with open(output_dir / "analysis_data.json", "w") as f:
        json.dump(analysis_data, f)
        
    with open(output_dir / "eval_index.json", "w") as f:
        json.dump(game_summaries, f)

    print("="*40)
    print("EVALUATION SUMMARY")
    print("="*40)
    print(f"Mean Winner VPs: {analysis_data['decomposition']['winner']['avg_vp']:.1f}")
    print(f"Mean Loser VPs:  {analysis_data['decomposition']['loser']['avg_vp']:.1f}")
    print("-" * 20)
    for p, rate in analysis_data["win_rates"].items():
        print(f"{p} Win Rate: {rate*100:.1f}%")
    print(f"Analytics saved to {output_dir / 'analysis_data.json'}")
    print("="*40)

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--model", type=str, required=True, help="Path to .zip model")
    parser.add_argument("--episodes", type=int, default=10)
    parser.add_argument("--trace-limit", type=int, default=20, help="Max number of full game traces to save as JSON")
    parser.add_argument("--output", type=str, default="eval_data", help="Output directory for traces and index")
    parser.add_argument("--sample", action="store_true", help="Use stochastic sampling instead of deterministic best action")
    parser.add_argument("--no-server", action="store_true", help="Skip server launch")
    args = parser.parse_args()
    
    server_proc = None
    if not args.no_server:
        server_proc = ensure_server(ROOT)
    
    try:
        run_eval(args.model, args.episodes, args.output, deterministic=not args.sample, trace_limit=args.trace_limit)
    finally:
        if server_proc is not None:
            server_proc.terminate()
            server_proc.wait()
            print("Server stopped.")
