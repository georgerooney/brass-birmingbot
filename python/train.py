"""
train.py — Brass Birmingham PPO training entry point.

Uses MaskablePPO from sb3-contrib so the policy never selects an illegal action.
The Go engine server is started automatically as a subprocess.

Usage:
    cd d:\\projects\\brass\\python
    uv run train.py [--envs N] [--steps N] [--lr LR]
"""

from __future__ import annotations
from collections import deque
from datetime import datetime
from pathlib import Path
import multiprocessing
from dotenv import load_dotenv

import numpy as np

# Base SB3 classes are okay at top-level; the heavy model/torch logic is moved inside main()
from stable_baselines3.common.callbacks import CheckpointCallback, BaseCallback
from stable_baselines3.common.vec_env import SubprocVecEnv, VecMonitor
from config import (
    CurriculumState,
    DynamicLRScheduler,
    get_args,
    PPO_N_STEPS,
    PPO_BATCH_SIZE,
    PPO_N_EPOCHS,
    PPO_GAMMA,
    PPO_GAE_LAMBDA,
    PPO_CLIP_RANGE,
    PPO_TARGET_KL,
    PPO_ENT_COEF,
    NET_ARCH,
)
from utils import DiagnosticCallback, ProfilingCallback, GCSCheckpointCallback
import torch as th
import torch.nn as nn
from stable_baselines3.common.torch_layers import BaseFeaturesExtractor

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

        # Output dim is total_dim to match old model shapes!
        super().__init__(observation_space, total_dim)

        self.hand_size = hand_size
        self.card_width = card_width
        self.hand_dim = hand_dim

        # Small transformer for cards
        self.card_embed = nn.Linear(card_width, 16)
        encoder_layer = nn.TransformerEncoderLayer(
            d_model=16, nhead=1, dim_feedforward=32, batch_first=True
        )
        self.transformer = nn.TransformerEncoder(encoder_layer, num_layers=1)

        # Project back to hand_dim so total output dim is board_dim + hand_dim = total_dim
        self.card_expand = nn.Linear(16, hand_dim)

    def forward(self, observations: th.Tensor) -> th.Tensor:
        board_obs = observations[:, : -self.hand_dim]
        hand_obs = observations[:, -self.hand_dim :]

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

    def __init__(
        self,
        state: CurriculumState,
        players: int,
        window_size: int = 1000,
        verbose: int = 0,
    ):
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
                    print(
                        f"\nDEBUG: Performance target {self.target_vp} reached! (Avg: {avg_vp:.1f})"
                    )
                    print(
                        f"DEBUG: Starting Curriculum Decay Phase at step {self.state.trigger_step}.\n"
                    )

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


# --- Schedulers moved to config.py ---

# --- Diagnostics moved to utils.py ---

# --- Main Logic ---


def main() -> None:
    try:
        multiprocessing.set_start_method("spawn", force=True)
    except RuntimeError:
        pass  # Already set

    import torch as th
    from sb3_contrib import MaskablePPO

    load_dotenv(Path(__file__).parent.parent / ".env.local")
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
        # Curriculum Tracking (phases out dense rewards after performance threshold)
        curriculum_state = CurriculumState(decay_steps=5_000_000)
        curriculum_callback = CurriculumCallback(
            state=curriculum_state, players=args.players
        )
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
        import os
        bucket_name = os.environ.get("GCS_BUCKET_NAME")
        if bucket_name:
            print(f"GCS Integration enabled. Bucket: {bucket_name}")
            checkpoint_callback = GCSCheckpointCallback(
                save_freq=max(250_000 // args.envs, 1),
                save_path=str(checkpoint_dir),
                name_prefix="brass_ppo",
                bucket_name=bucket_name,
                save_vecnormalize=True,
            )
        else:
            print("GCS Integration disabled (GCS_BUCKET_NAME not set in .env.local).")
            checkpoint_callback = CheckpointCallback(
                save_freq=max(250_000 // args.envs, 1),
                save_path=str(checkpoint_dir),
                name_prefix="brass_ppo",
                save_replay_buffer=False,
                save_vecnormalize=True,
            )

        diagnostic_callback = DiagnosticCallback(
            save_freq=max(250_000 // args.envs, 1),
            num_episodes=25,
            log_file=str(run_dir / "diagnostics.log"),
        )

        profiling_callback = ProfilingCallback(freq=100)

        model.learn(
            total_timesteps=args.steps,
            progress_bar=False,
            callback=[
                checkpoint_callback,
                win_rate_callback,
                curriculum_callback,
                diagnostic_callback,
                profiling_callback,
            ],
            reset_num_timesteps=False if args.load else True,
        )

        final_save_path = run_dir / "brass_ppo_final"
        model.save(str(final_save_path))
        print(f"\nTraining complete. Model saved to {final_save_path}.zip")

        # Upload the entire run directory to GCS before exiting
        if bucket_name:
            print(f"Uploading run directory {run_dir} to GCS...")
            from gcs_utils import upload_directory
            upload_directory(str(run_dir), bucket_name, f"runs/{run_id}")

        vec_env.close()

    finally:
        pass


if __name__ == "__main__":
    main()
