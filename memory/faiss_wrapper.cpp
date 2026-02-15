//go:build faiss
// +build faiss

/**
 * FAISS HNSW Index C++ Wrapper
 *
 * This file provides C-compatible functions for Go CGO to call.
 */

#include <faiss/IndexHNSW.h>
#include <faiss/IndexFlat.h>
#include <faiss/VectorTransform.h>
#include <faiss/MetricType.h>
#include <faiss/index_io.h>
#include <faiss/AutoTune.h>
#include <faiss/impl/HNSW.h>

#include <cstdlib>
#include <cstring>
#include <iostream>
#include <fstream>
#include <vector>
#include <random>

using namespace faiss;

// Distance metric mapping
MetricType get_metric(const char* metric) {
    if (strcmp(metric, "l2") == 0) {
        return METRIC_L2;
    } else if (strcmp(metric, "ip") == 0) {
        return METRIC_INNER_PRODUCT;
    } else if (strcmp(metric, "cosine") == 0) {
        return METRIC_INNER_PRODUCT; // FAISS cosine is IP with normalized vectors
    }
    return METRIC_L2;
}

// HNSW index wrapper
struct HNSWIndexWrapper {
    IndexHNSWFlat* index;
    std::vector<float> vectors;
    int dim;
    bool trained;
    
    HNSWIndexWrapper(int dim, MetricType metric, int M, int efConstruction) {
        this->dim = dim;
        this->trained = false;
        
        // Create HNSW index
        this->index = new IndexHNSWFlat(dim, M, metric);
        this->index->hnsw.efConstruction = efConstruction;
    }
    
    ~HNSWIndexWrapper() {
        if (index) {
            delete index;
            index = nullptr;
        }
    }
    
    void add_vectors(const float* data, int n) {
        if (n <= 0 || !data) return;
        
        // Persist vectors for saving
        vectors.insert(vectors.end(), data, data + n * dim);
        
        // Add to index
        index->add(n, data);
    }
    
    void search(const float* query, int k, float* distances, long* labels) {
        int nq = 1;  // search one query vector
        // Search
        index->search(nq, query, k, distances, labels);
    }
    
    int count() {
        return index->ntotal;
    }
    
    void save(const char* path) {
        // Save index
        std::ofstream out(path, std::ios::binary);
        if (!out) return;
        
        // Write dimension
        out.write(reinterpret_cast<char*>(&dim), sizeof(int));
        
        // Write vector count
        size_t n = vectors.size() / dim;
        out.write(reinterpret_cast<char*>(&n), sizeof(size_t));
        
        // Write all vectors
        if (vectors.size() > 0) {
            out.write(reinterpret_cast<char*>(vectors.data()), vectors.size() * sizeof(float));
        }
        
        out.close();
    }
    
    void load(const char* path) {
        std::ifstream in(path, std::ios::binary);
        if (!in) return;
        
        // Read dimension
        in.read(reinterpret_cast<char*>(&dim), sizeof(int));
        
        // Read vector count
        size_t n;
        in.read(reinterpret_cast<char*>(&n), sizeof(size_t));
        
        // Read vectors
        vectors.resize(n * dim);
        if (vectors.size() > 0) {
            in.read(reinterpret_cast<char*>(vectors.data()), vectors.size() * sizeof(float));
        }
        
        in.close();
        
        // Rebuild index
        if (vectors.size() > 0) {
            index->add(n, vectors.data());
        }
    }
};

// External C API
extern "C" {

// Create HNSW index
void* faiss_hnsw_create(int dim, const char* metric, int M, int efConstruction) {
    MetricType m = get_metric(metric);
    HNSWIndexWrapper* wrapper = new HNSWIndexWrapper(dim, m, M, efConstruction);
    return wrapper;
}

// Train index
void faiss_hnsw_train(void* ptr, int n, float* data) {
    if (!ptr || n <= 0 || !data) return;
    HNSWIndexWrapper* idx = static_cast<HNSWIndexWrapper*>(ptr);
    idx->add_vectors(data, n);
}

// Add vectors
void faiss_hnsw_add(void* ptr, int n, float* data) {
    if (!ptr || n <= 0 || !data) return;
    HNSWIndexWrapper* idx = static_cast<HNSWIndexWrapper*>(ptr);
    idx->add_vectors(data, n);
}

// Search (returns distances and labels)
void faiss_hnsw_search(void* ptr, float* query, int k, float* distances, long* labels) {
    if (!ptr || !query || k <= 0) return;
    HNSWIndexWrapper* idx = static_cast<HNSWIndexWrapper*>(ptr);
    idx->search(query, k, distances, labels);
}

// Get vector count
long faiss_hnsw_count(void* ptr) {
    if (!ptr) return 0;
    HNSWIndexWrapper* idx = static_cast<HNSWIndexWrapper*>(ptr);
    return idx->count();
}

// Save index
void faiss_hnsw_save(void* ptr, const char* path) {
    if (!ptr || !path) return;
    HNSWIndexWrapper* idx = static_cast<HNSWIndexWrapper*>(ptr);
    idx->save(path);
}

// Load index
void faiss_hnsw_load(void* ptr, const char* path) {
    if (!ptr || !path) return;
    HNSWIndexWrapper* idx = static_cast<HNSWIndexWrapper*>(ptr);
    idx->load(path);
}

// Delete index
void faiss_hnsw_delete(void* ptr) {
    if (!ptr) return;
    HNSWIndexWrapper* idx = static_cast<HNSWIndexWrapper*>(ptr);
    delete idx;
}

// Get FAISS version
const char* faiss_version() {
    return "faiss";
}

} // extern "C"
