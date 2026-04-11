# Stage 1: Build Go shared library
FROM golang:1.22 AS go-builder

WORKDIR /build
# Copy engine code and go.mod
COPY engine/ ./engine/
COPY go.mod ./

# Build the C-shared library
RUN go build -buildmode=c-shared -o brass_engine.so ./engine/cshared

# Stage 2: Runtime
FROM pytorch/pytorch:2.2.0-cuda12.1-cudnn8-runtime

# Install minimal system dependencies
RUN apt-get update && apt-get install -y curl git && rm -rf /var/lib/apt/lists/*

# Install uv
RUN curl -LsSf https://astral.sh/uv/install.sh | sh
ENV PATH=$PATH:/root/.local/bin

WORKDIR /app

# Pre-install Python dependencies for caching
# We do this before copying source code to leverage Docker cache
RUN uv pip install --system "torch==2.2.0" gymnasium numpy requests stable-baselines3 sb3-contrib tensorboard psutil python-dotenv google-cloud-storage

# Copy the rest of the application source
COPY . .

# Copy Go shared library from stage 1 (Ensure it overwrites any stale local build)
COPY --from=go-builder /build/brass_engine.so /app/python/brass_env/

# Set working directory for execution
WORKDIR /app/python

# Command to run training
CMD ["python", "train.py"]
