.PHONY: build train eval dashboard clean help

# Default model path for evaluation
MODEL_PATH ?= python/runs/ppo_latest/brass_ppo_final.zip
EVAL_EPISODES ?= 20
DASHBOARD_DATA_DIR = dashboard-v2/public/data

help:
	@echo "Brass Project Management Commands:"
	@echo "  make install      - Install all dependencies (Python uv + Dashboard npm)"
	@echo "  make build        - Compile the Go engine server"
	@echo "  make train        - Start the RL training pipeline (uv + python)"
	@echo "  make eval         - Run evaluation and update dashboard data"
	@echo "  make dashboard    - Start the React development server"
	@echo "  make clean        - Remove build artifacts and temporary data"

build:
	@echo "Building Go engine..."
	go build -o main.exe main.go

install:
	@echo "Installing all dependencies..."
	@echo "1/2: Python (uv sync)..."
	cd python && uv sync
	@echo "2/2: Dashboard (npm install)..."
	cd dashboard-v2 && npm install

train:
	@echo "Starting RL training..."
	cd python && uv run train.py

eval:
	@echo "Running evaluation with model: $(MODEL_PATH)"
	uv run python/test_agent.py --model $(MODEL_PATH) --episodes $(EVAL_EPISODES) --output $(DASHBOARD_DATA_DIR)

dashboard:
	@echo "Launching dashboard..."
	cd dashboard-v2 && npm run dev

clean:
	@echo "Cleaning artifacts..."
	rm -f main.exe
	rm -rf python/eval_data
	rm -rf dashboard-v2/dist
	@echo "Cleanup complete."
