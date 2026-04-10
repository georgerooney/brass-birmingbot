FROM pytorch/pytorch:2.2.0-cuda12.1-cudnn8-devel

# Install system dependencies
RUN apt-get update && apt-get install -y wget git build-essential make curl

# Install Go
RUN wget https://go.dev/dl/go1.22.1.linux-amd64.tar.gz
RUN tar -C /usr/local -xzf go1.22.1.linux-amd64.tar.gz
ENV PATH=$PATH:/usr/local/go/bin

# Install uv
RUN curl -LsSf https://astral.sh/uv/install.sh | sh
ENV PATH=$PATH:/root/.local/bin

WORKDIR /app

# Copy Go code, Makefile and Python code
COPY engine/ /app/engine/
COPY go.mod /app/
COPY Makefile /app/
COPY python/ /app/python/

# Build Go shared library
RUN make build-lib

# Install Python dependencies
WORKDIR /app/python
RUN uv sync

# Set command to run training
CMD ["uv", "run", "train.py"]
