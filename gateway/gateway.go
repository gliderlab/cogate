// Gateway module - HTTP server

package gateway

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/rpc"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gliderlab/cogate/cron"
	"github.com/gliderlab/cogate/gateway/channels"
	"github.com/gliderlab/cogate/processtool"
	"github.com/gliderlab/cogate/rpcproto"
)

func init() {
	// Register gob types for interface{} serialization
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})
}

// Config for Gateway
type Config struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	AgentAddr   string `json:"agentAddr"`
	UIAuthToken string `json:"uiAuthToken"`
}

type Gateway struct {
	cfg            Config
	client         *rpc.Client
	server         *http.Server
	channelAdapter *channels.ChannelAdapter
	cronHandler    *cron.CronHandler
	mu             sync.RWMutex
}

type ChatRequest struct {
	Model    string             `json:"model"`
	Messages []rpcproto.Message `json:"messages"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int              `json:"index"`
	Message      rpcproto.Message `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func New(cfg Config) *Gateway {
	if cfg.Port == 0 {
		cfg.Port = 18789
	}
	if cfg.Host == "" {
		cfg.Host = "0.0.0.0"
	}
	if cfg.UIAuthToken == "" {
		log.Printf("[WARN] UIAuthToken is empty; API will reject all requests")
	}
	return &Gateway{cfg: cfg}
}

func (g *Gateway) Config() Config {
	return g.cfg
}

func (g *Gateway) SetClient(c *rpc.Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.client = c
}

func (g *Gateway) Start() error {
	mux := http.NewServeMux()

	// Static files (web chat UI) embedded in binary
	staticHandler := embeddedFileServer()
	log.Printf("Static assets: embedded")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			r.URL.Path = "/index.html"
		}
		staticHandler.ServeHTTP(w, r)
	})

	// WebSocket endpoint for real-time chat
	mux.HandleFunc("/ws/chat", g.HandleWebSocket)

	// Auth middleware for API routes (header Authorization: Bearer <token> or X-OCG-UI-Token)
	requireAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			token := strings.TrimSpace(g.cfg.UIAuthToken)
			if token == "" {
				http.Error(w, "unauthorized (ui token not set)", http.StatusUnauthorized)
				return
			}
			header := r.Header.Get("Authorization")
			if strings.HasPrefix(strings.ToLower(header), "bearer ") {
				header = strings.TrimSpace(header[len("Bearer "):])
			}
			alt := r.Header.Get("X-OCG-UI-Token")
			if header == token || alt == token {
				next(w, r)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}
	}

	// API routes (protected)
	mux.HandleFunc("/v1/chat/completions", requireAuth(g.handleChat))
	mux.HandleFunc("/health", requireAuth(g.handleHealth))
	mux.HandleFunc("/storage/stats", requireAuth(g.handleStorageStats))
	// Process tool endpoints
	mux.HandleFunc("/process/start", requireAuth(g.handleProcessStart))
	mux.HandleFunc("/process/list", requireAuth(g.handleProcessList))
	mux.HandleFunc("/process/log", requireAuth(g.handleProcessLog))
	mux.HandleFunc("/process/write", requireAuth(g.handleProcessWrite))
	mux.HandleFunc("/process/kill", requireAuth(g.handleProcessKill))
	// Memory tool endpoints
	mux.HandleFunc("/memory/search", requireAuth(g.handleMemorySearch))
	mux.HandleFunc("/memory/get", requireAuth(g.handleMemoryGet))
	mux.HandleFunc("/memory/store", requireAuth(g.handleMemoryStore))

	// Cron endpoints
	mux.HandleFunc("/cron/status", requireAuth(g.handleCronStatus))
	mux.HandleFunc("/cron/list", requireAuth(g.handleCronList))
	mux.HandleFunc("/cron/add", requireAuth(g.handleCronAdd))
	mux.HandleFunc("/cron/update", requireAuth(g.handleCronUpdate))
	mux.HandleFunc("/cron/remove", requireAuth(g.handleCronRemove))
	mux.HandleFunc("/cron/run", requireAuth(g.handleCronRun))

	// Telegram Bot webhook endpoint (public, no auth)
	mux.HandleFunc("/telegram/webhook", g.handleTelegramWebhook)

	// Telegram Bot configuration endpoints (protected)
	mux.HandleFunc("/telegram/setWebhook", requireAuth(g.handleTelegramSetWebhook))
	mux.HandleFunc("/telegram/status", requireAuth(g.handleTelegramStatus))

	addr := fmt.Sprintf("%s:%d", g.cfg.Host, g.cfg.Port)
	g.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  90 * time.Second,
	}
	log.Printf("Gateway listening on %s", addr)

	// Initialize Channel Adapter with Telegram support
	g.channelAdapter = channels.NewChannelAdapter(
		channels.DefaultChannelAdapterConfig(),
		&GatewayAgentRPC{client: g.client},
	)

	// Initialize Cron handler
	cronStore := filepath.Join(getGatewayDir(), "data", "cron", "jobs.json")
	g.cronHandler = cron.NewCronHandler(cronStore)
	g.cronHandler.SetSystemEventCallback(func(text string) {
		if g.client == nil {
			log.Printf("[Cron] agent not connected")
			return
		}
		_, err := (&GatewayAgentRPC{client: g.client}).Chat([]channels.Message{{Role: "system", Content: text}})
		if err != nil {
			log.Printf("[Cron] system event error: %v", err)
		}
	})
	g.cronHandler.SetAgentTurnCallback(func(message, model, thinking string) (string, error) {
		if g.client == nil {
			return "", fmt.Errorf("agent not connected")
		}
		return (&GatewayAgentRPC{client: g.client}).Chat([]channels.Message{{Role: "user", Content: message}})
	})
	g.cronHandler.SetBroadcastCallback(func(message, channel, target string) error {
		if g.channelAdapter == nil {
			return fmt.Errorf("channel adapter not initialized")
		}
		chType := channelTypeFromString(channel)
		if chType == "" {
			return fmt.Errorf("unknown channel: %s", channel)
		}
		chatID, _ := strconv.ParseInt(target, 10, 64)
		_, err := g.channelAdapter.SendMessage(chType, &channels.SendMessageRequest{
			ChatID: chatID,
			Text:   message,
		})
		return err
	})
	g.cronHandler.Start()

	// Register Telegram channel if token is provided
	if telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN"); telegramToken != "" {
		if g.client != nil {
			// Create Telegram bot as a channel plugin
			bot := channels.NewTelegramBot(telegramToken, &GatewayAgentRPC{client: g.client})
			if err := g.channelAdapter.RegisterChannel(bot); err != nil {
				log.Printf("⚠️ Failed to register Telegram channel: %v", err)
			} else {
				log.Printf("🤖 Telegram channel registered")
				// Start the Telegram bot
				if err := g.channelAdapter.StartChannel(channels.ChannelTelegram); err != nil {
					log.Printf("⚠️ Failed to start Telegram channel: %v", err)
				}
			}
		} else {
			log.Printf("⚠️ Telegram Bot token found but agent not connected yet")
		}
	} else {
		log.Printf("ℹ️ No TELEGRAM_BOT_TOKEN environment variable found")
	}

	return g.server.ListenAndServe()
}

func (g *Gateway) Stop() {
	if g.cronHandler != nil {
		g.cronHandler.Stop()
	}
	if g.server != nil {
		g.server.Close()
	}
}

func (g *Gateway) clientOrError() (*rpc.Client, error) {
	g.mu.RLock()
	client := g.client
	g.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("agent not connected")
	}
	return client, nil
}

func (g *Gateway) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	client, err := g.clientOrError()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Read error", http.StatusBadRequest)
		return
	}

	var req ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Parse error", http.StatusBadRequest)
		return
	}

	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		log.Printf("Received message: role=%s len=%d", last.Role, len(last.Content))
	}

	var reply rpcproto.ChatReply
	args := rpcproto.ChatArgs{Messages: req.Messages}
	if err := client.Call("Agent.Chat", args, &reply); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return OpenAI-compatible response
	resp := ChatResponse{
		ID:      "chatcmpl-" + randomID(),
		Object:  "chat.completion",
		Created: nowUnix(),
		Model:   req.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: rpcproto.Message{
					Role:    "assistant",
					Content: reply.Content,
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     countTokens(body),
			CompletionTokens: countTokens([]byte(reply.Content)),
			TotalTokens:      countTokens(body) + countTokens([]byte(reply.Content)),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"status":"ok"}`))
}

func (g *Gateway) handleStorageStats(w http.ResponseWriter, r *http.Request) {
	client, err := g.clientOrError()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	var reply rpcproto.StatsReply
	if err := client.Call("Agent.Stats", struct{}{}, &reply); err != nil {
		http.Error(w, "error getting stats", http.StatusInternalServerError)
		return
	}

	type StatsResponse struct {
		Status string         `json:"status"`
		Stats  map[string]int `json:"stats"`
	}

	json.NewEncoder(w).Encode(StatsResponse{Status: "ok", Stats: reply.Stats})
}

// Utility functions
func randomID() string {
	// Simple ID for demo purposes
	return fmt.Sprintf("%d", nowUnix())
}

func nowUnix() int64 {
	return time.Now().Unix()
}

func countTokens(data []byte) int {
	return len(data) / 4 // Simple estimate
}

func channelTypeFromString(s string) channels.ChannelType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(channels.ChannelTelegram):
		return channels.ChannelTelegram
	case string(channels.ChannelWhatsApp):
		return channels.ChannelWhatsApp
	case string(channels.ChannelSlack):
		return channels.ChannelSlack
	case string(channels.ChannelDiscord):
		return channels.ChannelDiscord
	case string(channels.ChannelWebChat):
		return channels.ChannelWebChat
	default:
		return ""
	}
}

// Locate gateway directory
func getGatewayDir() string {
	if env := os.Getenv("OPENCLAW_GATEWAY_DIR"); env != "" {
		return env
	}

	isValid := func(dir string) bool {
		if _, err := os.Stat(filepath.Join(dir, "static", "index.html")); err == nil {
			return true
		}
		return false
	}

	execPath, _ := os.Executable()
	if execPath != "" {
		execDir := filepath.Dir(execPath)
		candidates := []string{
			filepath.Join(execDir, "gateway"),
			filepath.Join(filepath.Dir(execDir), "gateway"),
		}
		for _, c := range candidates {
			if isValid(c) {
				return c
			}
		}
	}
	if isValid("/opt/openclaw-go/gateway") {
		return "/opt/openclaw-go/gateway"
	}
	// fallback: keep previous behavior if nothing else matches
	if execPath != "" {
		return filepath.Join(filepath.Dir(execPath), "gateway")
	}
	return "gateway"
}

// Process handlers
func (g *Gateway) handleProcessStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var req struct {
		Command string `json:"command"`
		Workdir string `json:"workdir,omitempty"`
		Env     string `json:"env,omitempty"`
		Pty     bool   `json:"pty,omitempty"`
	}
	json.Unmarshal(body, &req)

	// directly call process tool
	procTool := processtool.ProcessTool{}
	result, err := procTool.Execute(map[string]interface{}{
		"action":  "start",
		"command": req.Command,
		"workdir": req.Workdir,
		"env":     req.Env,
		"pty":     req.Pty,
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(result)
}

func (g *Gateway) handleProcessList(w http.ResponseWriter, r *http.Request) {
	procTool := processtool.ProcessTool{}
	result, _ := procTool.Execute(map[string]interface{}{"action": "list"})
	json.NewEncoder(w).Encode(result)
}

func (g *Gateway) handleProcessLog(w http.ResponseWriter, r *http.Request) {
	sessionId := r.URL.Query().Get("sessionId")
	offset := 0
	limit := 0
	fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)

	procTool := processtool.ProcessTool{}
	result, err := procTool.Execute(map[string]interface{}{
		"action":    "log",
		"sessionId": sessionId,
		"offset":    offset,
		"limit":     limit,
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(result)
}

func (g *Gateway) handleProcessKill(w http.ResponseWriter, r *http.Request) {
	sessionId := r.URL.Query().Get("sessionId")

	procTool := processtool.ProcessTool{}
	result, err := procTool.Execute(map[string]interface{}{
		"action":    "kill",
		"sessionId": sessionId,
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(result)
}

func (g *Gateway) handleProcessWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var req struct {
		SessionID string `json:"sessionId"`
		Data      string `json:"data"`
		EOF       bool   `json:"eof,omitempty"`
	}
	json.Unmarshal(body, &req)

	procTool := processtool.ProcessTool{}
	result, err := procTool.Execute(map[string]interface{}{
		"action":    "write",
		"sessionId": req.SessionID,
		"data":      req.Data,
		"eof":       req.EOF,
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(result)
}

// Memory handlers
func (g *Gateway) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	client, err := g.clientOrError()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	query := r.URL.Query().Get("query")
	category := r.URL.Query().Get("category")
	limit := 5
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
	minScore := 0.7
	fmt.Sscanf(r.URL.Query().Get("minScore"), "%f", &minScore)

	var reply rpcproto.ToolResultReply
	if err := client.Call("Agent.MemorySearch", rpcproto.MemorySearchArgs{
		Query:    query,
		Category: category,
		Limit:    limit,
		MinScore: minScore,
	}, &reply); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse JSON string back into interface{}
	var result interface{}
	json.Unmarshal([]byte(reply.Result), &result)
	json.NewEncoder(w).Encode(result)
}

func (g *Gateway) handleMemoryGet(w http.ResponseWriter, r *http.Request) {
	client, err := g.clientOrError()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Query().Get("path")

	var reply rpcproto.ToolResultReply
	if err := client.Call("Agent.MemoryGet", rpcproto.MemoryGetArgs{Path: path}, &reply); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse JSON string back into interface{}
	var result interface{}
	json.Unmarshal([]byte(reply.Result), &result)
	json.NewEncoder(w).Encode(result)
}

func (g *Gateway) handleMemoryStore(w http.ResponseWriter, r *http.Request) {
	client, err := g.clientOrError()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var req struct {
		Text       string  `json:"text"`
		Category   string  `json:"category,omitempty"`
		Importance float64 `json:"importance,omitempty"`
	}
	json.Unmarshal(body, &req)

	var reply rpcproto.ToolResultReply
	if err := client.Call("Agent.MemoryStore", rpcproto.MemoryStoreArgs{
		Text:       req.Text,
		Category:   req.Category,
		Importance: req.Importance,
	}, &reply); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse JSON string back into interface{}
	var result interface{}
	json.Unmarshal([]byte(reply.Result), &result)
	json.NewEncoder(w).Encode(result)
}

// Cron handlers
func (g *Gateway) handleCronStatus(w http.ResponseWriter, r *http.Request) {
	if g.cronHandler == nil {
		http.Error(w, "cron not initialized", http.StatusServiceUnavailable)
		return
	}
	json.NewEncoder(w).Encode(g.cronHandler.GetStatus())
}

func (g *Gateway) handleCronList(w http.ResponseWriter, r *http.Request) {
	if g.cronHandler == nil {
		http.Error(w, "cron not initialized", http.StatusServiceUnavailable)
		return
	}
	json.NewEncoder(w).Encode(g.cronHandler.ListJobs())
}

func (g *Gateway) handleCronAdd(w http.ResponseWriter, r *http.Request) {
	if g.cronHandler == nil {
		http.Error(w, "cron not initialized", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req map[string]interface{}
	json.Unmarshal(body, &req)

	var jobData map[string]interface{}
	if v, ok := req["job"].(map[string]interface{}); ok {
		jobData = v
	} else {
		jobData = req
	}

	job, err := cron.CreateJobFromMap(jobData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := g.cronHandler.AddJob(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(job)
}

func (g *Gateway) handleCronUpdate(w http.ResponseWriter, r *http.Request) {
	if g.cronHandler == nil {
		http.Error(w, "cron not initialized", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req map[string]interface{}
	json.Unmarshal(body, &req)

	jobID, _ := req["jobId"].(string)
	if jobID == "" {
		jobID, _ = req["id"].(string)
	}
	patch, _ := req["patch"].(map[string]interface{})
	if jobID == "" || patch == nil {
		http.Error(w, "jobId and patch are required", http.StatusBadRequest)
		return
	}

	job, err := g.cronHandler.UpdateJob(jobID, patch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(job)
}

func (g *Gateway) handleCronRemove(w http.ResponseWriter, r *http.Request) {
	if g.cronHandler == nil {
		http.Error(w, "cron not initialized", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req map[string]interface{}
	json.Unmarshal(body, &req)
	jobID, _ := req["jobId"].(string)
	if jobID == "" {
		jobID, _ = req["id"].(string)
	}
	if jobID == "" {
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}
	if err := g.cronHandler.RemoveJob(jobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func (g *Gateway) handleCronRun(w http.ResponseWriter, r *http.Request) {
	if g.cronHandler == nil {
		http.Error(w, "cron not initialized", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req map[string]interface{}
	json.Unmarshal(body, &req)
	jobID, _ := req["jobId"].(string)
	if jobID == "" {
		jobID, _ = req["id"].(string)
	}
	if jobID == "" {
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}
	if err := g.cronHandler.RunJob(jobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// GatewayAgentRPC implements channels.AgentRPCInterface for gateway-agent communication
type GatewayAgentRPC struct {
	client *rpc.Client
}

// Chat sends a chat request to the agent via RPC
func (r *GatewayAgentRPC) Chat(messages []channels.Message) (string, error) {
	if r.client == nil {
		return "", fmt.Errorf("agent RPC client not connected")
	}

	// Convert to rpcproto format
	rpcMessages := make([]rpcproto.Message, 0, len(messages))
	for _, m := range messages {
		rpcMessages = append(rpcMessages, rpcproto.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	var reply rpcproto.ChatReply
	args := rpcproto.ChatArgs{Messages: rpcMessages}

	err := r.client.Call("Agent.Chat", args, &reply)
	if err != nil {
		return "", err
	}

	return reply.Content, nil
}

// GetStats gets statistics from the agent via RPC
func (r *GatewayAgentRPC) GetStats() (map[string]int, error) {
	if r.client == nil {
		return nil, fmt.Errorf("agent RPC client not connected")
	}

	var reply rpcproto.StatsReply
	if err := r.client.Call("Agent.Stats", struct{}{}, &reply); err != nil {
		return nil, err
	}

	return reply.Stats, nil
}
