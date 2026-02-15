package memory

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBackfillEmbeddingDim(t *testing.T) {
	dir := t.TempDir()
	store, err := NewVectorMemoryStore(filepath.Join(dir, "vec.db"), Config{})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	vector := serializeVector([]float32{1, 2, 3})
	now := time.Now().Unix()
	_, err = store.db.Exec(`INSERT INTO vector_memories (id, text, vector, importance, category, source, embedding_dim, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		"id-1", "hello", vector, 0.5, "test", "manual", now, now)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	store.backfillEmbeddingDim()

	var dim int
	if err := store.db.QueryRow(`SELECT embedding_dim FROM vector_memories WHERE id = ?`, "id-1").Scan(&dim); err != nil {
		t.Fatalf("query: %v", err)
	}
	if dim != 3 {
		t.Fatalf("expected embedding_dim 3, got %d", dim)
	}
}

func TestLoadExistingVectorsSkipsDimMismatch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "vec.db")

	store, err := NewVectorMemoryStore(dbPath, Config{EmbeddingDim: 4})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	vec := serializeVector([]float32{1, 2, 3})
	now := time.Now().Unix()
	_, err = store.db.Exec(`INSERT INTO vector_memories (id, text, vector, importance, category, source, embedding_dim, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"id-2", "text", vec, 0.5, "cat", "manual", 3, now, now)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	store.hnsw = &HNSWIndex{cfg: HNSWConfig{Dim: 4}}
	store.loadExistingVectors()

	if len(store.hnswIDs) != 0 {
		t.Fatalf("expected no hnsw ids due to dim mismatch, got %d", len(store.hnswIDs))
	}
}
