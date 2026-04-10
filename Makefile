.PHONY: build train eval dashboard clean help

# Default model path for evaluation
MODEL_PATH ?= brass_ppo
EVAL_EPISODES ?= 20
DASHBOARD_DATA_DIR = ../dashboard-v2/public/data

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

build-lib:
	@echo "Building Go shared library..."
	go build -buildmode=c-shared -o python/brass_env/brass_engine.so ./engine/cshared


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
	cd python && uv run --no-sync test_agent.py --model $(MODEL_PATH) --episodes $(EVAL_EPISODES) --output $(DASHBOARD_DATA_DIR)

dashboard:
	@echo "Launching dashboard..."
	cd dashboard-v2 && npm run dev

clean:
	@echo "Cleaning artifacts..."
	rm -f main.exe
	rm -rf python/eval_data
	rm -rf dashboard-v2/dist
	@echo "Cleanup complete."

IMAGE_NAME ?= brass-rl
IMAGE_TAG ?= latest
PROJECT_ID ?= $(shell gcloud config get-value project)
REGION ?= us-central1
REGISTRY ?= $(REGION)-docker.pkg.dev/$(PROJECT_ID)/brass-rl

docker-build:
	@echo "Building Docker image..."
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

cloud-build:
	@echo "Building Docker image in Cloud Build (with caching)..."
	gcloud builds submit --config cloudbuild.yaml --substitutions=_REGISTRY=$(REGISTRY),_IMAGE_NAME=$(IMAGE_NAME),_IMAGE_TAG=$(IMAGE_TAG) .

docker-tag:
	@echo "Tagging Docker image for registry..."
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

docker-push: docker-tag
	@echo "Pushing Docker image to registry..."
	docker push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

terraform-init:
	cd terraform && terraform init -backend-config="bucket=$$(grep GCS_BUCKET_NAME ../.env.local | cut -d= -f2)" -backend-config="prefix=terraform/state"

terraform-apply: terraform-init
	cd terraform && terraform apply -var="project_id=$(PROJECT_ID)" -var="bucket_name=$$(grep GCS_BUCKET_NAME ../.env.local | cut -d= -f2)"

deploy: docker-push terraform-apply
