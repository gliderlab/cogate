# OCG (OpenClaw-Go) Makefile
# Supports FAISS HNSW vector memory by default

.PHONY: all build build-no-faiss build-agent build-gateway build-embedding build-ocg clean run help test

BIN_DIR := bin
LDFLAGS := -s -w -buildid=

# Default target (FAISS HNSW on): build gateway + agent + embedding + ocg
all: build
	@echo "✅ build done"
	@echo "   $(BIN_DIR)/ocg           # Process manager (start/stop/status)"
	@echo "   $(BIN_DIR)/ocg-gateway   # Gateway entry"
	@echo "   $(BIN_DIR)/ocg-agent     # Agent RPC with FAISS HNSW"
	@echo "   $(BIN_DIR)/ocg-embedding # Local embedding service"
	@echo "   $(BIN_DIR)/llama-server  # llama.cpp server (make build-all or make build-llama)"

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

# Gateway
build-gateway: $(BIN_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/ocg-gateway ./cmd/gateway/

# OCG process manager
build-ocg: $(BIN_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/ocg ./cmd/ocg/

# Agent (FAISS HNSW enabled by default)
build-agent: $(BIN_DIR)
	CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
	CGO_CXXFLAGS="-I/usr/include" \
	CGO_LDFLAGS="-lfaiss -lomp" \
	go build -ldflags="$(LDFLAGS)" -tags "faiss sqlite_fts5" -o $(BIN_DIR)/ocg-agent ./cmd/agent/

# Agent without FAISS (fallback to SQLite linear search)
build-no-faiss: $(BIN_DIR)
	CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
	go build -ldflags="$(LDFLAGS)" -tags "sqlite_fts5" -o $(BIN_DIR)/ocg-agent ./cmd/agent/

# Lite build: ocg + gateway + agent (SQLite only) + embedding
build-lite: $(BIN_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/ocg ./cmd/ocg/
	go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/ocg-gateway ./cmd/gateway/
	CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
	go build -ldflags="$(LDFLAGS)" -tags "sqlite_fts5" -o $(BIN_DIR)/ocg-agent ./cmd/agent/
	go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/ocg-embedding ./cmd/embedding-server/

# llama.cpp server
LLAMA_JOBS ?= 1

build-llama:
	$(MAKE) -C llama.cpp llama-server JOBS=$(LLAMA_JOBS)

# Default build (FAISS on): ocg + gateway + agent + embedding
build: build-ocg build-gateway build-agent build-embedding
	@echo "Stripping symbols..."
	@strip -s $(BIN_DIR)/ocg $(BIN_DIR)/ocg-gateway $(BIN_DIR)/ocg-agent $(BIN_DIR)/ocg-embedding 2>/dev/null || true

# Build everything: gateway + agent + embedding + llama-server
build-all: build build-llama

# Embedding server
build-embedding: $(BIN_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/ocg-embedding ./cmd/embedding-server/

test:
	CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" go test -tags "sqlite_fts5" ./...

# Clean artifacts
clean:
	rm -f $(BIN_DIR)/ocg $(BIN_DIR)/ocg-gateway $(BIN_DIR)/ocg-agent $(BIN_DIR)/ocg-embedding
	rm -f *.db *.log

# Help
help:
	@echo "OCG build commands"
	@echo ""
	@echo "Build:" 
	@echo "  make               # OCG + Gateway + Agent (FAISS HNSW on by default)"
	@echo "  make build-no-faiss # OCG + Gateway + Agent (no FAISS, SQLite fallback)"
	@echo "  make build-embedding # Local embedding server"
	@echo ""
	@echo "Run:" 
	@echo "  $(BIN_DIR)/ocg start      # Start all services (ocg exits after ready)"
	@echo ""
	@echo "Clean:" 
	@echo "  make clean"
