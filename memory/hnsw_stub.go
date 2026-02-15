//go:build !faiss
// +build !faiss

// FAISS disabled stub
package memory

import "fmt"

// HNSW index config (kept in sync with the FAISS build)
type HNSWConfig struct {
	Dim         int
	M           int
	EfSearch    int
	EfConstruct int
	Distance    string
	StoragePath string
}

// HNSWIndex placeholder when FAISS build tag is missing.
// Used to compile and automatically fall back to SQLite search.
// Note: all methods return unavailable or empty results.
type HNSWIndex struct{ cfg HNSWConfig }

func (idx *HNSWIndex) Config() HNSWConfig { return idx.cfg }

func NewHNSWIndex(cfg HNSWConfig) (*HNSWIndex, error) {
	return nil, fmt.Errorf("FAISS not enabled (build without -tags faiss)")
}

func (idx *HNSWIndex) Add(vectors [][]float32) error {
	for _, v := range vectors {
		if len(v) != idx.cfg.Dim {
			return fmt.Errorf("vector dimension mismatch: got %d, expected %d", len(v), idx.cfg.Dim)
		}
	}
	return nil
}

func (idx *HNSWIndex) Search(query []float32, k int) ([]float32, []int64, error) {
	return nil, nil, fmt.Errorf("FAISS not enabled (build without -tags faiss)")
}

func (idx *HNSWIndex) SearchWithScores(query []float32, k int) ([]float32, []int64, error) {
	return nil, nil, fmt.Errorf("FAISS not enabled (build without -tags faiss)")
}

func (idx *HNSWIndex) Metric() string { return "" }

func (idx *HNSWIndex) Dim() int { return idx.cfg.Dim }

func (idx *HNSWIndex) Loaded() bool { return false }

func (idx *HNSWIndex) Save(path string) error { return nil }

func (idx *HNSWIndex) Load(path string) error { return nil }

func (idx *HNSWIndex) Count() int64 { return 0 }

func (idx *HNSWIndex) Close() error { return nil }

func FAISSVersion() string { return "disabled" }

func IsFAISSAvailable() bool { return false }
