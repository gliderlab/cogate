//go:build cgo && faiss
// +build cgo,faiss

// FAISS HNSW vector index (CGO implementation)
package memory

import (
	"fmt"
	"log"
	"os"
	"sync"
	"unsafe"
)

// #include <stdlib.h>
// #include "faiss_wrapper.h"
import "C"

// HNSW index configuration
type HNSWConfig struct {
	Dim         int    // vector dimension
	M           int    // number of connections per node
	EfSearch    int    // search ef (exploration) parameter
	EfConstruct int    // construction ef parameter
	Distance    string // distance metric: "l2", "ip", "cosine"
	StoragePath string // path for persistence
}

type HNSWIndex struct {
	ptr    unsafe.Pointer
	cfg    HNSWConfig
	dim    int
	mu     sync.Mutex
	loaded bool
}

func (idx *HNSWIndex) Config() HNSWConfig {
	return idx.cfg
}

// Create a new HNSW index
func NewHNSWIndex(cfg HNSWConfig) (*HNSWIndex, error) {
	if cfg.Dim <= 0 {
		return nil, fmt.Errorf("invalid dimension: %d", cfg.Dim)
	}

	// Defaults
	if cfg.M == 0 {
		cfg.M = 16
	}
	if cfg.EfSearch == 0 {
		cfg.EfSearch = 100
	}
	if cfg.EfConstruct == 0 {
		cfg.EfConstruct = 200
	}
	if cfg.Distance == "" {
		cfg.Distance = "l2"
	}

	// Create index
	ptr := C.faiss_hnsw_create(
		C.int(cfg.Dim),
		C.CString(cfg.Distance),
		C.int(cfg.M),
		C.int(cfg.EfConstruct),
	)

	if ptr == nil {
		return nil, fmt.Errorf("failed to create HNSW index")
	}

	idx := &HNSWIndex{
		ptr:    ptr,
		cfg:    cfg,
		dim:    cfg.Dim,
		loaded: false,
	}

	// Attempt to load existing index (only if file is non-empty)
	if cfg.StoragePath != "" {
		if fi, err := os.Stat(cfg.StoragePath); err == nil {
			if fi.Size() == 0 {
				log.Printf("HNSW index file is empty, skipping load: %s", cfg.StoragePath)
			} else {
				C.faiss_hnsw_load(ptr, C.CString(cfg.StoragePath))
				idx.loaded = true
				log.Printf("HNSW index loaded: %s", cfg.StoragePath)
			}
		}
	}

	log.Printf("✅ HNSW index created: dim=%d, M=%d, metric=%s", cfg.Dim, cfg.M, cfg.Distance)

	return idx, nil
}

// Add vectors
func (idx *HNSWIndex) Add(vectors [][]float32) error {
	if len(vectors) == 0 {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Flatten vectors
	n := len(vectors)
	data := make([]C.float, n*idx.dim)
	for i, v := range vectors {
		if len(v) != idx.dim {
			return fmt.Errorf("vector dimension mismatch: got %d, expected %d", len(v), idx.dim)
		}
		for j, f := range v {
			data[i*idx.dim+j] = C.float(f)
		}
	}

	// Add to FAISS
	C.faiss_hnsw_add(idx.ptr, C.int(n), &data[0])

	return nil
}

// Search nearest neighbors
func (idx *HNSWIndex) Search(query []float32, k int) (distances []float32, labels []int64, err error) {
	if k <= 0 {
		k = 5
	}

	if len(query) != idx.dim {
		return nil, nil, fmt.Errorf("query dimension mismatch: got %d, expected %d", len(query), idx.dim)
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Prepare query vector
	queryData := make([]C.float, idx.dim)
	for i, f := range query {
		queryData[i] = C.float(f)
	}

	// Allocate result buffers
	distBuf := make([]C.float, k)
	labelBuf := make([]C.long, k)

	// Search
	C.faiss_hnsw_search(idx.ptr, &queryData[0], C.int(k), &distBuf[0], &labelBuf[0])

	// Convert results
	distances = make([]float32, k)
	labels = make([]int64, k)
	for i := 0; i < k; i++ {
		distances[i] = float32(distBuf[i])
		labels[i] = int64(labelBuf[i])
	}

	return distances, labels, nil
}

// Search and return scores (already converted)
func (idx *HNSWIndex) SearchWithScores(query []float32, k int) ([]float32, []int64, error) {
	distances, labels, err := idx.Search(query, k)
	if err != nil {
		return nil, nil, err
	}
	return distances, labels, nil
}

// Metric returns the distance metric used by the index
func (idx *HNSWIndex) Metric() string {
	return idx.cfg.Distance
}

// Dim returns the dimension of the index
func (idx *HNSWIndex) Dim() int {
	return idx.dim
}

// Loaded indicates whether the index was loaded from disk
func (idx *HNSWIndex) Loaded() bool {
	return idx.loaded
}

// Save index to disk
func (idx *HNSWIndex) Save(path string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	C.faiss_hnsw_save(idx.ptr, C.CString(path))
	log.Printf("✅ HNSW index saved: %s", path)
	return nil
}

// Load index from disk
func (idx *HNSWIndex) Load(path string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	C.faiss_hnsw_load(idx.ptr, C.CString(path))
	log.Printf("✅ HNSW index loaded: %s", path)
	return nil
}

// Count returns the number of vectors
func (idx *HNSWIndex) Count() int64 {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	return int64(C.faiss_hnsw_count(idx.ptr))
}

// Close releases resources
func (idx *HNSWIndex) Close() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.ptr != nil {
		C.faiss_hnsw_delete(idx.ptr)
		idx.ptr = nil
	}

	return nil
}

// FAISSVersion returns the FAISS version string
func FAISSVersion() string {
	return C.GoString(C.faiss_version())
}

// IsFAISSAvailable indicates whether FAISS is available (build tag ensures this file is included)
func IsFAISSAvailable() bool {
	return true
}
