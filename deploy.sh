#!/usr/bin/env bash
set -euo pipefail

# One-shot deploy for OpenClaw-Go with FAISS HNSW + FTS5 on minimal Debian/Ubuntu
# Supports amd64/arm64. Builds gateway/agent/embedding + llama-server into bin/.
# Tunables via env:
#   LLAMA_JOBS   - parallelism for llama.cpp (default 1)
#   LLAMA_STATIC - ON for static, OFF for shared (default OFF to save memory)
#   BUILD_TYPE   - CMake build type (default Release)
#   USE_SWAP     - on/off to auto-create swap if none (default on)
#   SWAP_SIZE    - swap size (default 4G)

OCG_DIR="/opt/openclaw-go"
OCG_REPO="https://github.com/gliderlab/cogate.git"
LLAMA_REPO="https://github.com/ggml-org/llama.cpp.git"

LLAMA_JOBS="${LLAMA_JOBS:-1}"
LLAMA_STATIC="${LLAMA_STATIC:-OFF}"
BUILD_TYPE="${BUILD_TYPE:-Release}"
USE_SWAP="${USE_SWAP:-on}"
SWAP_SIZE="${SWAP_SIZE:-4G}"

if [ "$(id -u)" -ne 0 ]; then
  echo "Please run as root (or sudo -E)." >&2
  exit 1
fi

# Optional swap for minimal systems
if [ "$USE_SWAP" = "on" ] && ! swapon --show | grep -q .; then
  fallocate -l "$SWAP_SIZE" /swapfile
  chmod 600 /swapfile
  /sbin/mkswap /swapfile
  /sbin/swapon /swapfile
  echo "Swap enabled: $SWAP_SIZE"
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y \
  build-essential git curl ca-certificates \
  cmake pkg-config \
  libopenblas-dev liblapack-dev libgomp1 libssl-dev \
  libsqlite3-dev \
  libfaiss-dev libfaiss-openmp-dev

# Checkout or update
if [ -d "$OCG_DIR/.git" ]; then
  cd "$OCG_DIR"
  git remote set-url origin "$OCG_REPO"
  git fetch --all --prune
  git reset --hard origin/main
else
  rm -rf "$OCG_DIR"
  git clone "$OCG_REPO" "$OCG_DIR"
  cd "$OCG_DIR"
fi

# Sync llama.cpp
if [ -d "llama.cpp/.git" ]; then
  cd llama.cpp
  git remote set-url origin "$LLAMA_REPO"
  git fetch --all --prune
  git reset --hard origin/master
  cd ..
else
  rm -rf llama.cpp
  git clone "$LLAMA_REPO" llama.cpp
fi

# Wrapper Makefile for llama.cpp (outputs to ../bin, static/shared selectable)
cat > llama.cpp/Makefile <<'EOF'
.PHONY: all llama-server clean distclean
JOBS               ?= $(shell nproc)
BUILD_DIR          ?= build
BUILD_TYPE         ?= Release
RUNTIME_OUT        ?= ../bin
LLAMA_BUILD_SERVER     ?= ON
LLAMA_BUILD_EXAMPLES   ?= OFF
LLAMA_BUILD_TESTS      ?= OFF
LLAMA_BUILD_BENCHMARK  ?= OFF
LLAMA_BUILD_EMBEDDING  ?= ON
LLAMA_STATIC           ?= OFF
LLAMA_NATIVE           ?= OFF
BUILD_SHARED_LIBS      ?= ON

all: llama-server
llama-server:
	cmake -S . -B $(BUILD_DIR) \
	  -DCMAKE_BUILD_TYPE=$(BUILD_TYPE) \
	  -DCMAKE_RUNTIME_OUTPUT_DIRECTORY=$(RUNTIME_OUT) \
	  -DLLAMA_BUILD_SERVER=$(LLAMA_BUILD_SERVER) \
	  -DLLAMA_BUILD_EXAMPLES=$(LLAMA_BUILD_EXAMPLES) \
	  -DLLAMA_BUILD_TESTS=$(LLAMA_BUILD_TESTS) \
	  -DLLAMA_BUILD_BENCHMARK=$(LLAMA_BUILD_BENCHMARK) \
	  -DLLAMA_BUILD_EMBEDDING=$(LLAMA_BUILD_EMBEDDING) \
	  -DLLAMA_STATIC=$(LLAMA_STATIC) \
	  -DLLAMA_NATIVE=$(LLAMA_NATIVE) \
	  -DBUILD_SHARED_LIBS=$(BUILD_SHARED_LIBS)
	cmake --build $(BUILD_DIR) --config $(BUILD_TYPE) --target llama-server -- -j $(JOBS)

clean:
	cmake --build $(BUILD_DIR) --config $(BUILD_TYPE) --target clean -- -j $(JOBS) || true

distclean:
	rm -rf $(BUILD_DIR)
EOF

# Shared vs static
if [ "$LLAMA_STATIC" = "ON" ]; then
  export BUILD_SHARED_LIBS=OFF
else
  export BUILD_SHARED_LIBS=ON
fi

# Build all (root Makefile already enables FAISS + FTS5)
make build-all LLAMA_JOBS="$LLAMA_JOBS" BUILD_SHARED_LIBS="$BUILD_SHARED_LIBS" BUILD_TYPE="$BUILD_TYPE" LLAMA_STATIC="$LLAMA_STATIC"

echo "✅ build done: bin/ocg-gateway bin/ocg-agent bin/ocg-embedding bin/llama-server"
echo "LLAMA_STATIC=$LLAMA_STATIC, BUILD_SHARED_LIBS=$BUILD_SHARED_LIBS"
echo "Start: cd $OCG_DIR && ./bin/ocg-gateway"
