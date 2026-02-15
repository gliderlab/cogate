// Package channels - Channel adapter system for communication platforms
// Provides plugin-based channel integration with hot-reload support
package channels

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// ChannelType represents the type of communication channel
type ChannelType string

const (
	ChannelTelegram  ChannelType = "telegram"
	ChannelWhatsApp  ChannelType = "whatsapp"
	ChannelSlack     ChannelType = "slack"
	ChannelDiscord   ChannelType = "discord"
	ChannelWebChat   ChannelType = "webchat"
)

// ChannelInfo contains metadata about a channel
type ChannelInfo struct {
	Name        string                 `json:"name"`
	Type        ChannelType            `json:"type"`
	Version     string                 `json:"version"`
	Description string                 `json:"description"`
	Author      string                 `json:"author"`
	Capabilities []string              `json:"capabilities,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// ChannelLoader defines the interface for channel plugins
type ChannelLoader interface {
	// ChannelInfo returns metadata about the channel
	ChannelInfo() ChannelInfo

	// Initialize is called when the channel is loaded
	Initialize(config map[string]interface{}) error

	// Start starts the channel listener (webhook/polling)
	Start() error

	// Stop stops the channel listener
	Stop() error

	// SendMessage sends a message to a chat/user
	SendMessage(req *SendMessageRequest) (*SendMessageResponse, error)

	// HandleWebhook handles incoming webhook requests
	HandleWebhook(w http.ResponseWriter, r *http.Request)

	// HealthCheck verifies the channel is working
	HealthCheck() error
}

// SendMessageRequest represents a message to send
type SendMessageRequest struct {
	ChatID    int64         `json:"chatId"`
	Text      string        `json:"text"`
	ParseMode string        `json:"parseMode,omitempty"`
	Media     string        `json:"media,omitempty"`
	Buttons   [][]Button    `json:"buttons,omitempty"`
	ReplyTo   int64         `json:"replyToMessageId,omitempty"`
	ThreadID  int64         `json:"messageThreadId,omitempty"`
}

// Button represents an inline button
type Button struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

// SendMessageResponse represents the response from sending a message
type SendMessageResponse struct {
	OK         bool   `json:"ok"`
	MessageID  int64  `json:"messageId"`
	ChatID     int64  `json:"chatId"`
	Timestamp  int64  `json:"timestamp"`
	Error      string `json:"error,omitempty"`
}

// ChannelMessage represents an incoming channel message
type ChannelMessage struct {
	ID        string                 `json:"id"`
	Channel   ChannelType            `json:"channel"`
	ChatID    int64                  `json:"chatId"`
	UserID    int64                  `json:"userId"`
	Username  string                 `json:"username"`
	Text      string                 `json:"text"`
	Timestamp int64                  `json:"timestamp"`
	ThreadID  int64                  `json:"threadId,omitempty"`
	Media     *ChannelMedia          `json:"media,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ChannelMedia represents media attachments
type ChannelMedia struct {
	Type      string `json:"type"`       // image/audio/video/document/sticker
	URL       string `json:"url"`
	Caption   string `json:"caption"`
	MimeType  string `json:"mimeType"`
	Duration  int    `json:"duration,omitempty"` // for audio/video
	Width     int    `json:"width,omitempty"`    // for images/video
	Height    int    `json:"height,omitempty"`   // for images/video
}

// ChannelContext holds execution context for channels
type ChannelContext struct {
	AgentName   string
	SessionID   string
	Workspace   string
	GatewayURL  string
	UserID      string
	Channel     ChannelType
	Timestamp   int64
	IsGroup     bool
	GroupID     int64
	ThreadID    int64
	MessageID   int64
	ReplyToID   int64
	Extra       map[string]interface{}
}

// ChannelResult represents a channel operation result
type ChannelResult struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// ChannelAdapter is the main adapter that manages channel plugins
type ChannelAdapter struct {
	mu        sync.RWMutex
	channels  map[ChannelType]ChannelLoader
	registry  *ChannelRegistry
	config    ChannelAdapterConfig
	agentRPC  AgentRPCInterface
}

// AgentRPCInterface defines the interface for agent communication
type AgentRPCInterface interface {
	Chat(messages []Message) (string, error)
	GetStats() (map[string]int, error)
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChannelAdapterConfig holds adapter configuration
type ChannelAdapterConfig struct {
	Enabled      bool              `json:"enabled"`
	Channels     map[ChannelType]bool `json:"channels"`
	WebhookPath  string            `json:"webhookPath"`
	WebhookHost  string            `json:"webhookHost"`
	WebhookPort  int               `json:"webhookPort"`
	Polling      bool              `json:"pollingEnabled"`
	PollingLimit int               `json:"pollingLimit"`
	MaxRetries   int               `json:"maxRetries"`
	Timeout      int               `json:"defaultTimeoutSeconds"`
}

// DefaultChannelAdapterConfig returns default configuration
func DefaultChannelAdapterConfig() ChannelAdapterConfig {
	return ChannelAdapterConfig{
		Enabled:      true,
		Channels:     make(map[ChannelType]bool),
		WebhookPath:  "/webhook",
		WebhookHost:  "127.0.0.1",
		WebhookPort:  8787,
		Polling:      false,
		PollingLimit: 100,
		MaxRetries:   3,
		Timeout:      30,
	}
}

// NewChannelAdapter creates a new channel adapter
func NewChannelAdapter(cfg ChannelAdapterConfig, agentRPC AgentRPCInterface) *ChannelAdapter {
	return &ChannelAdapter{
		channels:  make(map[ChannelType]ChannelLoader),
		registry:  NewChannelRegistry(),
		config:   cfg,
		agentRPC: agentRPC,
	}
}

// RegisterChannel registers a channel with the adapter
func (a *ChannelAdapter) RegisterChannel(channel ChannelLoader) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	info := channel.ChannelInfo()
	channelType := info.Type

	// Check if channel already registered
	if _, exists := a.channels[channelType]; exists {
		return fmt.Errorf("channel %s already registered", channelType)
	}

	// Initialize the channel
	if err := channel.Initialize(info.Config); err != nil {
		return fmt.Errorf("failed to initialize channel %s: %w", channelType, err)
	}

	a.channels[channelType] = channel
	a.registry.Add(info)

	log.Printf("✅ registered channel: %s v%s", info.Name, info.Version)
	return nil
}

// UnregisterChannel removes a channel from the adapter
func (a *ChannelAdapter) UnregisterChannel(channelType ChannelType) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	channel, exists := a.channels[channelType]
	if !exists {
		return fmt.Errorf("channel %s not found", channelType)
	}

	// Call stop
	if err := channel.Stop(); err != nil {
		log.Printf("⚠️ channel %s stop error: %v", channelType, err)
	}

	delete(a.channels, channelType)
	a.registry.Remove(string(channelType))

	return nil
}

// StartChannel starts a specific channel
func (a *ChannelAdapter) StartChannel(channelType ChannelType) error {
	a.mu.RLock()
	channel, exists := a.channels[channelType]
	a.mu.RUnlock()

	if !exists {
		return fmt.Errorf("channel %s not found", channelType)
	}

	return channel.Start()
}

// StartAllChannels starts all registered channels
func (a *ChannelAdapter) StartAllChannels() error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for channelType, channel := range a.channels {
		if err := channel.Start(); err != nil {
			log.Printf("⚠️ failed to start channel %s: %v", channelType, err)
			continue
		}
		log.Printf("🚀 started channel: %s", channelType)
	}

	return nil
}

// StopAllChannels stops all channels
func (a *ChannelAdapter) StopAllChannels() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for channelType, channel := range a.channels {
		if err := channel.Stop(); err != nil {
			log.Printf("⚠️ failed to stop channel %s: %v", channelType, err)
		}
	}

	a.channels = make(map[ChannelType]ChannelLoader)
	return nil
}

// SendMessage sends a message through a channel
func (a *ChannelAdapter) SendMessage(channelType ChannelType, req *SendMessageRequest) (*SendMessageResponse, error) {
	a.mu.RLock()
	channel, exists := a.channels[channelType]
	a.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("channel %s not found", channelType)
	}

	return channel.SendMessage(req)
}

// HandleWebhook routes a webhook request to the appropriate channel
func (a *ChannelAdapter) HandleWebhook(channelType ChannelType, w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	channel, exists := a.channels[channelType]
	a.mu.RUnlock()

	if !exists {
		http.Error(w, "channel not found", 404)
		return
	}

	channel.HandleWebhook(w, r)
}

// ProcessMessage processes an incoming channel message
func (a *ChannelAdapter) ProcessMessage(msg *ChannelMessage) (*ChannelResult, error) {
	if a.agentRPC == nil {
		return nil, fmt.Errorf("agent RPC not configured")
	}

	// Convert to agent message format
	messages := []Message{
		{
			Role:    "system",
			Content: fmt.Sprintf("You are an AI assistant. Received message from %s channel, chat ID: %d, user: @%s", 
				msg.Channel, msg.ChatID, msg.Username),
		},
		{
			Role:    "user",
			Content: msg.Text,
		},
	}

	// Call agent
	response, err := a.agentRPC.Chat(messages)
	if err != nil {
		return &ChannelResult{
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now().Unix(),
		}, err
	}

	// Send response back to channel
	sendReq := &SendMessageRequest{
		ChatID: msg.ChatID,
		Text:   response,
	}

	if msg.ThreadID > 0 {
		sendReq.ThreadID = msg.ThreadID
	}

	resp, err := a.SendMessage(msg.Channel, sendReq)
	if err != nil {
		return &ChannelResult{
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now().Unix(),
		}, err
	}

	return &ChannelResult{
		Success:   true,
		Data:      resp,
		Timestamp: time.Now().Unix(),
	}, nil
}

// ListChannels returns the list of registered channel types
func (a *ChannelAdapter) ListChannels() []ChannelType {
	a.mu.RLock()
	defer a.mu.RUnlock()

	types := make([]ChannelType, 0, len(a.channels))
	for t := range a.channels {
		types = append(types, t)
	}
	return types
}

// HasChannel checks if a channel is registered
func (a *ChannelAdapter) HasChannel(channelType ChannelType) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, exists := a.channels[channelType]
	return exists
}

// GetChannelInfo returns info about a specific channel
func (a *ChannelAdapter) GetChannelInfo(channelType ChannelType) (*ChannelInfo, error) {
	a.mu.RLock()
	channel, exists := a.channels[channelType]
	a.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("channel %s not found", channelType)
	}

	info := channel.ChannelInfo()
	return &info, nil
}

// GetRegistry returns the channel registry
func (a *ChannelAdapter) GetRegistry() *ChannelRegistry {
	return a.registry
}

// HealthCheck performs health check on all channels
func (a *ChannelAdapter) HealthCheck() map[ChannelType]error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	results := make(map[ChannelType]error)
	for channelType, channel := range a.channels {
		if err := channel.HealthCheck(); err != nil {
			results[channelType] = err
		}
	}

	return results
}

// ChannelRegistry maintains a registry of all loaded channels
type ChannelRegistry struct {
	mu       sync.RWMutex
	channels map[string]ChannelInfo
}

func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{
		channels: make(map[string]ChannelInfo),
	}
}

func (r *ChannelRegistry) Add(info ChannelInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[string(info.Type)] = info
}

func (r *ChannelRegistry) Remove(channelType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.channels, channelType)
}

func (r *ChannelRegistry) Get(channelType string) (ChannelInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.channels[channelType]
	return info, ok
}

func (r *ChannelRegistry) List() []ChannelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]ChannelInfo, 0, len(r.channels))
	for _, info := range r.channels {
		infos = append(infos, info)
	}
	return infos
}

// NewResult creates a successful channel result
func NewChannelResult(data interface{}) *ChannelResult {
	return &ChannelResult{
		Success:   true,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}
}

// NewChannelErrorResult creates an error channel result
func NewChannelErrorResult(err error) *ChannelResult {
	return &ChannelResult{
		Success:   false,
		Error:     err.Error(),
		Timestamp: time.Now().Unix(),
	}
}

// GetChannelDocumentation returns markdown documentation for all channels
func (a *ChannelAdapter) GetChannelDocumentation() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	docs := "# Channel Documentation\n\n"
	for _, channel := range a.channels {
		info := channel.ChannelInfo()
		docs += fmt.Sprintf("## %s (%s)\n", info.Name, info.Type)
		docs += fmt.Sprintf("Version: %s\n\n", info.Version)
		docs += fmt.Sprintf("Description: %s\n\n", info.Description)
		if len(info.Capabilities) > 0 {
			docs += fmt.Sprintf("Capabilities: %v\n\n", info.Capabilities)
		}
	}

	return docs
}