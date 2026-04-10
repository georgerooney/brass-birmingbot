"""
Gymnasium environment wrapping the Brass Birmingham Go engine.
Uses C-bindings via ctypes for high performance.

Usage:
    env = BrassEnv(num_players=2)
    obs, info = env.reset()
    obs, reward, terminated, truncated, info = env.step(action)
    mask = env.action_masks()          # required by MaskablePPO
"""

from __future__ import annotations

import ctypes
import json
import os
from typing import Any

import gymnasium as gym
import numpy as np




class BrassEnv(gym.Env):
    """
    Single-player-perspective Gymnasium wrapper for Brass Birmingham.
    Uses C-bindings via ctypes for high performance.
    """

    metadata = {"render_modes": []}

    def __init__(
        self,
        num_players: int = 2,
    ) -> None:
        super().__init__()
        self.num_players = num_players
        
        # Load shared library
        lib_path = os.path.join(os.path.dirname(__file__), "brass_engine.so")
        self.lib = ctypes.CDLL(lib_path)
        
        # Define arg/return types
        self.lib.BrassNewEnv.argtypes = [ctypes.c_int32]
        self.lib.BrassNewEnv.restype = ctypes.c_int32
        
        self.lib.BrassReset.argtypes = [ctypes.c_int32]
        self.lib.BrassReset.restype = None
        
        self.lib.BrassFreeEnv.argtypes = [ctypes.c_int32]
        self.lib.BrassFreeEnv.restype = None
        
        self.lib.BrassStep.argtypes = [
            ctypes.c_int32,
            ctypes.c_int32,
            ctypes.c_double,
            ctypes.POINTER(ctypes.c_float),
            ctypes.POINTER(ctypes.c_int32),
        ]
        self.lib.BrassStep.restype = None
        
        self.lib.BrassObsSize.argtypes = []
        self.lib.BrassObsSize.restype = ctypes.c_int32
        
        self.lib.BrassGetObs.argtypes = [ctypes.c_int32, ctypes.POINTER(ctypes.c_float)]
        self.lib.BrassGetObs.restype = None
        
        self.lib.BrassActionSize.argtypes = []
        self.lib.BrassActionSize.restype = ctypes.c_int32
        
        self.lib.BrassGetMask.argtypes = [ctypes.c_int32, ctypes.POINTER(ctypes.c_int32)]
        self.lib.BrassGetMask.restype = None
        
        self.lib.BrassGetStateJSON.argtypes = [ctypes.c_int32, ctypes.c_char_p, ctypes.c_int]
        self.lib.BrassGetStateJSON.restype = ctypes.c_int
        
        self.lib.BrassGetActionNamesJSON.argtypes = [ctypes.c_char_p, ctypes.c_int]
        self.lib.BrassGetActionNamesJSON.restype = ctypes.c_int
        
        self.lib.BrassActivePlayer.argtypes = [ctypes.c_int32]
        self.lib.BrassActivePlayer.restype = ctypes.c_int32
        
        self.lib.BrassPlayerVP.argtypes = [ctypes.c_int32, ctypes.c_int32]
        self.lib.BrassPlayerVP.restype = ctypes.c_int32
        
        # Query static dimensions
        obs_size = self.lib.BrassObsSize()
        action_size = self.lib.BrassActionSize()
        
        self._action_size = action_size
        
        self.observation_space = gym.spaces.Box(
            low=0.0,
            high=1.0,
            shape=(obs_size,),
            dtype=np.float32,
        )
        self.action_space = gym.spaces.Discrete(action_size)
        
        # Pre-allocate buffers for performance
        self._obs_buf = np.zeros(obs_size, dtype=np.float32)
        self._mask_buf = np.zeros(action_size, dtype=np.int32)
        
        self.reward_scale = 1.0
        
        # Create Go environment
        self._env_id = self.lib.BrassNewEnv(num_players)
        
    def reset(
        self,
        *,
        seed: int | None = None,
        options: dict | None = None,
        include_state: bool = False,
    ) -> tuple[np.ndarray, dict]:
        super().reset(seed=seed)
        
        self.lib.BrassReset(self._env_id)
        
        # Fetch observation and mask
        self.lib.BrassGetObs(self._env_id, self._obs_buf.ctypes.data_as(ctypes.POINTER(ctypes.c_float)))
        self.lib.BrassGetMask(self._env_id, self._mask_buf.ctypes.data_as(ctypes.POINTER(ctypes.c_int32)))
        
        info = {}
        if include_state:
            state_json = self._get_state_json()
            info["state"] = state_json
            if state_json:
                self._fill_diagnostic_info(info, state_json)
            
        return self._obs_buf.copy(), info
        
    def step(self, action: int, include_state: bool = False) -> tuple[np.ndarray, float, bool, bool, dict]:
        reward = ctypes.c_float(0.0)
        done = ctypes.c_int32(0)
        
        self.lib.BrassStep(
            self._env_id,
            ctypes.c_int32(action),
            ctypes.c_double(self.reward_scale),
            ctypes.byref(reward),
            ctypes.byref(done),
        )
        
        # Fetch observation and mask
        self.lib.BrassGetObs(self._env_id, self._obs_buf.ctypes.data_as(ctypes.POINTER(ctypes.c_float)))
        self.lib.BrassGetMask(self._env_id, self._mask_buf.ctypes.data_as(ctypes.POINTER(ctypes.c_int32)))
        
        terminated = bool(done.value)
        truncated = False
        
        info = {}
        
        if include_state:
            state_json = self._get_state_json()
            info["state"] = state_json
            if state_json:
                self._fill_diagnostic_info(info, state_json)
                
        if terminated:
            if "vps" not in info:
                vps = [self.lib.BrassPlayerVP(self._env_id, i) for i in range(self.num_players)]
                info["vps"] = vps
            info["winner"] = int(np.argmax(info["vps"]))
            
        return self._obs_buf.copy(), float(reward.value), terminated, truncated, info
        
    def action_masks(self) -> np.ndarray:
        return self._mask_buf.astype(np.bool_)
        
    def _get_state_json(self) -> dict | None:
        max_len = 1024 * 1024 # 1MB should be enough for state JSON
        buf = ctypes.create_string_buffer(max_len)
        res = self.lib.BrassGetStateJSON(self._env_id, buf, max_len)
        if res > 0:
            return json.loads(buf.value[:res])
        return None
        
    def _fill_diagnostic_info(self, info: dict, state_json: dict) -> None:
        players = state_json.get("players", [])
        info["vps_industries"] = [p.get("vp_audit_industries", 0) for p in players]
        info["vps_links"] = [p.get("vp_audit_links", 0) for p in players]
        info["consumed_opponent_coal"] = [p.get("consumed_opponent_coal", 0) for p in players]
        info["consumed_opponent_iron"] = [p.get("consumed_opponent_iron", 0) for p in players]
        info["vps"] = [p.get("vp_audit_industries", 0) + p.get("vp_audit_links", 0) for p in players]
        
    def get_action_names(self) -> list[str]:
        max_len = 1024 * 1024 # 1MB should be enough
        buf = ctypes.create_string_buffer(max_len)
        res = self.lib.BrassGetActionNamesJSON(buf, max_len)
        if res > 0:
            return json.loads(buf.value[:res])
        return [f"Action {i}" for i in range(self.action_space.n)]
        
    def close(self) -> None:
        if hasattr(self, "_env_id"):
            self.lib.BrassFreeEnv(self._env_id)
            
    def set_reward_scale(self, scale: float) -> None:
        self.reward_scale = scale
