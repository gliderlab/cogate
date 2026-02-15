package channels

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gliderlab/cogate/rpcproto"
)

// TelegramBot implements the ChannelLoader interface for Telegram
type TelegramBot struct {
	token       string
	baseURL     string
	client      *http.Client
	agentRPC    AgentRPCInterface
	running     bool
	stopCh      chan struct{}
	// Greeting configuration
	greetingEnabled bool
	greetingText   string
	greetedUsers   map[int64]bool // Track users who have received greeting
}

// NewTelegramBot creates a new Telegram bot channel plugin
func NewTelegramBot(token string, agentRPC AgentRPCInterface) *TelegramBot {
	return &TelegramBot{
		token:           token,
		baseURL:         fmt.Sprintf("https://api.telegram.org/bot%s", token),
		client:          &http.Client{Timeout: 30 * time.Second},
		agentRPC:        agentRPC,
		stopCh:          make(chan struct{}),
		greetingEnabled: true,
		greetingText:    "Hello! I'm OpenClaw-Go 🤖. How can I help you today?",
		greetedUsers:    make(map[int64]bool),
	}
}

// SetGreeting configures the greeting message
func (b *TelegramBot) SetGreeting(enabled bool, text string) {
	b.greetingEnabled = enabled
	b.greetingText = text
}

// ChannelInfo returns metadata about this channel
func (b *TelegramBot) ChannelInfo() ChannelInfo {
	return ChannelInfo{
		Name:        "Telegram Bot",
		Type:        ChannelType("telegram"),
		Version:     "1.0.0",
		Description: "Telegram Bot API integration with webhook support",
		Author:      "OpenClaw-Go",
		Capabilities: []string{
			"messages",
			"webhook",
			"polling",
			"media",
			"buttons",
			"reactions",
		},
		Config: map[string]interface{}{
			"token":            b.token,
			"webhookPath":      "/telegram/webhook",
			"streamMode":       "partial",
			"linkPreview":      true,
			"textChunkLimit":   4000,
			"mediaMaxMb":       5,
			"dmPolicy":         "pairing",
			"groupPolicy":      "allowlist",
			"requireMention":   true,
		},
	}
}

// Initialize configures the channel
func (b *TelegramBot) Initialize(config map[string]interface{}) error {
	if token, ok := config["token"].(string); ok {
		b.token = token
		b.baseURL = fmt.Sprintf("https://api.telegram.org/bot%s", token)
	}
	return nil
}

// Start starts the Telegram bot webhook listener
func (b *TelegramBot) Start() error {
	if b.running {
		return nil
	}

	log.Printf("🚀 Starting Telegram bot...")
	b.running = true
	return nil
}

// Stop stops the Telegram bot
func (b *TelegramBot) Stop() error {
	if !b.running {
		return nil
	}

	close(b.stopCh)
	b.running = false
	log.Printf("🛑 Telegram bot stopped")
	return nil
}

// SendMessage sends a message to a Telegram chat
func (b *TelegramBot) SendMessage(req *SendMessageRequest) (*SendMessageResponse, error) {
	// Truncate if too long (Telegram has a 4096 character limit)
	text := req.Text
	if len(text) > 4096 {
		text = text[:4096] + "... (truncated)"
	}

	apiReq := map[string]interface{}{
		"chat_id":    req.ChatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	if req.ReplyTo > 0 {
		apiReq["reply_to_message_id"] = req.ReplyTo
	}

	if req.ThreadID > 0 {
		apiReq["message_thread_id"] = req.ThreadID
	}

	// Handle buttons
	if len(req.Buttons) > 0 {
		inlineKeyboard := make([][]map[string]string, 0, len(req.Buttons))
		for _, row := range req.Buttons {
			buttonRow := make([]map[string]string, 0, len(row))
			for _, btn := range row {
				buttonRow = append(buttonRow, map[string]string{
					"text":          btn.Text,
					"callback_data": btn.CallbackData,
				})
			}
			inlineKeyboard = append(inlineKeyboard, buttonRow)
		}
		apiReq["reply_markup"] = map[string]interface{}{
			"inline_keyboard": inlineKeyboard,
		}
	}

	payload, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := b.baseURL + "/sendMessage"
	httpReq, err := http.NewRequest("POST", url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var sendResp struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code,omitempty"`
		Description string `json:"description,omitempty"`
		Result      struct {
			MessageID int `json:"message_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			Date int `json:"date"`
		} `json:"result"`
	}

	json.Unmarshal(body, &sendResp)

	if !sendResp.OK {
		return &SendMessageResponse{
			OK:        false,
			ChatID:    req.ChatID,
			Error:     sendResp.Description,
			Timestamp: time.Now().Unix(),
		}, nil
	}

	return &SendMessageResponse{
		OK:        true,
		MessageID: int64(sendResp.Result.MessageID),
		ChatID:    sendResp.Result.Chat.ID,
		Timestamp: int64(sendResp.Result.Date),
	}, nil
}

// HandleWebhook handles incoming Telegram webhook requests
func (b *TelegramBot) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading webhook body: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var update IncomingUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		log.Printf("Error parsing webhook JSON: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Process message if present
	if update.Message.Text != "" {
		go b.processMessage(update.Message)
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok": true}`)
}

// processMessage handles an incoming Telegram message
func (b *TelegramBot) processMessage(TgMessage IncomingMessage) {
	if TgMessage.Text == "" {
		return
	}

	chatID := int64(TgMessage.Chat.ID)
	username := TgMessage.From.Username
	userID := int64(TgMessage.From.ID)

	log.Printf("📨 Received message from %s (@%s): %s", 
		TgMessage.From.FirstName, username, TgMessage.Text)

	// Send greeting to new users (not /start command)
	if b.greetingEnabled && !strings.HasPrefix(TgMessage.Text, "/") {
		if !b.greetedUsers[userID] {
			b.greetedUsers[userID] = true
			// Send greeting after a short delay
			go func() {
				time.Sleep(500 * time.Millisecond)
				b.sendSimpleMessage(chatID, b.greetingText)
			}()
		}
	}

	// Handle commands
	if strings.HasPrefix(TgMessage.Text, "/start") {
		b.greetedUsers[userID] = true // Mark as greeted
		b.sendSimpleMessage(chatID, fmt.Sprintf("Hello %s! I'm OpenClaw-Go Telegram Bot. Send me a message!", TgMessage.From.FirstName))
		return
	}

	if strings.HasPrefix(TgMessage.Text, "/help") {
		b.sendSimpleMessage(chatID, "Commands:\n/start - Start bot\n/help - Help\n/stats - Stats\nAny message for AI assistance")
		return
	}

	if strings.HasPrefix(TgMessage.Text, "/reset") {
		// Reset greeting status for this user
		delete(b.greetedUsers, userID)
		b.sendSimpleMessage(chatID, "Greeting status reset! You'll receive a greeting on your next message.")
		return
	}

	if strings.HasPrefix(TgMessage.Text, "/stats") {
		stats, err := b.agentRPC.GetStats()
		if err != nil {
			b.sendSimpleMessage(chatID, fmt.Sprintf("Error: %v", err))
			return
		}
		b.sendSimpleMessage(chatID, fmt.Sprintf("📊 Stats:\nMessages: %d\nMemories: %d", stats["messages"], stats["memories"]))
		return
	}

	// Send to agent
	messages := []Message{
		{
			Role:    "system",
			Content: fmt.Sprintf("You are an AI assistant. User @%s (ID: %d) sent a message in Telegram chat %d.", 
				username, userID, chatID),
		},
		{
			Role:    "user",
			Content: TgMessage.Text,
		},
	}

	response, err := b.agentRPC.Chat(messages)
	if err != nil {
		log.Printf("Agent error: %v", err)
		b.sendSimpleMessage(chatID, "Sorry, I encountered an error.")
		return
	}

	b.sendSimpleMessage(chatID, response)
}

// sendSimpleMessage sends a text message to a chat
func (b *TelegramBot) sendSimpleMessage(chatID int64, text string) {
	if len(text) > 4096 {
		text = text[:4096] + "... (truncated)"
	}

	apiReq := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	payload, _ := json.Marshal(apiReq)
	url := b.baseURL + "/sendMessage"
	
	b.client.Post(url, "application/json", strings.NewReader(string(payload)))
}

// HealthCheck verifies the bot is working
func (b *TelegramBot) HealthCheck() error {
	// Test API connection
	url := b.baseURL + "/getMe"
	resp, err := b.client.Get(url)
	if err != nil {
		return fmt.Errorf("API connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	return nil
}

// IncomingUpdate represents an incoming Telegram update
type IncomingUpdate struct {
	UpdateID int            `json:"update_id"`
	Message  IncomingMessage `json:"message"`
}

// IncomingMessage represents an incoming Telegram message
type IncomingMessage struct {
	MessageID int       `json:"message_id"`
	From      UserInfo `json:"from"`
	Chat      ChatInfo `json:"chat"`
	Date      int      `json:"date"`
	Text      string   `json:"text"`
	ThreadID  int      `json:"message_thread_id,omitempty"`
}

// UserInfo represents Telegram user info
type UserInfo struct {
	ID           int    `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Username     string `json:"username"`
	LanguageCode string `json:"language_code"`
}

// ChatInfo represents Telegram chat info
type ChatInfo struct {
	ID        int    `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	Type      string `json:"type"`
}

// RegisterAsPlugin registers this bot as a channel plugin with the adapter
func (b *TelegramBot) RegisterAsPlugin(adapter *ChannelAdapter) error {
	return adapter.RegisterChannel(b)
}

// CreateFromEnv creates a Telegram bot from environment variables
func CreateFromEnv(agentRPC AgentRPCInterface) (*TelegramBot, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN not set")
	}
	return NewTelegramBot(token, agentRPC), nil
}

// DefaultRPCClient implements AgentRPCInterface using RPC
type DefaultRPCClient struct {
	client *rpc.Client
}

// NewDefaultRPCClient creates a new RPC client
func NewDefaultRPCClient(client *rpc.Client) *DefaultRPCClient {
	return &DefaultRPCClient{client: client}
}

// Chat sends a chat request to the agent
func (c *DefaultRPCClient) Chat(messages []Message) (string, error) {
	// Convert to rpcproto format
	rpcMessages := make([]rpcproto.Message, 0, len(messages))
	for _, m := range messages {
		rpcMessages = append(rpcMessages, rpcproto.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// This is a simplified version - the actual implementation would call the RPC
	return "", nil
}

// GetStats gets agent statistics
func (c *DefaultRPCClient) GetStats() (map[string]int, error) {
	return map[string]int{
		"messages": 0,
		"memories": 0,
		"files":    0,
	}, nil
}

// Convenience function to create and configure a Telegram channel
func ConfigureTelegramChannel(agentRPC AgentRPCInterface) (*TelegramBot, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable not set")
	}

	bot := NewTelegramBot(token, agentRPC)
	
	config := map[string]interface{}{
		"token": token,
	}

	if webhookPort := os.Getenv("TELEGRAM_WEBHOOK_PORT"); webhookPort != "" {
		if port, err := strconv.Atoi(webhookPort); err == nil {
			config["webhookPort"] = port
		}
	}

	if webhookHost := os.Getenv("TELEGRAM_WEBHOOK_HOST"); webhookHost != "" {
		config["webhookHost"] = webhookHost
	}

	if err := bot.Initialize(config); err != nil {
		return nil, err
	}

	return bot, nil
}
