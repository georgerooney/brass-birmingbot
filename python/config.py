import argparse

class CurriculumState:
    """Simple picklable state for curriculum progress.
    Separated from the Callback to avoid serializing the environment/processes.
    """
    def __init__(self, decay_steps: int = 5_000_000):
        self.is_decaying = False
        self.trigger_step = 0
        self.decay_steps = decay_steps
        self.num_timesteps = 0

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

def get_args():
    parser = argparse.ArgumentParser(description="Train Brass Birmingham PPO agent")
    parser.add_argument("--envs",    type=int,   default=32,      help="Parallel envs")
    parser.add_argument("--steps",   type=int,   default=10_000_000, help="Total timesteps")
    parser.add_argument("--lr",      type=float, default=0.0003,      help="Learning rate (default: 3e-4)")
    parser.add_argument("--batch-size", type=int, default=4096,     help="Minibatch size")
    parser.add_argument("--n-steps", type=int,   default=1024,     help="Rollout steps per env")
    parser.add_argument("--players", type=int,   default=2,       help="Players per game")
    parser.add_argument("--load",    type=str,   default=None,    help="Path to existing .zip model to resume from")
    parser.add_argument("--run-name", type=str,  default=None,    help="Custom name for this run")
    parser.add_argument("--use-transformer", action="store_true", help="Use card attention transformer")
    parser.add_argument("--no-server", action="store_true", help="Skip server launch")
    parser.add_argument("--patience", type=int, default=200_000, help="Steps to maintain threshold")
    parser.add_argument("--fallback-steps", type=int, default=2_000_000, help="Max steps per phase")
    parser.add_argument("--multi-phase", action="store_true", help="Use multi-phase curriculum")
    return parser.parse_args()

# --- Hyperparameters ---
PPO_N_STEPS = 256
PPO_BATCH_SIZE = 256
PPO_N_EPOCHS = 4
PPO_GAMMA = 0.99
PPO_GAE_LAMBDA = 0.95
PPO_CLIP_RANGE = 0.2
PPO_TARGET_KL = 0.03
PPO_ENT_COEF = 0.05

# --- Curriculum Hyperparameters ---
ACTIONS_PER_PLAYER = {2: 40, 3: 36, 4: 32}
VP_MULTIPLIER = 3.5

# --- Network Architecture ---
NET_ARCH = dict(pi=[512, 256], vf=[512, 256])
