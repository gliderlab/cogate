# Vector Memory (FAISS HNSW)

Vector memory store with FAISS HNSW index + SQLite + Local/Remote Embedding.

## Architecture

```
VectorMemoryStore
├── SQLite (persistent storage)
├── FAISS HNSW (vector index)
├── FTS5 (keyword search)
└── Embedding Provider
    ├── LocalProvider (llama.cpp)
    └── OpenAIProvider (OpenAI API)
```

## Configuration

```go
type Config struct {
    ApiKey           string  // OpenAI API Key
    EmbeddingModel   string  // text-embedding-3-small/large
    EmbeddingServer  string  // local embedding service URL
    EmbeddingDim    int     // vector dimension (auto-detected)
    MaxResults      int     // default 5
    MinScore        float32 // minimum similarity (default 0.7)
    HNSWPath        string  // index file path
    HybridEnabled   bool    // hybrid search (default true)
    VectorWeight    float32 // vector weight (default 0.7)
    TextWeight      float32 // keyword weight (default 0.3)
    CandidateMult   int     // candidate multiplier (default 4)
}
```

## Initialization

```go
store, err := memory.NewVectorMemoryStore("ocg.db", memory.Config{
    EmbeddingServer: "http://localhost:50001",
    HNSWPath:       "vector.index",
})
```

Priority: local embedding → OpenAI → placeholder vector

## Core Operations

### Store Memory

```go
id, err := store.Store("I like blue", "preference", 0.8)
```

### Update Memory

```go
updated, err := store.Update(id, "I prefer green", "", 0.9)
```

### Search

```go
results, err := store.Search("my color preference", 5, 0.7)
// returns: []MemoryResult{Entry, Score, Matched}
```

### Get

```go
entry, err := store.Get(id)
```

### Delete

```go
deleted, err := store.Delete(id)
```

## Search Modes

| Mode | Condition | Description |
|------|-----------|-------------|
| **Hybrid** | HybridEnabled=true | Vector + BM25 fusion |
| **Vector** | HybridEnabled=false | HNSW / SQLite |
| **Keyword** | no embedding service | FTS5 / LIKE |

### HNSW Vector Search

- Uses FAISS HNSW index
- Supports cosine/ip/l2 distance
- Auto-fallback to SQLite linear search

### Hybrid Search

```
score = VectorWeight * vector_score + TextWeight * text_score
```

- vector candidates × CandidateMult
- keyword candidates (FTS5 BM25)
- fused ranking

## Database Schema

```sql
CREATE TABLE vector_memories (
    id TEXT PRIMARY KEY,
    text TEXT NOT NULL,
    vector BLOB NOT NULL,
    importance REAL DEFAULT 0.5,
    category TEXT DEFAULT 'other',
    source TEXT DEFAULT 'manual',
    embedding_dim INTEGER,
    created_at INTEGER,
    updated_at INTEGER
)

-- FTS5 index
CREATE VIRTUAL TABLE vector_memories_fts USING fts5(id, text, category)
```

## Categories

```go
var MEMORY_CATEGORIES = []string{
    "preference",  // user preferences
    "decisions",   // decisions made
    "facts",       // factual information
    "entities",    // entities/people
    "other",       // other
}
```

## Embedding Provider

### LocalProvider

```go
provider, err := memory.NewLocalProvider("http://localhost:50001", 768)
```

- Connect to local llama.cpp embedding service
- 30s timeout waiting for service readiness

### OpenAIProvider

```go
provider, err := memory.NewOpenAIProvider("sk-xxx", "text-embedding-3-small")
```

- Uses OpenAI API
- Supports text-embedding-3-small/large/ada-002

## Vector Dimensions

| Model | Dimension |
|-------|-----------|
| text-embedding-3-small | 1536 |
| text-embedding-3-large | 3072 |
| text-embedding-ada-002 | 1024 |
| embedding-gemma-300M | 768 |

## Index Persistence

- HNSW index saved to `HNSWPath`
- Auto-load existing vectors on startup
- Auto-rebuild index after add/update/delete

## Performance

- HNSW: O(log n) query
- SQLite linear: O(n)
- FTS5: O(log n)

## Error Handling

- embedding service unavailable → fallback to keyword search
- HNSW init failed → use SQLite linear search
- vector dimension mismatch → skip vector and log

## FAISS HNSW Implementation

- **faiss_hnsw.go**: CGO binding to FAISS library (requires `cgo` + `faiss` tag)
- **hnsw_stub.go**: stub for non-FAISS builds (returns error)

### Build

```bash
# With FAISS
CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
CGO_CXXFLAGS="-I/usr/include" \
CGO_LDFLAGS="-lfaiss -lgomp -lblas -llapack" \
go build -tags "faiss sqlite_fts5" -o ocg-agent ./cmd/agent/

# Without FAISS (SQLite linear search)
CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
go build -tags "sqlite_fts5" -o ocg-agent ./cmd/agent/
```

### HNSW Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| M | 16 | connections per node |
| EfSearch | 100 | search exploration factor |
| EfConstruct | 200 | construction exploration factor |
| Distance | l2 | distance metric (l2/ip/cosine) |
