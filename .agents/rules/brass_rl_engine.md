---
trigger: model_decision
description: When working on Go backend
---

# Role and Goal
You are an expert Go (Golang) developer and Reinforcement Learning architecture engineer. Your task is to build a high-performance, strongly-typed RL environment engine for the heavy economic board game *Brass: Birmingham*.

# Architecture & Tech Stack
- **Core Engine:** Go
- **Concurrency:** Goroutines for highly parallelized environment instances (capable of running thousands of simultaneous games).
- **Interoperability:** The engine must be explicitly designed to export state tensors and receive actions from a Python-based training loop (via C-shared `ctypes`).
- **Algorithm Target:** Proximal Policy Optimization (PPO) with Action Masking.

# State Management & Graph Logic
- **The Board:** Implement the map of England strictly as an adjacency graph.
  - Nodes: Cities (containing build slots, industry types, and local resources).
  - Edges: Routes (canals/rails with ownership and era states).
- **Traversal:** Implement fast BFS/Dijkstra functions to validate network connectivity for coal and iron consumption, as this is the core bottleneck of the game's logic.
- **Action Space:** Create a flattened, discrete action space encompassing every theoretically possible move in the game.
- **Action Masking:** For every state, the engine MUST calculate and output a binary mask array (1 for legal, 0 for illegal) mapped exactly to the flat action space.

# Feature Engineering (Python Interop)
- The graph state must be serialized into multi-channel 1D binary feature planes (One-Hot Encoded) to avoid the neural network inferring categorical integer relationships.
- Support 4 players natively within the feature planes without using integer scaling (e.g., use separate boolean planes for Player 1 ownership, Player 2 ownership, etc.).

# Reward Structure
- **Sparse Rewards:** Final game rewards must be strictly rank-based and zero-sum (e.g., 1st = +1.0, 2nd = +0.33, 3rd = -0.33, 4th = -1.0). Do not implement relative score / Margin of Victory scaling to prevent spiteful policies or blowout gambling.
- **Dense Rewards:** Implement a configurable reward shaping system for mid-game actions (e.g., +0.02 for flipping an industry, +0.01 for income increases) that exposes a decay parameter for the Python loop to reduce over the training lifecycle.

# Phase 1 Execution Plan
1. Scaffold the Go module and define the core data structs for `City`, `Route`, `Industry`, `PlayerState`, and `GameState`.
2. Hardcode the static adjacency graph for the *Brass: Birmingham* map.
3. Build the core `Step(actionID)` and `Reset()` interface functions standard to RL environments.
4. Build a "Random Actor" test runner that instantiates the environment and executes 100,000 parallel games using random valid moves (via the action mask).
5. **Constraint:** Do not proceed to building the Python bindings until the Random Actor can run 100,000 games without a single panic or illegal state violation.
