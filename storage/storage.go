// Storage module - SQLite data storage

package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Storage struct {
	db *sql.DB
}

type Message struct {
	ID         int64     `json:"id"`
	SessionKey string    `json:"session_key"`
	Role       string    `json:"role"` // user, assistant, system
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

type Memory struct {
	ID         int64     `json:"id"`
	Key        string    `json:"key"`
	Text       string    `json:"text"`     // memory content
	Category   string    `json:"category"` // preference, decision, fact, entity, other
	Importance float64   `json:"importance"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type FileRecord struct {
	ID        int64     `json:"id"`
	Path      string    `json:"path"`
	Content   string    `json:"content"`
	MimeType  string    `json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`
}

type Config struct {
	ID        int64     `json:"id"`
	Section   string    `json:"section"` // e.g., "llm", "gateway", "storage"
	Key       string    `json:"key"`     // e.g., "apiKey", "baseUrl", "model"
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SessionMeta struct {
	SessionKey               string    `json:"session_key"`
	TotalTokens              int       `json:"total_tokens"`
	CompactionCount          int       `json:"compaction_count"`
	LastSummary              string    `json:"last_summary"`
	MemoryFlushAt            time.Time `json:"memory_flush_at"`
	MemoryFlushCompactionCnt int       `json:"memory_flush_compaction_count"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// EventPriority levels (lower = higher priority)
// 0 = Critical (broadcast to all channels immediately)
// 1 = Important (channel broadcast)
// 2 = Normal (process when idle)
// 3 = Low (process when available)
type EventPriority int

const (
	PriorityCritical EventPriority = 0 // Broadcast to all channels
	PriorityHigh     EventPriority = 1 // Broadcast to configured channels
	PriorityNormal   EventPriority = 2 // Process when idle
	PriorityLow      EventPriority = 3 // Process when available
)

type Event struct {
	ID          int64       `json:"id"`
	Title       string      `json:"title"`
	Content     string      `json:"content"`
	Priority    EventPriority `json:"priority"` // 0-3
	Status      string      `json:"status"`      // pending, processing, completed, dismissed
	Channel     string      `json:"channel"`    // telegram, discord, etc (empty = all)
	CreatedAt   time.Time   `json:"created_at"`
	ProcessedAt *time.Time  `json:"processed_at,omitempty"`
}

func New(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	s := &Storage{db: db}

	// Set WAL mode
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, fmt.Errorf("failed to set WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL;"); err != nil {
		return nil, fmt.Errorf("failed to set synchronous: %v", err)
	}

	// Initialize tables
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %v", err)
	}

	// Optional: bind executable with database (build tag binddb)
	if err := BindExecutable(s, dbPath); err != nil {
		return nil, fmt.Errorf("bind executable failed: %v", err)
	}

	log.Printf("✅ Storage: database %s", dbPath)
	return s, nil
}

func (s *Storage) initSchema() error {
	// Messages table
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_key TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Memories table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS memories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT UNIQUE,
			value TEXT,
			category TEXT,
			importance REAL DEFAULT 0.0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Files table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT UNIQUE,
			content TEXT,
			mime_type TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Config table (persistent config)
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			section TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(section, key)
		)
	`)
	if err != nil {
		return err
	}

	// Session meta
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS session_meta (
			session_key TEXT PRIMARY KEY,
			total_tokens INTEGER DEFAULT 0,
			compaction_count INTEGER DEFAULT 0,
			last_summary TEXT,
			memory_flush_at DATETIME,
			memory_flush_compaction_count INTEGER DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Archive table (optional)
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages_archive (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_key TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT,
			created_at DATETIME,
			archived_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Create indexes
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_key)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_key ON memories(key)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_config_section ON config(section, key)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_session_meta ON session_meta(session_key)`)

	// Events table (for pulse/heartbeat system)
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			content TEXT,
			priority INTEGER DEFAULT 2,
			status TEXT DEFAULT 'pending',
			channel TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			processed_at DATETIME
		)
	`)
	if err != nil {
		return err
	}
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_events_priority ON events(priority)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_events_status ON events(status)`)

	return nil
}

// ============ Messages ============

func (s *Storage) AddMessage(sessionKey, role, content string) error {
	_, err := s.db.Exec(
		"INSERT INTO messages (session_key, role, content) VALUES (?, ?, ?)",
		sessionKey, role, content,
	)
	return err
}

func (s *Storage) GetMessages(sessionKey string, limit int) ([]Message, error) {
	rows, err := s.db.Query(
		"SELECT id, session_key, role, content, created_at FROM messages WHERE session_key = ? ORDER BY created_at DESC LIMIT ?",
		sessionKey, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		rows.Scan(&m.ID, &m.SessionKey, &m.Role, &m.Content, &m.CreatedAt)
		msgs = append(msgs, m)
	}

	// Reverse order (oldest to newest)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (s *Storage) ClearMessages(sessionKey string) error {
	_, err := s.db.Exec("DELETE FROM messages WHERE session_key = ?", sessionKey)
	return err
}

// ============ Session Meta ============

func (s *Storage) GetSessionMeta(sessionKey string) (SessionMeta, error) {
	var meta SessionMeta
	var memoryFlushAt, updatedAt string
	err := s.db.QueryRow(`
		SELECT session_key, total_tokens, compaction_count, last_summary,
		       COALESCE(memory_flush_at, datetime('now')),
		       COALESCE(memory_flush_compaction_count, 0),
		       COALESCE(updated_at, datetime('now'))
		FROM session_meta WHERE session_key = ?
	`, sessionKey).Scan(&meta.SessionKey, &meta.TotalTokens, &meta.CompactionCount, &meta.LastSummary, &memoryFlushAt, &meta.MemoryFlushCompactionCnt, &updatedAt)
	if err == sql.ErrNoRows {
		return SessionMeta{SessionKey: sessionKey}, nil
	}
	if err == nil {
		meta.MemoryFlushAt, _ = time.Parse("2006-01-02 15:04:05", memoryFlushAt)
		meta.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	}
	return meta, err
}

func (s *Storage) UpsertSessionMeta(meta SessionMeta) error {
	_, err := s.db.Exec(`
		INSERT INTO session_meta (session_key, total_tokens, compaction_count, last_summary, memory_flush_at, memory_flush_compaction_count, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(session_key) DO UPDATE SET
			total_tokens=excluded.total_tokens,
			compaction_count=excluded.compaction_count,
			last_summary=excluded.last_summary,
			memory_flush_at=excluded.memory_flush_at,
			memory_flush_compaction_count=excluded.memory_flush_compaction_count,
			updated_at=CURRENT_TIMESTAMP
	`, meta.SessionKey, meta.TotalTokens, meta.CompactionCount, meta.LastSummary, meta.MemoryFlushAt, meta.MemoryFlushCompactionCnt)
	return err
}

func (s *Storage) ArchiveMessages(sessionKey string, beforeID int64) error {
	_, err := s.db.Exec(`
		INSERT INTO messages_archive (session_key, role, content, created_at)
		SELECT session_key, role, content, created_at FROM messages
		WHERE session_key = ? AND id <= ?
	`, sessionKey, beforeID)
	return err
}

// ============ Memories ============

func (s *Storage) SetMemory(key, text, category string) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO memories (key, value, category, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, key, text, category)
	return err
}

func (s *Storage) AddMemory(text, category string, importance float64) (int64, error) {
	result, err := s.db.Exec(`
		INSERT INTO memories (key, value, category, importance, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, generateMemoryKey(), text, category, importance)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func generateMemoryKey() string {
	return fmt.Sprintf("mem_%d", time.Now().UnixNano())
}

func (s *Storage) GetMemory(idOrKey string) (Memory, error) {
	// Try lookup by ID first
	var m Memory
	err := s.db.QueryRow(`
		SELECT id, key, value AS text, category, COALESCE(importance, 0.0), 
		       COALESCE(created_at, datetime('now')), COALESCE(updated_at, datetime('now'))
		FROM memories WHERE id = ? OR key = ?
	`, idOrKey, idOrKey).Scan(&m.ID, &m.Key, &m.Text, &m.Category, &m.Importance, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return Memory{}, fmt.Errorf("memory not found: %s", idOrKey)
	}
	return m, err
}

func (s *Storage) GetMemoriesByCategory(category string) ([]Memory, error) {
	rows, err := s.db.Query(`
		SELECT id, key, value AS text, category, importance, created_at, updated_at
		FROM memories WHERE category = ?
	`, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMemories(rows)
}

func (s *Storage) DeleteMemory(key string) error {
	_, err := s.db.Exec("DELETE FROM memories WHERE key = ?", key)
	return err
}

func (s *Storage) DeleteMemoryByID(id int64) error {
	_, err := s.db.Exec("DELETE FROM memories WHERE id = ?", id)
	return err
}

func (s *Storage) SearchMemories(keyword string) ([]Memory, error) {
	// First, match key exactly
	var m Memory
	err := s.db.QueryRow(`
		SELECT id, key, value AS text, category, COALESCE(importance, 0.0), 
		       COALESCE(created_at, datetime('now')), COALESCE(updated_at, datetime('now'))
		FROM memories WHERE key = ?
	`, keyword).Scan(&m.ID, &m.Key, &m.Text, &m.Category, &m.Importance, &m.CreatedAt, &m.UpdatedAt)
	if err == nil {
		return []Memory{m}, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// Then fuzzy search by value
	rows, err := s.db.Query(`
		SELECT id, key, value AS text, category, COALESCE(importance, 0.0), 
		       COALESCE(created_at, datetime('now')), COALESCE(updated_at, datetime('now'))
		FROM memories WHERE value LIKE ? OR category LIKE ?
		ORDER BY importance DESC, created_at DESC LIMIT ?
	`, "%"+keyword+"%", "%"+keyword+"%", 10)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMemories(rows)
}

func scanMemories(rows *sql.Rows) ([]Memory, error) {
	var memories []Memory
	for rows.Next() {
		var m Memory
		rows.Scan(&m.ID, &m.Key, &m.Text, &m.Category, &m.Importance, &m.CreatedAt, &m.UpdatedAt)
		memories = append(memories, m)
	}
	return memories, nil
}

func (s *Storage) GetAllMemories(limit int) ([]Memory, error) {
	rows, err := s.db.Query(`
		SELECT id, key, value AS text, category, importance, created_at, updated_at
		FROM memories ORDER BY importance DESC, created_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemories(rows)
}

func memoryToJSON(m Memory) Memory {
	return m
}

// ============ Files ============

func (s *Storage) AddFile(path, content, mimeType string) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO files (path, content, mime_type) VALUES (?, ?, ?)",
		path, content, mimeType,
	)
	return err
}

func (s *Storage) GetFile(path string) (*FileRecord, error) {
	var f FileRecord
	err := s.db.QueryRow(
		"SELECT id, path, content, mime_type, created_at FROM files WHERE path = ?",
		path,
	).Scan(&f.ID, &f.Path, &f.Content, &f.MimeType, &f.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &f, err
}

func (s *Storage) ListFiles() ([]FileRecord, error) {
	rows, err := s.db.Query(
		"SELECT id, path, content, mime_type, created_at FROM files ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var f FileRecord
		rows.Scan(&f.ID, &f.Path, &f.Content, &f.MimeType, &f.CreatedAt)
		files = append(files, f)
	}
	return files, nil
}

// ============ Tools ============

func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) Stats() (map[string]int, error) {
	stats := make(map[string]int)

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	stats["messages"] = count

	s.db.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count)
	stats["memories"] = count

	s.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&count)
	stats["files"] = count

	return stats, nil
}

// Import from MD-style data (simplified)
func (s *Storage) ImportMemory(key, value, category string) error {
	return s.SetMemory(key, value, category)
}

// Export memories to JSON
func (s *Storage) ExportMemories() ([]byte, error) {
	rows, err := s.db.Query("SELECT id, key, value, category, updated_at FROM memories")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type ExportMem struct {
		ID        int64     `json:"id"`
		Key       string    `json:"key"`
		Value     string    `json:"value"`
		Category  string    `json:"category"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	var memories []ExportMem
	for rows.Next() {
		var m ExportMem
		rows.Scan(&m.ID, &m.Key, &m.Value, &m.Category, &m.UpdatedAt)
		memories = append(memories, m)
	}

	return json.MarshalIndent(memories, "", "  ")
}

// ============ Config (persistence) ============

// SetConfig writes a config entry to the database
func (s *Storage) SetConfig(section, key, value string) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO config (section, key, value, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)",
		section, key, value,
	)
	return err
}

// GetConfig reads a config value
func (s *Storage) GetConfig(section, key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM config WHERE section = ? AND key = ?", section, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// GetConfigSection reads all config values in a section
func (s *Storage) GetConfigSection(section string) (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM config WHERE section = ?", section)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	config := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		config[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return config, nil
}

// ConfigExists checks whether a section exists
func (s *Storage) ConfigExists(section string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM config WHERE section = ?", section).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteConfig deletes a config entry
func (s *Storage) DeleteConfig(section, key string) error {
	_, err := s.db.Exec("DELETE FROM config WHERE section = ? AND key = ?", section, key)
	return err
}

// ClearConfigSection clears a section
func (s *Storage) ClearConfigSection(section string) error {
	_, err := s.db.Exec("DELETE FROM config WHERE section = ?", section)
	return err
}

// ExportConfig exports all configs as JSON
func (s *Storage) ExportConfig() ([]byte, error) {
	rows, err := s.db.Query("SELECT section, key, value FROM config")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type ExportConfig struct {
		Section string `json:"section"`
		Key     string `json:"key"`
		Value   string `json:"value"`
	}

	var configs []ExportConfig
	for rows.Next() {
		var c ExportConfig
		rows.Scan(&c.Section, &c.Key, &c.Value)
		configs = append(configs, c)
	}

	return json.MarshalIndent(configs, "", "  ")
}

// ============ Events (Pulse/Heartbeat System) ============

// AddEvent adds a new event to the database
func (s *Storage) AddEvent(title, content string, priority EventPriority, channel string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO events (title, content, priority, status, channel) VALUES (?, ?, ?, 'pending', ?)",
		title, content, priority, channel,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetPendingEvents returns pending events ordered by priority (0 first)
func (s *Storage) GetPendingEvents(limit int) ([]Event, error) {
	rows, err := s.db.Query(`
		SELECT id, title, content, priority, status, channel, created_at, processed_at
		FROM events
		WHERE status = 'pending'
		ORDER BY priority ASC, created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var processedAt sql.NullTime
		if err := rows.Scan(&e.ID, &e.Title, &e.Content, &e.Priority, &e.Status, &e.Channel, &e.CreatedAt, &processedAt); err != nil {
			return nil, err
		}
		if processedAt.Valid {
			e.ProcessedAt = &processedAt.Time
		}
		events = append(events, e)
	}
	return events, nil
}

// GetNextEvent returns the highest priority pending event
func (s *Storage) GetNextEvent() (*Event, error) {
	var e Event
	var processedAt sql.NullTime
	err := s.db.QueryRow(`
		SELECT id, title, content, priority, status, channel, created_at, processed_at
		FROM events
		WHERE status = 'pending'
		ORDER BY priority ASC, created_at ASC
		LIMIT 1
	`).Scan(&e.ID, &e.Title, &e.Content, &e.Priority, &e.Status, &e.Channel, &e.CreatedAt, &processedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if processedAt.Valid {
		e.ProcessedAt = &processedAt.Time
	}
	return &e, nil
}

// UpdateEventStatus updates an event's status
func (s *Storage) UpdateEventStatus(id int64, status string) error {
	_, err := s.db.Exec(
		"UPDATE events SET status = ?, processed_at = CURRENT_TIMESTAMP WHERE id = ?",
		status, id,
	)
	return err
}

// GetEventCount returns counts by status
func (s *Storage) GetEventCount() (map[string]int, error) {
	rows, err := s.db.Query(`
		SELECT status, COUNT(*) as count
		FROM events
		GROUP BY status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, nil
}

// ClearOldEvents removes completed/dismissed events older than specified hours
func (s *Storage) ClearOldEvents(olderThanHours int) error {
	_, err := s.db.Exec(`
		DELETE FROM events
		WHERE status IN ('completed', 'dismissed')
		AND processed_at < datetime('now', '-? hours')
	`, olderThanHours)
	return err
}

// Exec executes a raw SQL query
func (s *Storage) Exec(query string, args ...interface{}) (interface{}, error) {
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Query executes a raw SQL query and returns rows
func (s *Storage) Query(query string, args ...interface{}) (interface{}, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}
