// Session management for OpenClaw-Go
// Supports multiple sessions with their own context and state

package agent

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gliderlab/cogate/storage"
)

// Session represents a conversation session
type Session struct {
	ID           string
	Key          string      // Unique session key (e.g., "main", "cron:job-123", "telegram:user-456")
	AgentID      string      // Agent instance ID
	Messages     []Message   // Conversation history
	CreatedAt    time.Time
	UpdatedAt    time.Time
	TotalTokens  int
	CompactionCount int
	ContextTokens int
	IsActive     bool
	Metadata     map[string]interface{}
	mu           sync.RWMutex
}

// SessionManager manages multiple sessions
type SessionManager struct {
	store      *storage.Storage
	sessions   map[string]*Session
	mu         sync.RWMutex
	defaultAgentID string
}

// NewSessionManager creates a new session manager
func NewSessionManager(store *storage.Storage, defaultAgentID string) *SessionManager {
	return &SessionManager{
		store:           store,
		sessions:        make(map[string]*Session),
		defaultAgentID:  defaultAgentID,
	}
}

// CreateSession creates a new session
func (sm *SessionManager) CreateSession(key, agentID string) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if session already exists
	if session, ok := sm.sessions[key]; ok {
		return session, nil
	}

	session := &Session{
		ID:              fmt.Sprintf("sess-%d", time.Now().UnixMilli()),
		Key:             key,
		AgentID:         agentID,
		Messages:        make([]Message, 0),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		TotalTokens:     0,
		ContextTokens:   0,
		IsActive:        true,
		Metadata:        make(map[string]interface{}),
	}

	sm.sessions[key] = session

	// Persist to database
	if sm.store != nil {
		sm.saveSession(session)
	}

	log.Printf("[Session] Created session: %s (agent: %s)", key, agentID)
	return session, nil
}

// GetSession returns a session by key
func (sm *SessionManager) GetSession(key string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, ok := sm.sessions[key]
	return session, ok
}

// GetOrCreateSession returns existing session or creates new one
func (sm *SessionManager) GetOrCreateSession(key, agentID string) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if exists
	if session, ok := sm.sessions[key]; ok {
		return session, nil
	}

	// Create new session
	return sm.CreateSession(key, agentID)
}

// AddMessage adds a message to a session
func (sm *SessionManager) AddMessage(key string, msg Message) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if !ok {
		return fmt.Errorf("session not found: %s", key)
	}

	session.Messages = append(session.Messages, msg)
	session.UpdatedAt = time.Now()

	// Persist to database periodically
	if len(session.Messages)%10 == 0 && sm.store != nil {
		sm.saveSession(session)
	}

	return nil
}

// GetMessages returns all messages in a session
func (sm *SessionManager) GetMessages(key string) ([]Message, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, ok := sm.sessions[key]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", key)
	}

	return session.Messages, nil
}

// ClearSession clears a session's messages
func (sm *SessionManager) ClearSession(key string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if !ok {
		return fmt.Errorf("session not found: %s", key)
	}

	session.Messages = make([]Message, 0)
	session.UpdatedAt = time.Now()
	session.CompactionCount++

	if sm.store != nil {
		sm.saveSession(session)
	}

	log.Printf("[Session] Cleared session: %s", key)
	return nil
}

// ListSessions returns all active sessions
func (sm *SessionManager) ListSessions() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessions := make([]*Session, 0, len(sm.sessions))
	for _, session := range sm.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// RemoveSession removes a session
func (sm *SessionManager) RemoveSession(key string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.sessions[key]; !ok {
		return fmt.Errorf("session not found: %s", key)
	}

	delete(sm.sessions, key)
	log.Printf("[Session] Removed session: %s", key)
	return nil
}

// saveSession persists session to database
func (sm *SessionManager) saveSession(session *Session) error {
	if sm.store == nil {
		return nil
	}

	// Save session metadata using storage methods
	// For now, we'll just update the session_meta table
	_, err := sm.store.Exec(`
		INSERT OR REPLACE INTO session_meta 
		(session_key, total_tokens, compaction_count, updated_at)
		VALUES (?, ?, ?, ?)
	`, session.Key, session.TotalTokens, session.CompactionCount, session.UpdatedAt)

	return err
}

// LoadSessions loads sessions from database
func (sm *SessionManager) LoadSessions() error {
	if sm.store == nil {
		return nil
	}

	// Use a simple approach - for now just log that we'd load sessions
	log.Printf("[Session] LoadSessions called (database loading not fully implemented)")
	return nil
}

// SessionRPCClient represents a client for making RPC calls to other agents
type SessionRPCClient struct {
	agentID  string
	address  string
	client   interface{} // RPC client
	mu       sync.RWMutex
	connected bool
}

// NewSessionRPCClient creates a new session RPC client
func NewSessionRPCClient(agentID, address string) *SessionRPCClient {
	return &SessionRPCClient{
		agentID: agentID,
		address: address,
		connected: false,
	}
}

// Connect connects to the agent
func (c *SessionRPCClient) Connect() error {
	// This would use net/rpc to connect
	// For now, just mark as connected
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = true
	return nil
}

// IsConnected returns connection status
func (c *SessionRPCClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// SessionManagerWithRPC extends SessionManager with RPC capabilities
type SessionManagerWithRPC struct {
	*SessionManager
	rpcClients map[string]*SessionRPCClient // agentID -> client
	mu         sync.RWMutex
}

// NewSessionManagerWithRPC creates a new session manager with RPC
func NewSessionManagerWithRPC(store *storage.Storage, defaultAgentID string) *SessionManagerWithRPC {
	sm := NewSessionManager(store, defaultAgentID)
	return &SessionManagerWithRPC{
		SessionManager: sm,
		rpcClients:    make(map[string]*SessionRPCClient),
	}
}

// RegisterAgentRPC registers an RPC client for an agent
func (sm *SessionManagerWithRPC) RegisterAgentRPC(agentID, address string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	client := NewSessionRPCClient(agentID, address)
	if err := client.Connect(); err != nil {
		log.Printf("[SessionRPC] Failed to connect to agent %s: %v", agentID, err)
		return
	}

	sm.rpcClients[agentID] = client
	log.Printf("[SessionRPC] Registered agent: %s at %s", agentID, address)
}

// GetAgentRPC returns an RPC client for an agent
func (sm *SessionManagerWithRPC) GetAgentRPC(agentID string) (*SessionRPCClient, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	client, ok := sm.rpcClients[agentID]
	return client, ok
}

// ForwardToAgent forwards a message to another agent via RPC
func (sm *SessionManagerWithRPC) ForwardToAgent(targetAgentID string, messages []Message) (string, error) {
	sm.mu.RLock()
	client, ok := sm.rpcClients[targetAgentID]
	sm.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("agent not found: %s", targetAgentID)
	}

	if !client.IsConnected() {
		return "", fmt.Errorf("agent not connected: %s", targetAgentID)
	}

	// In a real implementation, this would make an RPC call
	// For now, return an error indicating RPC not implemented
	return "", fmt.Errorf("RPC forwarding not implemented yet")
}

// CreateSessionForChannel creates a session for a specific channel
func (sm *SessionManager) CreateSessionForChannel(channelType, channelID, agentID string) (*Session, error) {
	key := fmt.Sprintf("%s:%s", channelType, channelID)
	return sm.CreateSession(key, agentID)
}

// SessionInfo returns basic session info for listing
type SessionInfo struct {
	Key             string    `json:"key"`
	AgentID         string    `json:"agentId"`
	MessageCount    int       `json:"messageCount"`
	TotalTokens     int       `json:"totalTokens"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	IsActive        bool      `json:"isActive"`
}

// GetSessionInfo returns session info
func (sm *SessionManager) GetSessionInfo(key string) (*SessionInfo, error) {
	session, ok := sm.sessions[key]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", key)
	}

	return &SessionInfo{
		Key:          session.Key,
		AgentID:      session.AgentID,
		MessageCount: len(session.Messages),
		TotalTokens:  session.TotalTokens,
		CreatedAt:    session.CreatedAt,
		UpdatedAt:    session.UpdatedAt,
		IsActive:     session.IsActive,
	}, nil
}

// ListSessionInfos returns all session infos
func (sm *SessionManager) ListSessionInfos() []SessionInfo {
	sessions := sm.ListSessions()
	infos := make([]SessionInfo, 0, len(sessions))

	for _, session := range sessions {
		infos = append(infos, SessionInfo{
			Key:          session.Key,
			AgentID:      session.AgentID,
			MessageCount: len(session.Messages),
			TotalTokens:  session.TotalTokens,
			CreatedAt:    session.CreatedAt,
			UpdatedAt:    session.UpdatedAt,
			IsActive:     session.IsActive,
		})
	}

	return infos
}

// GetOrCreateChannelSession gets or creates a session for a channel
func (sm *SessionManager) GetOrCreateChannelSession(channelType, channelID, agentID string) (*Session, error) {
	key := fmt.Sprintf("%s:%s", channelType, channelID)
	return sm.GetOrCreateSession(key, agentID)
}
