import os
import multiprocessing
import psutil
from collections import defaultdict
import numpy as np
import torch as th
from stable_baselines3.common.callbacks import BaseCallback
from sb3_contrib import MaskablePPO
from brass_env.env import BrassEnv


def run_diagnostics(
    model_path: str, num_episodes: int, log_file: str, train_steps: int
):
    try:
        env = BrassEnv(num_players=2)
        model = MaskablePPO.load(model_path)

        move_types = defaultdict(int)
        specific_moves = defaultdict(int)

        action_names = env.get_action_names()

        total_steps = 0
        valid_link_count = 0
        total_valid_link_prob = 0.0
        steps_with_links = 0

        for _ in range(num_episodes):
            obs, info = env.reset()
            done = False
            while not done:
                masks = env.action_masks()
                valid_actions = [i for i, m in enumerate(masks) if m]
                for a in valid_actions:
                    if action_names[a].startswith("Network"):
                        valid_link_count += 1

                # Extract probabilities for links
                obs_tensor = (
                    th.tensor(obs, dtype=th.float32).unsqueeze(0).to(model.device)
                )
                with th.no_grad():
                    distribution = model.policy.get_distribution(obs_tensor)
                    probs = distribution.distribution.probs.detach().cpu().numpy()[0]

                link_indices = [
                    i
                    for i, name in enumerate(action_names)
                    if name.startswith("Network")
                ]
                valid_link_indices = [i for i in link_indices if masks[i]]

                if len(valid_link_indices) > 0:
                    total_valid_link_prob += np.sum(probs[valid_link_indices])
                    steps_with_links += 1

                # Manually sample based on policy to guarantee mask application
                masked_probs = probs * masks
                if np.sum(masked_probs) > 0:
                    masked_probs = masked_probs / np.sum(masked_probs)
                    action = np.random.choice(len(masked_probs), p=masked_probs)
                else:
                    action, _ = model.predict(
                        obs, action_masks=masks, deterministic=False
                    )

                action_name = action_names[action]
                specific_moves[action_name] += 1

                if action_name.startswith("Network (Double)"):
                    move_type = "Network (Double)"
                elif action_name.startswith("Network"):
                    move_type = "Network"
                else:
                    move_type = action_name.split()[0]
                move_types[move_type] += 1

                obs, reward, terminated, truncated, info = env.step(action)
                done = terminated or truncated
                total_steps += 1

        sorted_types = sorted(move_types.items(), key=lambda x: x[1], reverse=True)
        sorted_specific = sorted(
            specific_moves.items(), key=lambda x: x[1], reverse=True
        )[:10]

        with open(log_file, "a") as f:
            f.write(f"\n--- Diagnostics at training step {train_steps} ---\n")
            f.write(f"Total moves in {num_episodes} games: {total_steps}\n")
            f.write(
                f"Average valid links available per step: {valid_link_count/total_steps:.2f}\n"
            )
            avg_link_prob = (
                total_valid_link_prob / steps_with_links
                if steps_with_links > 0
                else 0.0
            )
            f.write(f"Average policy prob of valid links: {avg_link_prob:.4f}\n")
            f.write("Most common move types:\n")
            for t, c in sorted_types:
                f.write(f"  {t}: {c} ({c/total_steps:.2%})\n")
            f.write("Top 10 specific moves:\n")
            for m, c in sorted_specific:
                f.write(f"  {m}: {c} ({c/total_steps:.2%})\n")
            f.write("-" * 40 + "\n")

        # Clean up temp model file
        os.remove(model_path)

    except Exception as e:
        with open(log_file, "a") as f:
            f.write(f"Error in diagnostics at step {train_steps}: {str(e)}\n")


class DiagnosticCallback(BaseCallback):
    def __init__(
        self, save_freq: int, num_episodes: int, log_file: str, verbose: int = 0
    ):
        super().__init__(verbose)
        self.save_freq = save_freq
        self.num_episodes = num_episodes
        self.log_file = log_file

    def _on_step(self) -> bool:
        if self.n_calls % self.save_freq == 0:
            run_dir = os.path.dirname(self.log_file)
            temp_path = os.path.join(
                run_dir, f"temp_diag_model_{self.num_timesteps}.zip"
            )
            self.model.save(temp_path)

            p = multiprocessing.Process(
                target=run_diagnostics,
                args=(temp_path, self.num_episodes, self.log_file, self.num_timesteps),
            )
            p.start()

        return True


class ProfilingCallback(BaseCallback):
    def __init__(self, freq: int = 100, verbose: int = 0):
        super().__init__(verbose)
        self.freq = freq
        self.process = psutil.Process(os.getpid())
        self.process.cpu_percent(interval=None)
        psutil.cpu_percent(interval=None)

    def _on_step(self) -> bool:
        if self.n_calls == 1:
            self.logger.record("hparams/batch_size", self.model.batch_size)
            self.logger.record("hparams/n_steps", self.model.n_steps)
            self.logger.record("hparams/n_epochs", self.model.n_epochs)
            self.logger.record("hparams/gamma", self.model.gamma)
            if isinstance(self.model.learning_rate, float):
                self.logger.record("hparams/learning_rate", self.model.learning_rate)

        if self.n_calls % self.freq == 0:
            self.logger.record(
                "system/cpu_percent", psutil.cpu_percent(interval=None)
            )
            self.logger.record("system/ram_percent", psutil.virtual_memory().percent)

            self.logger.record(
                "process/cpu_percent", self.process.cpu_percent(interval=None)
            )
            self.logger.record("process/ram_percent", self.process.memory_percent())

            if th.cuda.is_available():
                self.logger.record(
                    "system/gpu_vram_allocated_mb",
                    th.cuda.memory_allocated() / 1024 / 1024,
                )
                self.logger.record(
                    "system/gpu_vram_reserved_mb",
                    th.cuda.memory_reserved() / 1024 / 1024,
                )

        return True


class GCSCheckpointCallback(BaseCallback):
    def __init__(
        self,
        save_freq: int,
        save_path: str,
        name_prefix: str,
        bucket_name: str,
        save_vecnormalize: bool = False,
        verbose: int = 0,
    ):
        super().__init__(verbose)
        self.save_freq = save_freq
        self.save_path = save_path
        self.name_prefix = name_prefix
        self.bucket_name = bucket_name
        self.save_vecnormalize = save_vecnormalize

    def _on_step(self) -> bool:
        if self.n_calls % self.save_freq == 0:
            path = os.path.join(
                self.save_path,
                f"{self.name_prefix}_{self.num_timesteps}_steps.zip",
            )
            self.model.save(path)
            print(f"Saved checkpoint to {path}")

            from gcs_utils import upload_file

            gcs_path = f"checkpoints/{os.path.basename(path)}"
            p = multiprocessing.Process(
                target=upload_file, args=(path, self.bucket_name, gcs_path)
            )
            p.start()

            if self.save_vecnormalize and self.training_env is not None:
                from stable_baselines3.common.vec_env import VecNormalize

                if isinstance(self.training_env, VecNormalize):
                    vec_normalize_path = os.path.join(
                        self.save_path,
                        f"{self.name_prefix}_vecnormalize_{self.num_timesteps}.pkl",
                    )
                    self.training_env.save(vec_normalize_path)
                    print(f"Saved VecNormalize to {vec_normalize_path}")

                    gcs_vn_path = (
                        f"checkpoints/{os.path.basename(vec_normalize_path)}"
                    )
                    p_vn = multiprocessing.Process(
                        target=upload_file,
                        args=(vec_normalize_path, self.bucket_name, gcs_vn_path),
                    )
                    p_vn.start()

        return True
