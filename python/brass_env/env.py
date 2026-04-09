"""
Gymnasium environment wrapping the Brass Birmingham Go engine server.

The Go server communicates over HTTP/JSON (no CGO required).
Observations and action masks are transferred as base64-encoded binary
to minimise serialisation overhead.

Usage:
    env = BrassEnv(num_players=2)      # connects to localhost:8765
    obs, info = env.reset()
    obs, reward, terminated, truncated, info = env.step(action)
    mask = env.action_masks()          # required by MaskablePPO
"""

from __future__ import annotations

import base64
import struct
import time

import gymnasium as gym
import numpy as np
import requests

# Must match engine.ObsTotalSize (observation.go)
_OBS_SIZE = 2396


def _decode_obs(b64: str) -> np.ndarray:
    """Decode base64 → LE float32 array without copy."""
    raw = base64.b64decode(b64)
    return np.frombuffer(raw, dtype="<f4").astype(np.float32, copy=False)


def _decode_mask(b64: str, n: int) -> np.ndarray:
    """Decode base64 bit-packed mask (LSB first) → bool array of length n."""
    raw = np.frombuffer(base64.b64decode(b64), dtype=np.uint8)
    bits = np.unpackbits(raw, bitorder="little")
    return bits[:n].astype(np.bool_)


class BrassEnv(gym.Env):
    """
    Single-player-perspective Gymnasium wrapper for Brass Birmingham.

    The environment connects to a running brass_engine server.
    Start the server first:
        go run ./server   (from f:\\Projects\\brass)
    or use train.py which handles the subprocess automatically.

    Action masking:
        env.action_masks() returns a bool np.ndarray consumed by
        sb3_contrib.MaskablePPO.
    """

    metadata = {"render_modes": []}

    def __init__(
        self,
        num_players: int = 2,
        server_url: str = "http://localhost:8765",
        timeout: float = 10.0,
    ) -> None:
        super().__init__()
        self.num_players = num_players
        self.server_url = server_url.rstrip("/")
        self._env_id: int | None = None
        self._action_size: int | None = None
        self._action_names: list[str] | None = None

        # Use a session for persistent connections (Keep-Alive) to prevent 
        # socket exhaustion (WinError 10048) during high-FPS training.
        self.session = requests.Session()

        # Wait for the server to be ready (handles subprocess startup delay)
        self._wait_for_server(timeout)

        # Query static dimensions from server
        info = self.session.get(f"{self.server_url}/health").json()
        obs_size = info["obs_size"]
        action_size = info["action_size"]

        assert obs_size == _OBS_SIZE, (
            f"Server obs_size {obs_size} != expected {_OBS_SIZE}. "
            "Rebuild the Go server after changing ObsTotalSize."
        )
        self._action_size = action_size

        self.observation_space = gym.spaces.Box(
            low=0.0,
            high=1.0,
            shape=(obs_size,),
            dtype=np.float32,
        )
        self.action_space = gym.spaces.Discrete(action_size)

        # Stored mask — updated on every reset() and step()
        self._mask = np.zeros(action_size, dtype=np.bool_)
        self.reward_scale = 1.0

    # ─── Gymnasium API ────────────────────────────────────────────────────────

    def reset(
        self,
        *,
        seed: int | None = None,
        options: dict | None = None,
        include_state: bool = False,
    ) -> tuple[np.ndarray, dict]:
        super().reset(seed=seed)

        if self._env_id is None:
            # Allocate a new server-side env
            resp = self.session.post(
                f"{self.server_url}/envs?players={self.num_players}"
            ).json()
            self._env_id = resp["env_id"]
        else:
            # Reset the existing server-side env (preserves RNG sequence)
            self.session.post(f"{self.server_url}/envs/{self._env_id}/reset")

        url = f"{self.server_url}/envs/{self._env_id}/reset"
        if include_state:
            url += "?include_state=true"
        resp = self.session.post(url).json()

        obs = _decode_obs(resp["obs_b64"])
        self._mask = _decode_mask(resp["mask_b64"], self._action_size)
        
        info = {"state": resp.get("state")} if include_state else {}
        # v2.5: Persist terminal stats so auto-resetting VecEnvs (SB3) can see them
        if hasattr(self, "last_info"):
            info["terminal_info"] = self.last_info
            
        return obs, info

    def step(self, action: int, include_state: bool = False) -> tuple[np.ndarray, float, bool, bool, dict]:
        url = f"{self.server_url}/envs/{self._env_id}/step"
        if include_state:
            url += "?include_state=true"
            
        resp = self.session.post(
            url,
            json={
                "action": int(action),
                "dense_reward_scale": float(self.reward_scale),
            },
        ).json()

        obs = _decode_obs(resp["obs_b64"])
        self._mask = _decode_mask(resp["mask_b64"], self._action_size)

        terminated = bool(resp["done"])
        truncated = False  # Brass has no time limit; it always terminates naturally
        info: dict = {
            "vps": resp.get("vps", []),
            "vps_industries": resp.get("vps_industries", []),
            "vps_links": resp.get("vps_links", []),
            "vps_merchant": resp.get("vps_merchant", []),
            "consumed_opponent_coal": resp.get("consumed_opponent_coal", []),
            "consumed_opponent_iron": resp.get("consumed_opponent_iron", []),
            "step_metadata": resp.get("metadata", {}),
            "state": resp.get("state"), # Full state snapshot if requested
            "winner": int(np.argmax(resp.get("vps", [0,0]))) if terminated else -1
        }
        self.last_info = info.copy() # Store for reset persistence
        return obs, float(resp["reward"]), terminated, truncated, info

    def action_masks(self) -> np.ndarray:
        """
        Returns the valid-action mask for the current state.
        Called by sb3_contrib.MaskablePPO before each action selection.
        """
        return self._mask

    def get_action_names(self) -> list[str]:
        """Fetch human-readable action names from the server (cached)."""
        if self._action_names is None:
            resp = self.session.get(f"{self.server_url}/actions")
            if resp.status_code == 200:
                self._action_names = resp.json()
            else:
                self._action_names = [f"Action {i}" for i in range(self.action_space.n)]
        return self._action_names

    def set_reward_scale(self, scale: float) -> None:
        """Update the dense reward scale factor [0, 1]."""
        self.reward_scale = scale

    def close(self) -> None:
        if self._env_id is not None:
            try:
                self.session.delete(
                    f"{self.server_url}/envs/{self._env_id}/free",
                    timeout=2,
                )
            except Exception:
                pass
            self._env_id = None
        self.session.close()

    def render(self) -> None:
        pass  # Terminal/GUI rendering out of scope for RL training

    # ─── Helpers ──────────────────────────────────────────────────────────────

    def _wait_for_server(self, timeout: float) -> None:
        deadline = time.monotonic() + timeout
        last_exc: Exception | None = None
        while time.monotonic() < deadline:
            try:
                self.session.get(f"{self.server_url}/health", timeout=1).raise_for_status()
                return
            except Exception as e:
                last_exc = e
                time.sleep(0.25)
        raise RuntimeError(
            f"Brass server not reachable at {self.server_url} after {timeout}s. "
            f"Last error: {last_exc}\n"
            "Start it with:  go run ./server   (from f:\\Projects\\brass)"
        ) from last_exc
