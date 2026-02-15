#pragma once

#ifdef __cplusplus
extern "C" {
#endif

void* faiss_hnsw_create(int dim, const char* metric, int M, int efConstruction);
void  faiss_hnsw_train(void* ptr, int n, float* data);
void  faiss_hnsw_add(void* ptr, int n, float* data);
void  faiss_hnsw_search(void* ptr, float* query, int k, float* distances, long* labels);
long  faiss_hnsw_count(void* ptr);
void  faiss_hnsw_save(void* ptr, const char* path);
void  faiss_hnsw_load(void* ptr, const char* path);
void  faiss_hnsw_delete(void* ptr);
const char* faiss_version();

#ifdef __cplusplus
}
#endif
