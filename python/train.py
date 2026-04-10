"""
train.py — Brass Birmingham PPO training entry point.

Uses MaskablePPO from sb3-contrib so the policy never selects an illegal action.
The Go engine server is started automatically as a subprocess.

Usage:
    cd d:\\projects\\brass\\python
    uv run train.py [--envs N] [--steps N] [--lr LR]
"""

from __future__ import annotations
import argparse
import os
import subprocess
import sys
import time
from collections import deque, defaultdict
from datetime import datetime
from pathlib import Path
import multiprocessing

import numpy as np
import torch # Just the base import, we'll use th inside functions

# Base SB3 classes are okay at top-level; the heavy model/torch logic is moved inside main()
from stable_baselines3.common import utils
from stable_baselines3.common.callbacks import CheckpointCallback, BaseCallback
from stable_baselines3.common.vec_env import SubprocVecEnv, VecMonitor
from config import CurriculumState, DynamicLRScheduler, get_args, PPO_N_STEPS, PPO_BATCH_SIZE, PPO_N_EPOCHS, PPO_GAMMA, PPO_GAE_LAMBDA, PPO_CLIP_RANGE, PPO_TARGET_KL, PPO_ENT_COEF, NET_ARCH
from utils import DiagnosticCallback

# --- Data Structures moved to config.py ---

# Removed server import

# --- Global Helpers ---

def make_env_fn(rank: int, num_players: int):
    """Factory that SubprocVecEnv calls in each worker subprocess."""
    def _init():
        from brass_env import BrassEnv
        env = BrassEnv(num_players=num_players)
        return env
    return _init

class CardAttentionExtractor(BaseFeaturesExtractor):
    def __init__(self, observation_space, hand_size: int = 8, card_width: int = 24):
        total_dim = observation_space.shape[0]
        hand_dim = hand_size * card_width
        board_dim = total_dim - hand_dim
        
        # Output dim is total_dim to match old model shapes!
        super().__init__(observation_space, total_dim)
        
        self.hand_size = hand_size
        self.card_width = card_width
        self.hand_dim = hand_dim
        
        # Small transformer for cards
        self.card_embed = nn.Linear(card_width, 16)
        encoder_layer = nn.TransformerEncoderLayer(d_model=16, nhead=1, dim_feedforward=32, batch_first=True)
        self.transformer = nn.TransformerEncoder(encoder_layer, num_layers=1)
        
        # Project back to hand_dim so total output dim is board_dim + hand_dim = total_dim
        self.card_expand = nn.Linear(16, hand_dim)
        
    def forward(self, observations: th.Tensor) -> th.Tensor:
        board_obs = observations[:, :-self.hand_dim]
        hand_obs = observations[:, -self.hand_dim:]
        
        hand_obs = hand_obs.view(-1, self.hand_size, self.card_width)
        hand_embed = self.card_embed(hand_obs)
        hand_trans = self.transformer(hand_embed)
        hand_pooled = th.mean(hand_trans, dim=1)
        
        hand_expanded = self.card_expand(hand_pooled)
        
        return th.cat([board_obs, hand_expanded], dim=1)

# --- Callbacks ---

class PositionWinRateCallback(BaseCallback):
    """Logs the win rate of Player 1 to TensorBoard."""
    def __init__(self, window_size: int = 100, verbose: int = 0):
        super().__init__(verbose)
        self.p1_wins = deque(maxlen=window_size)

    def _on_step(self) -> bool:
        for info in self.locals.get("infos", []):
            if "winner" in info:
                t_info = info.get("terminal_info", info)
                p1_win = 1 if t_info.get("winner") == 0 else 0
                self.p1_wins.append(p1_win)
                self.logger.record("rollout/p1_win_rate", np.mean(self.p1_wins))
                
                # Check terminal_observation from VecMonitor if present
                vps = t_info.get("vps")
                if vps is None and "terminal_observation" in info:
                    # If SB3 wrapped the terminal state, we might need to look harder
                    pass 
                
                if vps:
                    print(f"DEBUG: Game Finished. VPs: {vps}. P1 Win: {p1_win}")
        
        return True

class CurriculumCallback(BaseCallback):
    """Callback to phase out dense rewards (VP/Income pulses) only after hitting performance targets."""
    def __init__(self, state: CurriculumState, players: int, window_size: int = 1000, verbose: int = 0):
        super().__init__(verbose)
        self.state = state
        # Target: 4 VP per TOTAL action in the game.
        # 2p: 40 actions * 4 = 160.
        # 3p: 36 actions * 4 = 144.
        # 4p: 32 actions * 4 = 128.
        actions_per_game = {2: 40, 3: 36, 4: 32}
        self.target_vp = actions_per_game.get(players, 40) * 3.5
        
        self.vp_history = deque(maxlen=window_size)
        self.current_reward_scale = 1.0

    def _on_step(self) -> bool:
        # Update current step count in shared state
        self.state.num_timesteps = self.num_timesteps

        # Track episode ends for VP rolling average
        for info in self.locals.get("infos", []):
            if "winner" in info:
                t_info = info.get("terminal_info", info)
                if "vps" in t_info:
                    self.vp_history.append(np.mean(t_info["vps"]))

        if not self.state.is_decaying:
            if len(self.vp_history) >= self.vp_history.maxlen:
                avg_vp = np.mean(self.vp_history)
                if avg_vp >= self.target_vp:
                    self.state.is_decaying = True
                    self.state.trigger_step = self.num_timesteps
                    print(f"\nDEBUG: Performance target {self.target_vp} reached! (Avg: {avg_vp:.1f})")
                    print(f"DEBUG: Starting Curriculum Decay Phase at step {self.state.trigger_step}.\n")

        if self.state.is_decaying:
            elapsed = self.num_timesteps - self.state.trigger_step
            progress = max(0, 1.0 - (elapsed / self.state.decay_steps))
            self.current_reward_scale = 0.1 + (0.9 * progress)
        else:
            self.current_reward_scale = 1.0
        
        self.training_env.env_method("set_reward_scale", self.current_reward_scale)
        
        self.logger.record("train/dense_reward_scale", self.current_reward_scale)
        self.logger.record("train/is_decaying", int(self.state.is_decaying))
        if len(self.vp_history) > 0:
            self.logger.record("train/rolling_avg_vp", np.mean(self.vp_history))
        return True


class MultiPhaseCurriculumCallback(BaseCallback):
    def __init__(self, state: CurriculumState, total_envs: int, patience: int, fallback_steps: int, diagnostic_callback=None, verbose: int = 0):
        super().__init__(verbose)
        self.state = state
        self.total_envs = total_envs
        self.patience = patience
        self.fallback_steps = fallback_steps
        self.diagnostic_callback = diagnostic_callback
        
        # Phase definitions: (p2_frac, p3_frac, p4_frac)
        self.phases = [
            (1.0, 0.0, 0.0),  # Phase 1: 100% 2p
            (0.75, 0.25, 0.0), # Phase 2: 75% 2p, 25% 3p
            (0.20, 0.80, 0.0), # Phase 3: 20% 2p, 80% 3p
            (0.40, 0.40, 0.20),# Phase 4: 40% 2p, 40% 3p, 20% 4p
            (0.20, 0.20, 0.60),# Phase 5: 20% 2p, 20% 3p, 60% 4p
            (0.33, 0.33, 0.34),# Phase 6: 33% 2p, 33% 3p, 33% 4p (approx)
        ]
        self.current_phase = 0
        self.steps_in_phase = 0
        self.patience_counter = 0
        
        from config import ACTIONS_PER_PLAYER, VP_MULTIPLIER
        self.actions_per_player = ACTIONS_PER_PLAYER
        self.vp_multiplier = VP_MULTIPLIER
        
        # History for VPs per player count
        self.vp_history = {2: deque(maxlen=100), 3: deque(maxlen=100), 4: deque(maxlen=100)}
        
    def _on_training_start(self) -> None:
        # Initialize first phase
        self._apply_phase_distribution(self.current_phase)
        
    def _apply_phase_distribution(self, phase_idx: int):
        p2, p3, p4 = self.phases[phase_idx]
        n2 = int(self.total_envs * p2)
        n3 = int(self.total_envs * p3)
        n4 = self.total_envs - n2 - n3 # Remainder
        
        # Assign to workers
        idx = 0
        for _ in range(n2):
            self.training_env.env_method("set_num_players", 2, indices=[idx])
            idx += 1
        for _ in range(n3):
            self.training_env.env_method("set_num_players", 3, indices=[idx])
            idx += 1
        for _ in range(n4):
            self.training_env.env_method("set_num_players", 4, indices=[idx])
            idx += 1
            
        print(f"\nDEBUG: Switched to Phase {phase_idx + 1} distribution: 2p={n2}, 3p={n3}, 4p={n4}\n")
        
    def _on_step(self) -> bool:
        self.state.num_timesteps = self.num_timesteps
        self.steps_in_phase += self.training_env.num_envs
        
        # Track episode ends for VP
        for info in self.locals.get("infos", []):
            if "winner" in info:
                t_info = info.get("terminal_info", info)
                num_players = t_info.get("num_players")
                vps = t_info.get("vps")
                if num_players and vps:
                    avg_vp = np.mean(vps)
                    self.vp_history[num_players].append(avg_vp)
                    
        # Determine dominant player count for current phase to check threshold
        p2, p3, p4 = self.phases[self.current_phase]
        dom_players = 2
        if p3 > p2 and p3 > p4: dom_players = 3
        if p4 > p2 and p4 > p3: dom_players = 4
        
        target_vp = self.actions_per_player[dom_players] * self.vp_multiplier
        
        # Check threshold for dominant player count
        met_threshold = False
        if len(self.vp_history[dom_players]) >= 10: # Need some history
            avg_vp = np.mean(self.vp_history[dom_players])
            if avg_vp >= target_vp:
                met_threshold = True
                
        if met_threshold:
            self.patience_counter += self.training_env.num_envs
        else:
            self.patience_counter = 0 # Reset if drops below
            
        # Check for transition
        should_transition = False
        if self.patience_counter >= self.patience:
            print(f"\nDEBUG: Phase {self.current_phase + 1} target reached! Stability maintained.")
            should_transition = True
        elif self.steps_in_phase >= self.fallback_steps and self.current_phase < len(self.phases) - 1:
            print(f"\nDEBUG: Phase {self.current_phase + 1} fallback steps reached.")
            should_transition = True
            
        if should_transition and self.current_phase < len(self.phases) - 1:
            self.current_phase += 1
            self.steps_in_phase = 0
            self.patience_counter = 0
            self._apply_phase_distribution(self.current_phase)
            
            # Update diagnostics player count
            if self.diagnostic_callback:
                self.diagnostic_callback.set_num_players(dom_players)
            
        # Log metrics
        self.logger.record("train/phase", self.current_phase + 1)
        self.logger.record("train/steps_in_phase", self.steps_in_phase)
        for k, v in self.vp_history.items():
            if len(v) > 0:
                self.logger.record(f"train/avg_vp_{k}p", np.mean(v))
                
        return True

# --- Schedulers moved to config.py ---

# --- Diagnostics moved to utils.py ---

# --- Main Logic ---

def main() -> None:
    try:
        multiprocessing.set_start_method('spawn', force=True)
    except RuntimeError:
        pass # Already set
        
    import torch as th
    from sb3_contrib import MaskablePPO
    args = get_args()

    # Setup Directory Structure
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    run_id = args.run_name if args.run_name else f"ppo_{timestamp}"
    run_dir = Path(__file__).parent / "runs" / run_id
    run_dir.mkdir(parents=True, exist_ok=True)
    
    checkpoint_dir = run_dir / "checkpoints"
    tensorboard_dir = run_dir / "tb_logs"

    # Removed server launch

    try:
        print(f"Spinning up {args.envs} parallel environments...")
        env_fns = [make_env_fn(i, args.players) for i in range(args.envs)]
        vec_env = SubprocVecEnv(env_fns)
        vec_env = VecMonitor(vec_env)

        # Query board size from environment
        board_size = vec_env.get_attr('board_size')[0]
        print(f"Queried board size: {board_size}")

        if args.use_transformer:
            policy_kwargs = dict(
                features_extractor_class=CardAttentionExtractor,
                features_extractor_kwargs=dict(hand_size=8, card_width=24),
                net_arch=NET_ARCH,
            )
        else:
            policy_kwargs = dict(
                net_arch=NET_ARCH,
            )

        # n_steps = 1024 (rollout length, total batch = n_steps × n_envs)
        
        # Instantiate Callbacks
        diagnostic_callback = DiagnosticCallback(
            save_freq=max(250_000 // args.envs, 1),
            num_episodes=25,
            log_file=str(run_dir / "diagnostics.log"),
            num_players=args.players
        )

        # Curriculum Tracking (phases out dense rewards after performance threshold)
        curriculum_state = CurriculumState(decay_steps=5_000_000)
        if args.multi_phase:
            curriculum_callback = MultiPhaseCurriculumCallback(
                state=curriculum_state, 
                total_envs=args.envs, 
                patience=args.patience, 
                fallback_steps=args.fallback_steps,
                diagnostic_callback=diagnostic_callback
            )
        else:
            curriculum_callback = CurriculumCallback(state=curriculum_state, players=args.players)
        lr_schedule = DynamicLRScheduler(args.lr, curriculum_state)
        win_rate_callback = PositionWinRateCallback(window_size=100)

        if args.load:
            print(f"Loading existing model from: {args.load}")
            # Load the saved model to get its weights (uses its saved architecture)
            loaded_model = MaskablePPO.load(args.load)
            
            print("Creating new model with target architecture...")
            model = MaskablePPO(
                "MlpPolicy",
                vec_env,
                n_steps=PPO_N_STEPS,
                batch_size=PPO_BATCH_SIZE,
                n_epochs=PPO_N_EPOCHS,
                gamma=PPO_GAMMA,
                gae_lambda=PPO_GAE_LAMBDA,
                clip_range=PPO_CLIP_RANGE,
                target_kl=PPO_TARGET_KL,
                ent_coef=PPO_ENT_COEF,
                learning_rate=lr_schedule,
                policy_kwargs=policy_kwargs,
                verbose=1,
                device="cuda" if th.cuda.is_available() else "cpu",
                tensorboard_log=str(tensorboard_dir),
            )
            
            print("Transferring weights (non-strict)...")
            model.policy.load_state_dict(loaded_model.policy.state_dict(), strict=False)
            del loaded_model
        else:
            print("Creating new PPO model.")
            model = MaskablePPO(
                "MlpPolicy",
                vec_env,
                n_steps=PPO_N_STEPS,
                batch_size=PPO_BATCH_SIZE,
                n_epochs=PPO_N_EPOCHS,
                gamma=PPO_GAMMA,
                gae_lambda=PPO_GAE_LAMBDA,
                clip_range=PPO_CLIP_RANGE,
                target_kl=PPO_TARGET_KL,
                ent_coef=PPO_ENT_COEF,
                learning_rate=args.lr,
                policy_kwargs=policy_kwargs,
                verbose=1,
                device="cuda" if th.cuda.is_available() else "cpu",
                tensorboard_log=str(tensorboard_dir),
            )

        print(f"Starting training: {args.steps:,} total timesteps")
        print(f"  obs_size={model.observation_space.shape[0]}")
        print(f"  action_size={model.action_space.n}")
        print(f"  batch={args.envs * args.n_steps} (n_envs × args.n_steps)")
        print(f"  run_dir={run_dir}")
        print()

        # Save a checkpoint every 250k steps
        checkpoint_callback = CheckpointCallback(
            save_freq=max(250_000 // args.envs, 1),
            save_path=str(checkpoint_dir),
            name_prefix="brass_ppo",
            save_replay_buffer=False,
            save_vecnormalize=True,
        )



        model.learn(
            total_timesteps=args.steps,
            progress_bar=False,
            callback=[checkpoint_callback, win_rate_callback, curriculum_callback, diagnostic_callback],
            reset_num_timesteps=False if args.load else True,
        )

        final_save_path = run_dir / "brass_ppo_final"
        model.save(str(final_save_path))
        print(f"\nTraining complete. Model saved to {final_save_path}.zip")

        vec_env.close()

    finally:
        pass


if __name__ == "__main__":
    main()
