# Vector Memory (FAISS HNSW)

向量记忆存储，支持 FAISS HNSW 索引 + SQLite + 本地/远程 Embedding。

## 架构

```
VectorMemoryStore
├── SQLite (持久化存储)
├── FAISS HNSW (向量索引)
├── FTS5 (关键词搜索)
└── Embedding Provider
    ├── LocalProvider (llama.cpp)
    └── OpenAIProvider (OpenAI API)
```

## 配置

```go
type Config struct {
    ApiKey           string  // OpenAI API Key
    EmbeddingModel   string  // text-embedding-3-small/large
    EmbeddingServer  string  // 本地 embedding 服务 URL
    EmbeddingDim    int     // 向量维度 (自动检测)
    MaxResults      int     // 默认 5
    MinScore        float32 // 最小相似度 (默认 0.7)
    HNSWPath        string  // 索引文件路径
    HybridEnabled   bool    // 混合搜索 (默认 true)
    VectorWeight    float32 // 向量权重 (默认 0.7)
    TextWeight      float32 // 关键词权重 (默认 0.3)
    CandidateMult   int     // 候选乘数 (默认 4)
}
```

## 初始化

```go
store, err := memory.NewVectorMemoryStore("ocg.db", memory.Config{
    EmbeddingServer: "http://localhost:50001",
    HNSWPath:       "vector.index",
})
```

优先级：本地 embedding → OpenAI → 占位向量

## 核心操作

### 存储记忆

```go
id, err := store.Store("我喜欢蓝色", "preference", 0.8)
```

### 更新记忆

```go
updated, err := store.Update(id, "我更喜欢绿色", "", 0.9)
```

### 搜索

```go
results, err := store.Search("我的颜色偏好", 5, 0.7)
// 返回: []MemoryResult{Entry, Score, Matched}
```

### 获取

```go
entry, err := store.Get(id)
```

### 删除

```go
deleted, err := store.Delete(id)
```

## 搜索模式

| 模式 | 条件 | 说明 |
|------|------|------|
| **混合搜索** | HybridEnabled=true | 向量 + BM25 融合 |
| **向量搜索** | HybridEnabled=false | HNSW / SQLite |
| **关键词搜索** | 无 embedding 服务 | FTS5 / LIKE |

### HNSW 向量搜索

- 使用 FAISS HNSW 索引
- 支持 cosine/ip/l2 距离
- 自动回退到 SQLite 线性搜索

### 混合搜索

```
score = VectorWeight * vector_score + TextWeight * text_score
```

- 向量候选 × CandidateMult
- 关键词候选 (FTS5 BM25)
- 融合排序

## 数据库 Schema

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

-- FTS5 索引
CREATE VIRTUAL TABLE vector_memories_fts USING fts5(id, text, category)
```

## 类别

```go
var MEMORY_CATEGORIES = []string{
    "preference",  // 偏好
    "decision",    // 决策
    "fact",        // 事实
    "entity",      // 实体
    "other",       // 其他
}
```

## Embedding Provider

### LocalProvider

```go
provider, err := memory.NewLocalProvider("http://localhost:50001", 768)
```

- 连接本地 llama.cpp embedding 服务
- 30s 超时等待服务就绪

### OpenAIProvider

```go
provider, err := memory.NewOpenAIProvider("sk-xxx", "text-embedding-3-small")
```

- 使用 OpenAI API
- 支持 text-embedding-3-small/large/ada-002

## 向量维度

| 模型 | 维度 |
|------|------|
| text-embedding-3-small | 1536 |
| text-embedding-3-large | 3072 |
| text-embedding-ada-002 | 1024 |
| embedding-gemma-300M | 768 |

## 索引持久化

- HNSW 索引保存到 `HNSWPath`
- 启动时自动加载已有向量
- 增删改后自动重建索引

## 性能

- HNSW: O(log n) 查询
- SQLite 线性: O(n)
- FTS5: O(log n)

## 错误处理

- embedding 服务不可用 → 回退到关键词搜索
- HNSW 初始化失败 → 使用 SQLite 线性搜索
- 向量维度不匹配 → 跳过该向量并记录日志

## FAISS HNSW 实现

- **faiss_hnsw.go**: CGO 调用 FAISS 库 (需要 `cgo` + `faiss` tag)
- **hnsw_stub.go**: 非 FAISS 编译时的存根 (返回错误)

### 编译

```bash
# 带 FAISS
CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
CGO_CXXFLAGS="-I/usr/include" \
CGO_LDFLAGS="-lfaiss -lgomp -lblas -llapack" \
go build -tags "faiss sqlite_fts5" -o ocg-agent ./cmd/agent/

# 无 FAISS (SQLite 线性搜索)
CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
go build -tags "sqlite_fts5" -o ocg-agent ./cmd/agent/
```

### HNSW 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| M | 16 | 每节点连接数 |
| EfSearch | 100 | 搜索时探索因子 |
| EfConstruct | 200 | 构建时探索因子 |
| Distance | l2 | 距离度量 (l2/ip/cosine) |
