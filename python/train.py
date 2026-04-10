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
from collections import deque
from datetime import datetime
from pathlib import Path

import numpy as np
import torch as th
import torch.nn as nn
from sb3_contrib import MaskablePPO
from sb3_contrib.common.maskable.policies import MaskableActorCriticPolicy
from stable_baselines3.common import utils
from stable_baselines3.common.torch_layers import BaseFeaturesExtractor
from stable_baselines3.common.callbacks import CheckpointCallback, BaseCallback
from stable_baselines3.common.vec_env import SubprocVecEnv, VecMonitor

# --- Data Structures ---

class CurriculumState:
    """Simple picklable state for curriculum progress.
    Separated from the Callback to avoid serializing the environment/processes.
    """
    def __init__(self, decay_steps: int = 5_000_000):
        self.is_decaying = False
        self.trigger_step = 0
        self.decay_steps = decay_steps
        self.num_timesteps = 0

ROOT = Path(__file__).resolve().parent.parent  # d:\projects\brass
from brass_env.server import ensure_server

# --- Global Helpers ---

def make_env_fn(rank: int, num_players: int):
    """Factory that SubprocVecEnv calls in each worker subprocess."""
    def _init():
        from brass_env import BrassEnv
        env = BrassEnv(num_players=num_players)
        return env
    return _init

# --- Neural Architecture v2.0 (Expert Compatibility Matrix) ---

class BrassExpertExtractor(BaseFeaturesExtractor):
    """Separates the flat observation into Board and Hand components."""
    def __init__(self, observation_space, features_dim: int = 1024):
        super().__init__(observation_space, features_dim)
        # Slices (must match engine/observation.go)
        self.board_size = 2204
        
        # Board Encoder: Legacy 1024-dimensional capacity
        self.board_encoder = nn.Sequential(
            nn.Linear(self.board_size, 1024),
            nn.ReLU(),
            nn.Linear(1024, features_dim),
            nn.ReLU(),
        )

    def forward(self, observations: th.Tensor) -> th.Tensor:
        # Narrow focus: strategic board state only
        board_obs = observations[:, :self.board_size]
        return self.board_encoder(board_obs)

class BrassExpertPolicy(MaskableActorCriticPolicy):
    """Simplified policy head for purely strategic actions."""
    def _get_action_logits(self, latent_pi: th.Tensor) -> th.Tensor:
        # Standard linear head for the flattened 886 strategic action space
        return self.action_net(latent_pi)

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        # Re-allocate the action_net to match our new 886 strategic action space
        self.action_net = nn.Linear(self.mlp_extractor.latent_dim_pi, 886)
        
        # Orthogonal init for stable start
        nn.init.orthogonal_(self.action_net.weight, gain=0.01)
        nn.init.constant_(self.action_net.bias, 0)

    def forward(self, obs: th.Tensor, deterministic: bool = False, action_masks: th.Tensor = None):
        features = self.extract_features(obs)
        latent_pi, latent_vf = self.mlp_extractor(features)
        
        # Generate distribution (which applies our dot-product logic)
        distribution = self._get_action_distribution_from_latent(latent_pi)
        if action_masks is not None:
            distribution.apply_masking(action_masks)
            
        # Values from the standard critic branch
        values = self.value_net(latent_vf)
        
        # Sample actions
        actions = distribution.get_actions(deterministic=deterministic)
        log_prob = distribution.log_prob(actions)
        
        return actions, values, log_prob

    def _get_action_distribution_from_latent(self, latent_pi: th.Tensor):
        # latent_pi is the output of the mlp_extractor (pi branch)
        # For simplicity, we assume the mlp_extractor PI branch preserves our feature layout
        # or we just extract from features directly since the board context is what matters.
        logits = self._get_action_logits(latent_pi) # If PI branch = Identity, this works
        return self.action_dist.proba_distribution(action_logits=logits)

# --- Callbacks ---

class PositionWinRateCallback(BaseCallback):
    """Logs the win rate of Player 1 to TensorBoard."""
    def __init__(self, window_size: int = 100, verbose: int = 0):
        super().__init__(verbose)
        self.p1_wins = deque(maxlen=window_size)

    def _on_step(self) -> bool:
        for info in self.locals.get("infos", []):
            if "episode" in info and "winner" in info:
                # v2.5: check persistent terminal info for auto-resetting VecEnvs
                t_info = info.get("terminal_info", info)
                p1_win = 1 if t_info.get("winner") == 0 else 0
                self.p1_wins.append(p1_win)
                
                # Record immediately to ensure it shows up in the SB3 table
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
        self.target_vp = actions_per_game.get(players, 40) * 4
        
        self.vp_history = deque(maxlen=window_size)
        self.current_reward_scale = 1.0

    def _on_step(self) -> bool:
        # Update current step count in shared state
        self.state.num_timesteps = self.num_timesteps

        # Track episode ends for VP rolling average
        for info in self.locals.get("infos", []):
            if "episode" in info:
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

class DynamicLRScheduler:
    """Delayed LR decay based on curriculum trigger.
    Defined as a class to be picklable without capturing the main() local scope.
    """
    def __init__(self, initial_lr: float, state: CurriculumState):
        self.initial_lr = initial_lr
        self.state = state

    def __call__(self, progress_remaining: float) -> float:
        if not self.state.is_decaying:
            return self.initial_lr
            
        elapsed = self.state.num_timesteps - self.state.trigger_step
        decay_progress = max(0, 1.0 - (elapsed / self.state.decay_steps))
        return self.initial_lr * decay_progress

# --- Main Logic ---

def main() -> None:
    parser = argparse.ArgumentParser(description="Train Brass Birmingham PPO agent")
    parser.add_argument("--envs",    type=int,   default=32,      help="Parallel envs")
    parser.add_argument("--steps",   type=int,   default=10_000_000, help="Total timesteps")
    parser.add_argument("--lr", type=float, default=0.0005, help="Learning rate (default: 5e-4)")
    parser.add_argument("--players", type=int,   default=2,       help="Players per game")
    parser.add_argument("--load",    type=str,   default=None,    help="Path to existing .zip model to resume from")
    parser.add_argument("--run-name", type=str,  default=None,    help="Custom name for this run")
    parser.add_argument("--no-server", action="store_true", help="Skip server launch")
    args = parser.parse_args()

    # Setup Directory Structure
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    run_id = args.run_name if args.run_name else f"ppo_{timestamp}"
    run_dir = Path(__file__).parent / "runs" / run_id
    run_dir.mkdir(parents=True, exist_ok=True)
    
    checkpoint_dir = run_dir / "checkpoints"
    tensorboard_dir = run_dir / "tb_logs"

    server_proc = None
    if not args.no_server:
        server_proc = ensure_server(ROOT)

    try:
        print(f"Spinning up {args.envs} parallel environments...")
        env_fns = [make_env_fn(i, args.players) for i in range(args.envs)]
        vec_env = SubprocVecEnv(env_fns)
        vec_env = VecMonitor(vec_env)

        # Simplified Network configuration (Legacy 1024)
        features_dim = 512
        policy_kwargs = dict(
            features_extractor_class=BrassExpertExtractor,
            features_extractor_kwargs=dict(features_dim=features_dim),
            net_arch=dict(pi=[], vf=[512, 256]), 
        )

        n_steps = 256  # rollout length (total batch = n_steps × n_envs)
        
        # Instantiate Callbacks
        # Curriculum Tracking (phases out dense rewards after performance threshold)
        curriculum_state = CurriculumState(decay_steps=5_000_000)
        curriculum_callback = CurriculumCallback(state=curriculum_state, players=args.players)
        lr_schedule = DynamicLRScheduler(args.lr, curriculum_state)
        win_rate_callback = PositionWinRateCallback(window_size=100)

        if args.load:
            print(f"Loading existing model from: {args.load}")
            model = MaskablePPO.load(
                args.load,
                env=vec_env,
                learning_rate=lr_schedule,
                policy_kwargs=policy_kwargs,
                verbose=1,
                tensorboard_log=str(tensorboard_dir),
            )
        else:
            print("Creating new PPO model.")
            model = MaskablePPO(
                BrassExpertPolicy,
                vec_env,
                n_steps=n_steps,
                batch_size=256,        # Tighter batch for faster convergence
                n_epochs=10,           
                gamma=0.99,            # Strategic focus (reverted from 0.997)
                gae_lambda=0.98,       
                clip_range=0.2,
                target_kl=0.015,       
                ent_coef=0.01,         # Tighter entropy for smaller space
                learning_rate=lr_schedule,
                policy_kwargs=policy_kwargs,
                verbose=1,
                device="cuda",
                tensorboard_log=str(tensorboard_dir),
            )

        print(f"Starting training: {args.steps:,} total timesteps")
        print(f"  obs_size={model.observation_space.shape[0]}")
        print(f"  action_size={model.action_space.n}")
        print(f"  batch={args.envs * n_steps} (n_envs × n_steps)")
        print(f"  run_dir={run_dir}")
        print()

        # Save a checkpoint every 50k steps
        checkpoint_callback = CheckpointCallback(
            save_freq=max(50_000 // args.envs, 1),
            save_path=str(checkpoint_dir),
            name_prefix="brass_ppo",
            save_replay_buffer=False,
            save_vecnormalize=True,
        )

        model.learn(
            total_timesteps=args.steps,
            progress_bar=False,
            callback=[checkpoint_callback, win_rate_callback, curriculum_callback],
            reset_num_timesteps=False if args.load else True,
        )

        final_save_path = run_dir / "brass_ppo_final"
        model.save(str(final_save_path))
        print(f"\nTraining complete. Model saved to {final_save_path}.zip")

        vec_env.close()

    finally:
        if server_proc is not None:
            server_proc.terminate()
            server_proc.wait()
            print("Server stopped.")


if __name__ == "__main__":
    main()
