// Unified Vector Memory Store - FAISS HNSW + SQLite + Local/OpenAI Embeddings
package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	openai "github.com/sashabaranov/go-openai"
)

// Vector memory store - unified architecture
type VectorMemoryStore struct {
	db           *sql.DB
	hnsw         *HNSWIndex // FAISS HNSW index
	hnswIDs      []string   // HNSW index -> memory ID mapping
	embedding    EmbeddingProvider
	ftsAvailable bool
	cfg          Config
}

// Config
type Config struct {
	ApiKey          string  // OpenAI API Key (or ${OPENAI_API_KEY})
	EmbeddingModel  string  // OpenAI model: text-embedding-3-small/large
	EmbeddingServer string  // Local embedding service URL
	EmbeddingDim    int     // Embedding dimension (auto-detected)
	MaxResults      int     // Max results (default 5)
	MinScore        float32 // Minimum similarity score (default 0.7)
	HNSWPath        string  // HNSW index file path
	HybridEnabled   bool    // Enable hybrid search (default true)
	VectorWeight    float32 // Vector weight (default 0.7)
	TextWeight      float32 // Keyword weight (default 0.3)
	CandidateMult   int     // Candidate multiplier (default 4)
}

// Embedding provider interface
type EmbeddingProvider interface {
	Embed(text string) ([]float32, error)
	Dim() int
	Name() string
}

// OpenAI embedding
type OpenAIProvider struct {
	client *openai.Client
	model  string
	dim    int
}

// Local embedding (llama.cpp server)
type LocalProvider struct {
	serverURL string
	dim       int
	client    *http.Client
}

// Memory entry
type MemoryEntry struct {
	ID         string
	Text       string
	Vector     []float32
	Importance float64
	Category   string
	Source     string
	CreatedAt  int64
	UpdatedAt  int64
}

// Search result (with similarity score)
type MemoryResult struct {
	Entry   MemoryEntry
	Score   float32 // Similarity score (0-1)
	Matched bool    // Whether matched
}

// Model dimension
var EMBEDDING_DIMENSIONS = map[string]int{
	"text-embedding-3-small": 1536,
	"text-embedding-3-large": 3072,
	"text-embedding-ada-002": 1024,
}

// Categories compatible with OpenClaw
var MEMORY_CATEGORIES = []string{"preference", "decision", "fact", "entity", "other"}

// ==================== OpenAI Provider ====================

func NewOpenAIProvider(apiKey, model string) (*OpenAIProvider, error) {
	apiKey = parseEnvVar(apiKey)
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key required")
	}

	dim := EMBEDDING_DIMENSIONS[model]
	if dim == 0 {
		dim = 1536
	}

	return &OpenAIProvider{
		client: openai.NewClient(apiKey),
		model:  model,
		dim:    dim,
	}, nil
}

func (p *OpenAIProvider) Embed(text string) ([]float32, error) {
	resp, err := p.client.CreateEmbeddings(context.Background(), openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(p.model),
		Input: text,
	})
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %v", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	result := make([]float32, len(resp.Data[0].Embedding))
	for i, v := range resp.Data[0].Embedding {
		result[i] = float32(v)
	}
	return result, nil
}

func (p *OpenAIProvider) Dim() int     { return p.dim }
func (p *OpenAIProvider) Name() string { return "openai:" + p.model }

// ==================== Local Provider ====================

func NewLocalProvider(serverURL string, dim int) (*LocalProvider, error) {
	if serverURL == "" {
		serverURL = "http://localhost:50000"
	}
	if dim == 0 {
		dim = 768 // embedding-gemma default dimension
	}

	// Wait for service ready (up to 30s)
	var lastErr error
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/health", nil)
		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			log.Printf("Local embedding service connected: %s", serverURL)
			return &LocalProvider{
				serverURL: serverURL,
				dim:       dim,
				client:    &http.Client{Timeout: 60 * time.Second},
			}, nil
		}
		lastErr = fmt.Errorf("server returned %d", resp.StatusCode)
		time.Sleep(time.Second)
	}

	return nil, fmt.Errorf("local server unavailable: %v", lastErr)
}

func (p *LocalProvider) Embed(text string) ([]float32, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{"text": text})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.serverURL+"/embed", strings.NewReader(string(reqBody)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}

func (p *LocalProvider) Dim() int     { return p.dim }
func (p *LocalProvider) Name() string { return "local:" + p.serverURL }

// ==================== Config Utils ====================

func parseEnvVar(v string) string {
	if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
		return os.Getenv(v[2 : len(v)-1])
	}
	return v
}

// ==================== Main Store ====================

func NewVectorMemoryStore(dbPath string, cfg Config) (*VectorMemoryStore, error) {
	// Default config
	if cfg.MaxResults == 0 {
		cfg.MaxResults = 5
	}
	if cfg.MinScore == 0 {
		cfg.MinScore = 0.7
	}
	if cfg.CandidateMult == 0 {
		cfg.CandidateMult = 4
	}
	if cfg.VectorWeight == 0 {
		cfg.VectorWeight = 0.7
	}
	if cfg.TextWeight == 0 {
		cfg.TextWeight = 0.3
	}
	// default true unless explicitly set to false
	if cfg.HybridEnabled == false {
		// keep as false
	} else {
		cfg.HybridEnabled = true
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	// avoid lock errors in concurrent access
	db.Exec("PRAGMA busy_timeout=5000")

	if err := initSchema(db); err != nil {
		return nil, fmt.Errorf("failed to init schema: %v", err)
	}

	store := &VectorMemoryStore{db: db, cfg: cfg}
	if err := store.ensureFTS(); err != nil {
		log.Printf("FTS init failed: %v", err)
	} else {
		store.ftsAvailable = true
	}

	// Initialize embedding provider (priority: local > OpenAI > placeholder)
	if cfg.EmbeddingServer != "" {
		provider, err := NewLocalProvider(cfg.EmbeddingServer, cfg.EmbeddingDim)
		if err != nil {
			log.Printf("Local embedding connection failed: %v", err)
		} else {
			store.embedding = provider
			cfg.EmbeddingDim = provider.Dim()
			store.cfg.EmbeddingDim = provider.Dim()
			log.Printf("Local embedding: %s (dim=%d)", provider.Name(), provider.Dim())
		}
	}

	if store.embedding == nil && cfg.EmbeddingModel != "" {
		provider, err := NewOpenAIProvider(cfg.ApiKey, cfg.EmbeddingModel)
		if err != nil {
			log.Printf("OpenAI embedding init failed: %v", err)
		} else {
			store.embedding = provider
			cfg.EmbeddingDim = provider.Dim()
			store.cfg.EmbeddingDim = provider.Dim()
			log.Printf("OpenAI embedding: %s (dim=%d)", provider.Name(), provider.Dim())
		}
	}

	if store.embedding == nil {
		log.Printf("No embedding service, using placeholder vectors")
		if cfg.EmbeddingDim == 0 {
			cfg.EmbeddingDim = 768
			store.cfg.EmbeddingDim = cfg.EmbeddingDim
			log.Printf("Embedding dimension not set, defaulting to %d", cfg.EmbeddingDim)
		}
	}

	// Backfill embedding_dim for old rows when NULL/0
	store.backfillEmbeddingDim()

	// Initialize FAISS HNSW when embedding is available
	if store.embedding != nil {
		hnswCfg := HNSWConfig{
			Dim:         cfg.EmbeddingDim,
			M:           16,
			EfSearch:    100,
			EfConstruct: 200,
			Distance:    "cosine",
			StoragePath: cfg.HNSWPath,
		}

		hnsw, err := NewHNSWIndex(hnswCfg)
		if err != nil {
			log.Printf("FAISS HNSW init failed: %v", err)
			log.Printf("Falling back to SQLite linear search")
			store.hnsw = nil
		} else {
			store.hnsw = hnsw
			log.Printf("FAISS HNSW index enabled")

			// Load existing vectors
			store.loadExistingVectors()
		}
	} else {
		log.Printf("No embedding service, skipping FAISS init")
	}

	log.Printf("Vector memory store initialized: faiss=%v, embedding=%v", store.hnsw != nil, store.embedding != nil)
	return store, nil
}

// ==================== Database Schema ====================

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS vector_memories (
			id TEXT PRIMARY KEY,
			text TEXT NOT NULL,
			vector BLOB NOT NULL,
			importance REAL DEFAULT 0.5,
			category TEXT DEFAULT 'other',
			source TEXT DEFAULT 'manual',
			embedding_dim INTEGER,
			created_at INTEGER DEFAULT (strftime('%s','now')),
			updated_at INTEGER DEFAULT (strftime('%s','now'))
		)
	`)
	if err != nil {
		return err
	}

	// Legacy table compatibility: add missing columns
	rows, err := db.Query(`PRAGMA table_info(vector_memories)`)
	if err == nil {
		defer rows.Close()
		hasDim := false
		hasSource := false
		hasUpdated := false
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull int
			var dflt interface{}
			var pk int
			rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk)
			switch name {
			case "embedding_dim":
				hasDim = true
			case "source":
				hasSource = true
			case "updated_at":
				hasUpdated = true
			}
		}
		if !hasDim {
			db.Exec(`ALTER TABLE vector_memories ADD COLUMN embedding_dim INTEGER`)
		}
		if !hasSource {
			db.Exec(`ALTER TABLE vector_memories ADD COLUMN source TEXT DEFAULT 'manual'`)
		}
		if !hasUpdated {
			db.Exec(`ALTER TABLE vector_memories ADD COLUMN updated_at INTEGER DEFAULT (strftime('%s','now'))`)
		}
	}

	db.Exec(`CREATE INDEX IF NOT EXISTS idx_vm_category ON vector_memories(category)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_vm_created ON vector_memories(created_at)`)

	// FTS5 index (keyword search)
	if _, err := db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS vector_memories_fts
		USING fts5(id, text, category)
	`); err != nil {
		log.Printf("⚠️ FTS init failed: %v", err)
	}
	return nil
}

// ==================== Core Operations ====================

func (s *VectorMemoryStore) Store(text string, category string, importance float64) (string, error) {
	return s.StoreWithSource(text, category, importance, "manual")
}

func (s *VectorMemoryStore) StoreWithSource(text string, category string, importance float64, source string) (string, error) {
	vector, err := s.getEmbedding(text)
	if err != nil {
		return "", fmt.Errorf("embedding failed: %v", err)
	}

	id := generateUUID()
	now := time.Now().Unix()
	vectorBlob := serializeVector(vector)
	if source == "" {
		source = "manual"
	}

	_, err = s.db.Exec(`
		INSERT INTO vector_memories (id, text, vector, importance, category, source, embedding_dim, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, text, vectorBlob, importance, category, source, s.cfg.EmbeddingDim, now, now)
	if err == nil {
		s.upsertFTS(id, text, category)
	}
	if err != nil {
		return "", err
	}

	// Add to HNSW index
	if s.hnsw != nil {
		if err := s.hnsw.Add([][]float32{vector}); err != nil {
			log.Printf("HNSW add failed, disabling index: %v", err)
			s.hnsw.Close()
			s.hnsw = nil
			s.hnswIDs = nil
		} else {
			s.hnswIDs = append(s.hnswIDs, id)
			s.saveHNSW()
		}
	}

	log.Printf("✅ Memory stored: %s [%s]", shortID(id), category)
	return id, nil
}

// Update existing memory (re-embed on text change)
func (s *VectorMemoryStore) Update(id string, text string, category string, importance float64) (bool, error) {
	if id == "" {
		return false, fmt.Errorf("id required")
	}
	entry, err := s.getByID(id)
	if err != nil {
		return false, err
	}

	newText := entry.Text
	if strings.TrimSpace(text) != "" {
		newText = text
	}
	newCategory := entry.Category
	if category != "" {
		newCategory = category
	}
	newImportance := entry.Importance
	if importance > 0 {
		newImportance = importance
	}

	vector := entry.Vector
	if strings.TrimSpace(text) != "" {
		vector, err = s.getEmbedding(newText)
		if err != nil {
			return false, err
		}
	}

	now := time.Now().Unix()
	_, err = s.db.Exec(`
		UPDATE vector_memories
		SET text = ?, vector = ?, importance = ?, category = ?, updated_at = ?
		WHERE id = ?
	`, newText, serializeVector(vector), newImportance, newCategory, now, id)
	if err != nil {
		return false, err
	}

	s.upsertFTS(id, newText, newCategory)
	s.rebuildHNSW()
	return true, nil
}

func (s *VectorMemoryStore) getEmbedding(text string) ([]float32, error) {
	var vector []float32
	var err error
	if s.embedding != nil {
		vector, err = s.embedding.Embed(text)
		if err != nil {
			return nil, err
		}
	} else {
		// Placeholder vector
		vector = make([]float32, s.cfg.EmbeddingDim)
		for i := range vector {
			vector[i] = float32(i%256) / 256.0
		}
	}

	// Normalize for cosine/ip metrics
	if s.hnsw != nil {
		metric := s.hnsw.Metric()
		if metric == "cosine" || metric == "ip" {
			normalizeVector(vector)
		}
	}
	return vector, nil
}

// Search - with similarity scores
func (s *VectorMemoryStore) Search(query string, limit int, minScore float32) ([]MemoryResult, error) {
	if limit <= 0 {
		limit = s.cfg.MaxResults
	}
	if minScore == 0 {
		minScore = s.cfg.MinScore
	}

	if s.embedding == nil {
		return s.keywordSearch(query, limit)
	}

	if s.embedding == nil {
		return s.keywordSearch(query, limit)
	}

	queryVec, err := s.getEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("query embedding failed: %v", err)
	}

	if s.cfg.HybridEnabled {
		return s.hybridSearch(query, queryVec, limit, minScore)
	}

	var results []MemoryResult

	// FAISS HNSW search (preferred)
	if s.hnsw != nil && s.hnsw.Count() > 0 {
		results, err = s.hnswSearch(queryVec, limit, minScore)
	} else {
		// Fallback to SQLite linear search
		results, err = s.linearSearch(queryVec, limit, minScore)
	}

	return results, err
}

// HNSW search
func (s *VectorMemoryStore) hnswSearch(queryVec []float32, limit int, minScore float32) ([]MemoryResult, error) {
	distances, labels, err := s.hnsw.SearchWithScores(queryVec, limit)
	if err != nil {
		return nil, err
	}

	metric := s.hnsw.Metric()
	results := make([]MemoryResult, 0, limit)
	for i, dist := range distances {
		label := int(labels[i])
		if label < 0 || label >= len(s.hnswIDs) {
			continue
		}
		id := s.hnswIDs[label]
		entry, err := s.getByID(id)
		if err != nil {
			continue
		}

		var score float32
		switch metric {
		case "ip", "cosine":
			score = dist
		default: // l2
			score = 1.0 / (1.0 + dist)
		}

		if score < minScore {
			continue
		}

		results = append(results, MemoryResult{
			Entry:   entry,
			Score:   score,
			Matched: true,
		})
	}
	return results, nil
}

// SQLite linear search (fallback)
func (s *VectorMemoryStore) linearSearch(queryVec []float32, limit int, minScore float32) ([]MemoryResult, error) {
	rows, err := s.db.Query(`
		SELECT id, text, vector, importance, category, source, created_at, updated_at FROM vector_memories
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type withScore struct {
		entry MemoryEntry
		score float32
	}

	var all []withScore
	for rows.Next() {
		var w withScore
		var vectorBlob []byte
		if err := rows.Scan(&w.entry.ID, &w.entry.Text, &vectorBlob,
			&w.entry.Importance, &w.entry.Category, &w.entry.Source, &w.entry.CreatedAt, &w.entry.UpdatedAt); err != nil {
			return nil, err
		}
		w.entry.Vector = deserializeVector(vectorBlob)
		if len(w.entry.Vector) == len(queryVec) {
			w.score = cosineSimilarity(queryVec, w.entry.Vector)
		}
		all = append(all, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sorting
	for i := 0; i < len(all)-1; i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].score > all[i].score {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	// Filter and return top N
	results := make([]MemoryResult, 0, limit)
	for _, w := range all {
		if w.score >= minScore && len(results) < limit {
			results = append(results, MemoryResult{
				Entry:   w.entry,
				Score:   w.score,
				Matched: w.score >= minScore,
			})
		}
	}
	return results, nil
}

// Keyword search (fallback when no embedding service)
func (s *VectorMemoryStore) keywordSearch(query string, limit int) ([]MemoryResult, error) {
	rows, err := s.db.Query(`
		SELECT id, text, importance, category, source, created_at, updated_at
		FROM vector_memories
		WHERE text LIKE ? OR category LIKE ?
		ORDER BY importance DESC, created_at DESC
		LIMIT ?
	`, "%"+query+"%", "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]MemoryResult, 0, limit)
	for rows.Next() {
		var entry MemoryEntry
		if err := rows.Scan(&entry.ID, &entry.Text, &entry.Importance, &entry.Category, &entry.Source, &entry.CreatedAt, &entry.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, MemoryResult{
			Entry:   entry,
			Score:   1.0,
			Matched: true,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return []MemoryResult{}, nil
	}
	return results, nil
}

// FTS5 keyword search (returns bm25 score)
func (s *VectorMemoryStore) ftsSearch(query string, limit int) (map[string]float32, error) {
	rows, err := s.db.Query(`
		SELECT id, bm25(vector_memories_fts) AS score
		FROM vector_memories_fts
		WHERE vector_memories_fts MATCH ?
		ORDER BY score ASC
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]float32)
	for rows.Next() {
		var id string
		var score float32
		if err := rows.Scan(&id, &score); err != nil {
			return nil, err
		}
		out[id] = score
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *VectorMemoryStore) likeScores(query string, limit int) map[string]float32 {
	rows, err := s.db.Query(`
		SELECT id
		FROM vector_memories
		WHERE text LIKE ? OR category LIKE ?
		ORDER BY importance DESC, created_at DESC
		LIMIT ?
	`, "%"+query+"%", "%"+query+"%", limit)
	if err != nil {
		return map[string]float32{}
	}
	defer rows.Close()

	out := make(map[string]float32)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return map[string]float32{}
		}
		out[id] = 1.0
	}
	return out
}

// Hybrid search: vector + BM25
func (s *VectorMemoryStore) hybridSearch(query string, queryVec []float32, limit int, minScore float32) ([]MemoryResult, error) {
	cand := limit * s.cfg.CandidateMult
	vecResults, err := s.vectorSearch(queryVec, cand)
	if err != nil {
		return nil, err
	}

	textScores := map[string]float32{}
	if s.ftsAvailable {
		textScores, _ = s.ftsSearch(query, cand)
	} else {
		textScores = s.likeScores(query, cand)
	}

	type scored struct {
		entry MemoryEntry
		score float32
	}
	merged := make(map[string]*scored)

	for _, r := range vecResults {
		merged[r.Entry.ID] = &scored{entry: r.Entry, score: s.cfg.VectorWeight * r.Score}
	}

	for id, bm25 := range textScores {
		entry, err := s.getByID(id)
		if err != nil {
			continue
		}
		textScore := float32(1.0 / (1.0 + maxf(0, bm25)))
		if m, ok := merged[id]; ok {
			m.score = m.score + s.cfg.TextWeight*textScore
		} else {
			merged[id] = &scored{entry: entry, score: s.cfg.TextWeight * textScore}
		}
	}

	// Sorting
	list := make([]*scored, 0, len(merged))
	for _, v := range merged {
		list = append(list, v)
	}
	for i := 0; i < len(list)-1; i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].score > list[i].score {
				list[i], list[j] = list[j], list[i]
			}
		}
	}

	results := make([]MemoryResult, 0, limit)
	for _, it := range list {
		if it.score < minScore {
			continue
		}
		results = append(results, MemoryResult{
			Entry:   it.entry,
			Score:   it.score,
			Matched: true,
		})
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// Unified vector search (for hybrid candidate pool)
func (s *VectorMemoryStore) vectorSearch(queryVec []float32, limit int) ([]MemoryResult, error) {
	if s.hnsw != nil && s.hnsw.Count() > 0 {
		return s.hnswSearch(queryVec, limit, 0)
	}
	return s.linearSearch(queryVec, limit, 0)
}

func maxf(a float32, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func (s *VectorMemoryStore) getByID(id string) (MemoryEntry, error) {
	var entry MemoryEntry
	var vectorBlob []byte
	s.db.QueryRow(`
		SELECT text, vector, importance, category, source, created_at, updated_at FROM vector_memories WHERE id = ?
	`, id).Scan(&entry.Text, &vectorBlob, &entry.Importance, &entry.Category, &entry.Source, &entry.CreatedAt, &entry.UpdatedAt)
	entry.ID = id
	entry.Vector = deserializeVector(vectorBlob)
	return entry, nil
}

// Get memory entry (exposed to tools)
func (s *VectorMemoryStore) Get(id string) (MemoryEntry, error) {
	return s.getByID(id)
}

func (s *VectorMemoryStore) Delete(id string) (bool, error) {
	res, err := s.db.Exec("DELETE FROM vector_memories WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return false, nil
	}
	// remove from FTS
	s.db.Exec("DELETE FROM vector_memories_fts WHERE id = ?", id)
	// Rebuild HNSW to keep in sync
	s.rebuildHNSW()
	return true, nil
}

func (s *VectorMemoryStore) rebuildHNSW() {
	if s.hnsw == nil {
		return
	}
	cfg := s.hnsw.Config()
	s.hnsw.Close()
	idx, err := NewHNSWIndex(cfg)
	if err != nil {
		log.Printf("rebuild HNSW failed: %v", err)
		s.hnsw = nil
		s.hnswIDs = nil
		return
	}
	s.hnsw = idx
	s.hnswIDs = nil
	s.loadExistingVectors()
	s.saveHNSW()
}

func (s *VectorMemoryStore) Count() (int, error) {
	var count int
	return count, s.db.QueryRow("SELECT COUNT(*) FROM vector_memories").Scan(&count)
}

func (s *VectorMemoryStore) Close() error {
	if s.hnsw != nil {
		if s.cfg.HNSWPath != "" {
			s.hnsw.Save(s.cfg.HNSWPath)
		}
		s.hnsw.Close()
	}
	return s.db.Close()
}

// Load existing vectors into HNSW
func (s *VectorMemoryStore) loadExistingVectors() {
	s.rebuildFTSIfEmpty()
	rows, err := s.db.Query("SELECT id, vector, embedding_dim FROM vector_memories ORDER BY rowid")
	if err != nil {
		return
	}
	defer rows.Close()

	var vectors [][]float32
	var ids []string
	for rows.Next() {
		var id string
		var vectorBlob []byte
		var embeddingDim sql.NullInt64
		if err := rows.Scan(&id, &vectorBlob, &embeddingDim); err != nil {
			log.Printf("hnsw reload scan err: %v", err)
			continue
		}
		if len(vectorBlob) == 0 {
			continue
		}
		vector := deserializeVector(vectorBlob)
		if vector == nil {
			continue
		}
		dim := len(vector)
		if dim == 0 {
			continue
		}
		if (!embeddingDim.Valid || embeddingDim.Int64 == 0) && dim > 0 {
			if _, err := s.db.Exec(`UPDATE vector_memories SET embedding_dim = ? WHERE id = ?`, dim, id); err != nil {
				log.Printf("embedding_dim backfill during load failed: %v", err)
			}
		}
		if s.hnsw != nil {
			if hnswDim := s.hnsw.Dim(); hnswDim > 0 && dim != hnswDim {
				log.Printf("skip vector %s: dim mismatch %d != %d", shortID(id), dim, hnswDim)
				continue
			}
		}
		vectors = append(vectors, vector)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		log.Printf("hnsw reload rows err: %v", err)
	}

	if s.hnsw != nil {
		s.hnswIDs = ids
		if len(vectors) > 0 {
			if s.hnsw.Loaded() {
				log.Printf("HNSW loaded from disk, restored %d id mappings", len(ids))
			} else {
				if err := s.hnsw.Add(vectors); err != nil {
					log.Printf("Load existing vectors add failed: %v", err)
				} else {
					log.Printf("Loaded %d vectors into HNSW", len(vectors))
				}
			}
			s.saveHNSW()
		}
	}
}

func (s *VectorMemoryStore) saveHNSW() {
	if s.hnsw != nil && s.cfg.HNSWPath != "" {
		if err := s.hnsw.Save(s.cfg.HNSWPath); err != nil {
			log.Printf("save hnsw failed: %v", err)
		}
	}
}

// ==================== Utils ====================

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func serializeVector(v []float32) []byte {
	result := make([]byte, len(v)*4)
	for i, f := range v {
		bits := math.Float32bits(f)
		result[i*4] = byte(bits)
		result[i*4+1] = byte(bits >> 8)
		result[i*4+2] = byte(bits >> 16)
		result[i*4+3] = byte(bits >> 24)
	}
	return result
}

func (s *VectorMemoryStore) backfillEmbeddingDim() {
	rows, err := s.db.Query("SELECT id, vector, embedding_dim FROM vector_memories WHERE embedding_dim IS NULL OR embedding_dim = 0")
	if err != nil {
		log.Printf("backfill embedding_dim skipped: %v", err)
		return
	}
	defer rows.Close()

	type pendingUpdate struct {
		id  string
		dim int
	}
	var pending []pendingUpdate

	for rows.Next() {
		var id string
		var vectorBlob []byte
		var embeddingDim sql.NullInt64
		if err := rows.Scan(&id, &vectorBlob, &embeddingDim); err != nil {
			log.Printf("backfill scan err: %v", err)
			continue
		}
		dim := 0
		if len(vectorBlob) >= 4 {
			dim = len(vectorBlob) / 4
		}
		if dim == 0 && s.cfg.EmbeddingDim > 0 {
			dim = s.cfg.EmbeddingDim
		}
		if dim == 0 {
			continue
		}
		pending = append(pending, pendingUpdate{id: id, dim: dim})
	}
	if err := rows.Err(); err != nil {
		log.Printf("backfill rows err: %v", err)
	}

	updated := 0
	for _, u := range pending {
		if _, err := s.db.Exec("UPDATE vector_memories SET embedding_dim = ? WHERE id = ?", u.dim, u.id); err != nil {
			log.Printf("backfill update err: %v", err)
			continue
		}
		updated++
	}
	if updated > 0 {
		log.Printf("backfilled embedding_dim for %d rows", updated)
	}
}

func (s *VectorMemoryStore) ensureFTS() error {
	_, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS vector_memories_fts
		USING fts5(id, text, category)
	`)
	if err != nil {
		return err
	}
	s.ftsAvailable = true
	return nil
}

func (s *VectorMemoryStore) rebuildFTSIfEmpty() {
	if err := s.ensureFTS(); err != nil {
		s.ftsAvailable = false
		return
	}
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM vector_memories_fts").Scan(&count)
	if count > 0 {
		return
	}
	rows, err := s.db.Query("SELECT id, text, category FROM vector_memories")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, text, category string
		rows.Scan(&id, &text, &category)
		s.upsertFTS(id, text, category)
	}
}

func (s *VectorMemoryStore) upsertFTS(id, text, category string) {
	if err := s.ensureFTS(); err != nil {
		s.ftsAvailable = false
		return
	}
	_, _ = s.db.Exec(`
		INSERT INTO vector_memories_fts(id, text, category)
		VALUES (?, ?, ?)
	`, id, text, category)
}

func deserializeVector(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	result := make([]float32, len(b)/4)
	for i := 0; i < len(result); i++ {
		bits := uint32(b[i*4]) | uint32(b[i*4+1])<<8 |
			uint32(b[i*4+2])<<16 | uint32(b[i*4+3])<<24
		result[i] = math.Float32frombits(bits)
	}
	return result
}

func cosineSimilarity(a, b []float32) float32 {
	var dot, normA, normB float32
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / float32(math.Sqrt(float64(normA*normB)))
}

func normalizeVector(v []float32) {
	var norm float32
	for _, x := range v {
		norm += x * x
	}
	if norm == 0 {
		return
	}
	norm = float32(math.Sqrt(float64(norm)))
	for i := range v {
		v[i] /= norm
	}
}

func generateUUID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	hexStr := hex.EncodeToString(bytes)
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hexStr[:8], hexStr[8:12], hexStr[12:16], hexStr[16:20], hexStr[20:32])
}

// Category detection
func DetectCategory(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "prefer") || strings.Contains(lower, "like") ||
		strings.Contains(lower, "love") || strings.Contains(lower, "want"):
		return "preference"
	case strings.Contains(lower, "decided") || strings.Contains(lower, "will use"):
		return "decision"
	case strings.Contains(lower, "@") || strings.Contains(lower, "e-mail"):
		return "entity"
	default:
		return "other"
	}
}
