# OCG (OpenClaw-Go) Makefile
# Supports FAISS HNSW vector memory by default

.PHONY: all build build-no-faiss build-agent build-gateway build-embedding clean run help test

BIN_DIR := bin

# Default target (FAISS HNSW on): build gateway + agent + embedding
all: build
	@echo "✅ build done"
	@echo "   $(BIN_DIR)/ocg-gateway   # Gateway entry"
	@echo "   $(BIN_DIR)/ocg-agent     # Agent RPC with FAISS HNSW"
	@echo "   $(BIN_DIR)/ocg-embedding # Local embedding service"
	@echo "   $(BIN_DIR)/llama-server  # llama.cpp server (make build-all or make build-llama)"

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

# Gateway
build-gateway: $(BIN_DIR)
	go build -o $(BIN_DIR)/ocg-gateway ./cmd/gateway/

# Agent (FAISS HNSW enabled by default)
build-agent: $(BIN_DIR)
	CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
	CGO_CXXFLAGS="-I/usr/include" \
	CGO_LDFLAGS="-lfaiss -lgomp -lblas -llapack" \
	go build -tags "faiss sqlite_fts5" -o $(BIN_DIR)/ocg-agent ./cmd/agent/

# Agent without FAISS (fallback to SQLite linear search)
build-no-faiss: $(BIN_DIR)
	CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
	go build -tags "sqlite_fts5" -o $(BIN_DIR)/ocg-agent ./cmd/agent/

# Lite build: gateway + agent (SQLite only) + embedding
build-lite: $(BIN_DIR)
	go build -o $(BIN_DIR)/ocg-gateway ./cmd/gateway/
	CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
	go build -tags "sqlite_fts5" -o $(BIN_DIR)/ocg-agent ./cmd/agent/
	go build -o $(BIN_DIR)/ocg-embedding ./cmd/embedding-server/

# llama.cpp server
LLAMA_JOBS ?= 1

build-llama:
	$(MAKE) -C llama.cpp llama-server JOBS=$(LLAMA_JOBS)

# Default build (FAISS on): gateway + agent + embedding
build: build-gateway build-agent build-embedding

# Build everything: gateway + agent + embedding + llama-server
build-all: build build-llama

# Embedding server
build-embedding: $(BIN_DIR)
	go build -o $(BIN_DIR)/ocg-embedding ./cmd/embedding-server/

test:
	CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" go test -tags "sqlite_fts5" ./...

# Clean artifacts
clean:
	rm -f $(BIN_DIR)/ocg-gateway $(BIN_DIR)/ocg-agent $(BIN_DIR)/ocg-embedding
	rm -f *.db *.log

# Help
help:
	@echo "OCG build commands"
	@echo ""
	@echo "Build:" 
	@echo "  make               # Gateway + Agent (FAISS HNSW on by default)"
	@echo "  make build-no-faiss # Gateway + Agent (no FAISS, SQLite fallback)"
	@echo "  make build-embedding # Local embedding server"
	@echo ""
	@echo "Run:" 
	@echo "  $(BIN_DIR)/ocg-gateway    # Run gateway (auto-starts agent)"
	@echo ""
	@echo "Clean:" 
	@echo "  make clean"
