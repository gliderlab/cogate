// Agent module - external LLM API, SQLite storage, config persistence, and tool calls

package agent

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gliderlab/cogate/memory"
	"github.com/gliderlab/cogate/rpcproto"
	"github.com/gliderlab/cogate/storage"
	"github.com/gliderlab/cogate/tools"
)

func init() {
	// Register gob types to avoid interface{} serialization issues
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})
}

const (
	CONFIG_SECTION         = "llm"
	DEFAULT_CONTEXT_TOKENS = 8192
	DEFAULT_RESERVE_TOKENS = 1024
	DEFAULT_SOFT_TOKENS    = 800
	DEFAULT_KEEP_MESSAGES  = 30
)

type Agent struct {
	name           string
	model          string
	apiKey         string
	baseURL        string
	client         *http.Client
	store          *storage.Storage
	memoryStore    *memory.VectorMemoryStore
	registry       *tools.Registry
	autoRecall     bool
	recallLimit    int
	recallMinScore float64
	systemTools    []rpcproto.Tool
	// Pulse/Heartbeat system
	pulse *PulseHandler
}

type Message struct {
	Role                 string       `json:"role"`
	Content              string       `json:"content"`
	ToolCalls            []ToolCall   `json:"tool_calls,omitempty"`
	ToolCallID           string       `json:"tool_call_id,omitempty"`
	ToolExecutionResults []ToolResult `json:"tool_results,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ToolResult struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Result any    `json:"result"`
}

// Tool spec for OpenAI-compatible schema
func (a *Agent) ToolSpecs() []map[string]any {
	if a.registry == nil {
		return nil
	}
	return a.registry.GetToolSpecs()
}

// update tool specs cache
func (a *Agent) refreshToolSpecs() {
	if a.registry == nil {
		return
	}
	specs := a.registry.GetToolSpecs()
	a.systemTools = make([]rpcproto.Tool, 0, len(specs))
	for _, s := range specs {
		// Get the function object
		functionObj, ok := s["function"].(map[string]interface{})
		if !ok {
			log.Printf("⚠️ Could not extract function object from spec: %+v", s)
			continue
		}

		name, _ := functionObj["name"].(string)
		desc, _ := functionObj["description"].(string)
		params, _ := functionObj["parameters"].(map[string]interface{})
		log.Printf("🔧 Converting tool: name='%s', desc='%s', params=%+v", name, desc, params)
		a.systemTools = append(a.systemTools, rpcproto.Tool{
			Type: "function",
			Function: rpcproto.ToolFunction{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}
}

type ChatRequest struct {
	Model       string          `json:"model"`
	Messages    []Message       `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Tools       []rpcproto.Tool `json:"tools,omitempty"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Config
type Config struct {
	APIKey         string
	BaseURL        string
	Model          string
	Storage        *storage.Storage
	MemoryStore    *memory.VectorMemoryStore
	Registry       *tools.Registry
	AutoRecall     bool
	RecallLimit    int
	RecallMinScore float64
	// Pulse/Heartbeat system configuration
	PulseEnabled bool
	PulseConfig  *PulseConfig
}

func New(cfg Config) *Agent {
	a := &Agent{
		name:        "OpenClaw-Go",
		model:       cfg.Model,
		apiKey:      cfg.APIKey,
		baseURL:     cfg.BaseURL,
		client:      &http.Client{Timeout: 30 * time.Second},
		store:       cfg.Storage,
		memoryStore: cfg.MemoryStore,
		registry:    cfg.Registry,
	}

	// Use default registry if none is provided
	if a.registry == nil {
		a.registry = tools.NewDefaultRegistry()
	}

	// Load configuration from database
	if cfg.Storage != nil {
		a.loadConfigFromDB()
	}

	a.autoRecall = cfg.AutoRecall
	if cfg.RecallLimit > 0 {
		a.recallLimit = cfg.RecallLimit
	}
	if cfg.RecallMinScore > 0 {
		a.recallMinScore = cfg.RecallMinScore
	}

	// Initialize pulse/heartbeat system
	if cfg.PulseEnabled && cfg.Storage != nil {
		a.pulse = NewPulseHandler(cfg.Storage, cfg.PulseConfig)
		a.pulse.Start()
		log.Printf("[Agent] Pulse/Heartbeat system started")
	}

	return a
}

func (a *Agent) Store() *storage.Storage {
	return a.store
}

func (a *Agent) Registry_() *tools.Registry {
	return a.registry
}

func (a *Agent) MemoryStore() *memory.VectorMemoryStore {
	return a.memoryStore
}

// Pulse returns the pulse handler if available
func (a *Agent) Pulse() *PulseHandler {
	return a.pulse
}

// AddPulseEvent adds a new event to the pulse system
// priority: 0=critical, 1=high, 2=normal, 3=low
func (a *Agent) AddPulseEvent(title, content string, priority int, channel string) (int64, error) {
	if a.pulse == nil {
		return 0, fmt.Errorf("pulse system not enabled")
	}
	return a.pulse.AddEvent(title, content, priority, channel)
}

// GetPulseStatus returns the current status of the pulse system
func (a *Agent) GetPulseStatus() (map[string]interface{}, error) {
	if a.pulse == nil {
		return map[string]interface{}{
			"enabled": false,
		}, nil
	}
	return a.pulse.GetStatus(), nil
}

// Load configuration from database
func (a *Agent) loadConfigFromDB() {
	if a.store == nil {
		return
	}

	exists, err := a.store.ConfigExists(CONFIG_SECTION)
	if err != nil {
		log.Printf("⚠️ failed to check config: %v", err)
		return
	}

	if !exists {
		log.Printf("📝 first start, saving config to database...")
		a.saveConfigToDB()
		return
	}

	log.Printf("📂 loading config from database...")
	config, _ := a.store.GetConfigSection(CONFIG_SECTION)

	if v, ok := config["apiKey"]; ok && v != "" {
		a.apiKey = v
	}
	if v, ok := config["baseUrl"]; ok && v != "" {
		a.baseURL = v
	}
	if v, ok := config["model"]; ok && v != "" {
		a.model = v
	}

	log.Printf("✅ config loaded from database")
}

func (a *Agent) saveConfigToDB() {
	if a.store == nil {
		return
	}

	if a.apiKey != "" {
		a.store.SetConfig(CONFIG_SECTION, "apiKey", a.apiKey)
	}
	if a.baseURL != "" {
		a.store.SetConfig(CONFIG_SECTION, "baseUrl", a.baseURL)
	}
	if a.model != "" {
		a.store.SetConfig(CONFIG_SECTION, "model", a.model)
	}
}

func (a *Agent) UpdateConfig(apiKey, baseURL, model string) {
	a.apiKey = apiKey
	a.baseURL = baseURL
	a.model = model
	if a.store != nil {
		a.saveConfigToDB()
	}
}

func (a *Agent) GetConfig() (apiKey, baseURL, model string) {
	return a.apiKey, a.baseURL, a.model
}

func (a *Agent) Chat(messages []Message) string {
	if a.store != nil {
		lastMsg := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				lastMsg = messages[i].Content
				break
			}
		}
		if lastMsg != "" {
			a.store.AddMessage("default", "user", "[redacted]")
			if a.memoryStore != nil && tools.ShouldCapture(lastMsg) {
				category := tools.DetectCategory(lastMsg)
				results, _ := a.memoryStore.Search(lastMsg, 1, 0.95)
				if len(results) == 0 {
					_, err := a.memoryStore.StoreWithSource(lastMsg, category, 0.6, "auto")
					if err != nil {
						log.Printf("⚠️ auto memory write failed")
					}
				}
			}
			// Soft-trigger memory flush (based on message count + time)
			a.maybeFlushMemory(lastMsg)
			// compaction check
			a.maybeCompact("default", messages)
		}
	}

	// Handle tool calls
	if len(messages) > 0 && len(messages[len(messages)-1].ToolCalls) > 0 {
		return a.handleToolCalls(messages, messages[len(messages)-1].ToolCalls, nil, 0)
	}

	// Detect edit intent
	if len(messages) > 0 {
		lastUserMsg := messages[len(messages)-1].Content
		if editArgs := detectEditIntent(lastUserMsg); editArgs != nil {
			return a.handleEdit(editArgs)
		}
	}

	// Explicit recall trigger: user can request recall via keywords
	if len(messages) > 0 && a.memoryStore != nil {
		lastUserMsg := messages[len(messages)-1].Content
		if isRecallRequest(lastUserMsg) {
			if memories := a.recallRelevantMemories(lastUserMsg); memories != "" {
				log.Printf("recall command injected %d memories", strings.Count(memories, "- ["))
				injected := Message{Role: "system", Content: memories}
				messages = append([]Message{injected}, messages...)
			}
		}
	}

	// Auto recall: inject relevant memories as a system message before sending to model
	if a.autoRecall && a.memoryStore != nil && len(messages) > 0 {
		lastUserMsg := messages[len(messages)-1].Content
		if memories := a.recallRelevantMemories(lastUserMsg); memories != "" {
			log.Printf("auto-recall injected %d memories", strings.Count(memories, "- ["))
			injected := Message{Role: "system", Content: memories}
			messages = append([]Message{injected}, messages...)
		}
	}

	if a.apiKey == "" {
		return a.simpleResponse(messages)
	}

	return a.callAPI(messages)
}

func (a *Agent) executeToolCalls(toolCalls []ToolCall) []ToolResult {
	results := make([]ToolResult, 0, len(toolCalls))

	for _, call := range toolCalls {
		var result interface{}
		var err error

		if a.registry != nil {
			result, err = a.registry.CallTool(call.Function.Name, parseArgs(call.Function.Arguments))
		} else {
			err = fmt.Errorf("tool registry not initialized")
		}

		if err != nil {
			result = map[string]interface{}{
				"error":   err.Error(),
				"tool":    call.Function.Name,
				"success": false,
			}
		} else {
			result = map[string]interface{}{
				"result":  result,
				"tool":    call.Function.Name,
				"success": true,
			}
		}

		results = append(results, ToolResult{
			ID:     call.ID,
			Type:   "function",
			Result: result,
		})
	}

	return results
}

func (a *Agent) handleToolCalls(messages []Message, toolCalls []ToolCall, assistantMsg *Message, depth int) string {
	results := a.executeToolCalls(toolCalls)

	resp := ToolResponse{
		ToolResults: results,
	}
	respBytes, _ := json.Marshal(resp)

	if a.apiKey == "" || depth >= 2 {
		return string(respBytes)
	}

	newMessages := make([]Message, 0, len(messages)+2)
	newMessages = append(newMessages, messages...)

	if assistantMsg != nil {
		newMessages = append(newMessages, *assistantMsg)
	} else if len(messages) == 0 || len(messages[len(messages)-1].ToolCalls) == 0 {
		newMessages = append(newMessages, Message{Role: "assistant", ToolCalls: toolCalls})
	}

	// OpenAI-style tool messages
	for i, tr := range results {
		contentBytes, _ := json.Marshal(tr.Result)
		toolMsg := Message{Role: "tool", Content: string(contentBytes)}
		if i < len(toolCalls) {
			toolMsg.ToolCallID = toolCalls[i].ID
		} else {
			toolMsg.ToolCallID = tr.ID
		}
		newMessages = append(newMessages, toolMsg)
	}

	return a.callAPIWithDepth(newMessages, depth+1)
}

func parseArgs(argsJSON string) map[string]interface{} {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		args = make(map[string]interface{})
	}
	return args
}

// parseCustomToolCalls parses custom tool call format from MiniMax and similar models
// Format: <minimax:tool_call>\n<invoke name="toolname">\n<parameter name="key">value</parameter>\n</invoke>\n</minimax:tool_call> OR
// Format: <minimax:tool_call><invoke name="toolname"><parameter name="key">value</parameter></invoke>\n</minimax:tool_call>
func parseCustomToolCalls(content string) []ToolCall {
	var toolCalls []ToolCall

	// Pattern 1: <minimax:tool_call>...<invoke name="...">...</invoke>...</minimax:tool_call> (with newlines)
	re1 := regexp.MustCompile(`(?i)<minimax:tool_call>\s*<invoke\s+name="([^"]+)"[^>]*>(.*?)</invoke>\s*</minimax:tool_call>`)
	matches1 := re1.FindAllStringSubmatch(content, -1)

	// Pattern 2: <minimax:tool_call><invoke name="..."><parameter>...</invoke>...</invoke> (without newlines)
	re2 := regexp.MustCompile(`(?i)<minimax:tool_call>\s*<invoke\s+name="([^"]+)"[^>]*>(.*?)</invoke>\s*`)
	matches2 := re2.FindAllStringSubmatch(content, -1)

	matches := append(matches1, matches2...)

	log.Printf("🔍 parseCustomToolCalls: content length=%d, matches found=%d", len(content), len(matches))

	for _, m := range matches {
		if len(m) >= 3 {
			toolName := m[1]
			paramsStr := m[2]

			log.Printf("🔍 Found tool: %s, params: %s", toolName, paramsStr[:min(100, len(paramsStr))])

			// Parse parameters
			args := make(map[string]interface{})

			// Match <parameter name="key">value</parameter>
			paramRe := regexp.MustCompile(`<parameter\s+name="([^"]+)">([^<]*)</parameter>`)
			paramMatches := paramRe.FindAllStringSubmatch(paramsStr, -1)

			for _, pm := range paramMatches {
				if len(pm) >= 3 {
					key := pm[1]
					value := strings.TrimSpace(pm[2])
					args[key] = value
					log.Printf("🔍   param: %s = %s", key, value)
				}
			}

			// Map tool names if needed (e.g., "read_file" -> "read")
			actualToolName := mapToolName(toolName)

			// Convert args to JSON string
			argsJSON, _ := json.Marshal(args)

			toolCalls = append(toolCalls, ToolCall{
				ID:   fmt.Sprintf("call_%d", len(toolCalls)),
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      actualToolName,
					Arguments: string(argsJSON),
				},
			})
		}
	}

	return toolCalls
}

// mapToolName maps model-specific tool names to actual tool names
func mapToolName(modelToolName string) string {
	switch modelToolName {
	case "read_file":
		return "read"
	case "write_file":
		return "write"
	case "execute_command", "exec_cmd":
		return "exec"
	case "cat":
		return "read" // cat is similar to read
	default:
		return modelToolName // return as-is if no mapping
	}
}

type ToolResponse struct {
	ToolResults []ToolResult `json:"tool_results"`
}

// Edit intent detection - detect natural language edit requests
func detectEditIntent(msg string) map[string]interface{} {
	// Match patterns:
	// - "Edit <path>: replace <old> with <new>"
	// - "Edit <path>: change <old> to <new>"
	// - "Replace <old> with <new> in <path>"
	// - "replace <old> with <new> in <path>"

	// Pattern 1: Edit <path>: replace <old> with <new>
	re1 := regexp.MustCompile(`(?i)Edit\s+([^:]+):\s*replace\s+(.+)\s+with\s+(.+)`)
	if m := re1.FindStringSubmatch(msg); m != nil {
		return map[string]interface{}{
			"path":    strings.TrimSpace(m[1]),
			"oldText": strings.TrimSpace(m[2]),
			"newText": strings.TrimSpace(m[3]),
		}
	}

	// Pattern 2: Edit <path>: change <old> to <new>
	re2 := regexp.MustCompile(`(?i)Edit\s+([^:]+):\s*change\s+(.+)\s+to\s+(.+)`)
	if m := re2.FindStringSubmatch(msg); m != nil {
		return map[string]interface{}{
			"path":    strings.TrimSpace(m[1]),
			"oldText": strings.TrimSpace(m[2]),
			"newText": strings.TrimSpace(m[3]),
		}
	}

	// Pattern 3: Replace <old> with <new> in <path>
	re3 := regexp.MustCompile(`(?i)Replace\s+(.+)\s+with\s+(.+)\s+in\s+(.+)`)
	if m := re3.FindStringSubmatch(msg); m != nil {
		return map[string]interface{}{
			"path":    strings.TrimSpace(m[3]),
			"oldText": strings.TrimSpace(m[1]),
			"newText": strings.TrimSpace(m[2]),
		}
	}

	// Pattern 4: Chinese pattern: replace <old> with <new> in <path>
	re4 := regexp.MustCompile(`(?i)replace\s+(.+)\s+with\s+(.+)\s+in\s+([^ ]+)`)
	if m := re4.FindStringSubmatch(msg); m != nil {
		return map[string]interface{}{
			"path":    strings.TrimSpace(m[1]),
			"oldText": strings.TrimSpace(m[2]),
			"newText": strings.TrimSpace(m[3]),
		}
	}

	return nil
}

// handleEdit processes edit requests
func (a *Agent) handleEdit(args map[string]interface{}) string {
	if a.registry == nil {
		return "Error: tool registry not initialized"
	}

	result, err := a.registry.CallTool("edit", args)
	if err != nil {
		return fmt.Sprintf("Edit failed: %v", err)
	}

	// Format result
	b, _ := json.Marshal(result)
	return fmt.Sprintf("Edit completed: %s", string(b))
}

// recallRelevantMemories automatically retrieves memories related to the prompt
func (a *Agent) recallRelevantMemories(prompt string) string {
	if a.memoryStore == nil {
		return ""
	}
	limit := a.recallLimit
	if limit <= 0 {
		limit = 3
	}
	minScore := float32(a.recallMinScore)
	if minScore <= 0 {
		minScore = 0.3
	}

	results, err := a.memoryStore.Search(prompt, limit*2, minScore)
	if err != nil || len(results) == 0 {
		return ""
	}

	// re-rank by category/importance weighting
	catBoost := map[string]float32{
		"decision":   0.2,
		"preference": 0.15,
		"fact":       0.1,
		"entity":     0.05,
	}
	sort.Slice(results, func(i, j int) bool {
		ri := results[i]
		rj := results[j]
		wi := ri.Score * (1 + float32(ri.Entry.Importance)) * (1 + catBoost[strings.ToLower(ri.Entry.Category)])
		wj := rj.Score * (1 + float32(rj.Entry.Importance)) * (1 + catBoost[strings.ToLower(rj.Entry.Category)])
		return wi > wj
	})
	if len(results) > limit {
		results = results[:limit]
	}

	return tools.FormatMemoriesForContext(results)
}

func isRecallRequest(msg string) bool {
	low := strings.ToLower(strings.TrimSpace(msg))
	if strings.HasPrefix(low, "/recall") || strings.HasPrefix(low, "recall") {
		return true
	}
	if strings.HasPrefix(low, "recall") || strings.HasPrefix(low, "remember") {
		return true
	}
	return false
}

// maybeFlushMemory soft-triggers long memory flush (SQLite storage)
// Rules: trigger every 50 messages with a minimum interval of 10 minutes
func (a *Agent) maybeFlushMemory(lastMsg string) {
	if a.store == nil || a.memoryStore == nil {
		return
	}

	stats, err := a.store.Stats()
	if err != nil {
		return
	}
	msgCount := stats["messages"]
	if msgCount == 0 || msgCount%50 != 0 {
		return
	}

	lastFlushAtStr, _ := a.store.GetConfig("memory", "lastFlushAt")
	lastFlushCountStr, _ := a.store.GetConfig("memory", "lastFlushCount")
	lastFlushAt, _ := strconv.ParseInt(lastFlushAtStr, 10, 64)
	lastFlushCount, _ := strconv.Atoi(lastFlushCountStr)

	if lastFlushCount == msgCount {
		return
	}
	if time.Now().Unix()-lastFlushAt < 600 {
		return
	}

	if lastMsg != "" && tools.ShouldCapture(lastMsg) {
		category := tools.DetectCategory(lastMsg)
		_, _ = a.memoryStore.StoreWithSource(lastMsg, category, 0.5, "flush")
	}

	_ = a.store.SetConfig("memory", "lastFlushAt", fmt.Sprintf("%d", time.Now().Unix()))
	_ = a.store.SetConfig("memory", "lastFlushCount", fmt.Sprintf("%d", msgCount))
}

func (a *Agent) maybeCompact(sessionKey string, messages []Message) {
	if a.store == nil {
		return
	}
	meta, err := a.store.GetSessionMeta(sessionKey)
	if err != nil {
		return
	}

	stored, err := a.store.GetMessages(sessionKey, 500)
	if err != nil {
		return
	}

	tokens := estimateTokensFromStore(stored)
	meta.TotalTokens = tokens
	_ = a.store.UpsertSessionMeta(meta)

	threshold := DEFAULT_CONTEXT_TOKENS - DEFAULT_RESERVE_TOKENS - DEFAULT_SOFT_TOKENS
	if tokens < threshold || len(stored) <= DEFAULT_KEEP_MESSAGES {
		return
	}

	cut := len(stored) - DEFAULT_KEEP_MESSAGES
	old := stored[:cut]
	keep := stored[cut:]

	summary := buildSummary(old)
	meta.CompactionCount += 1
	meta.LastSummary = summary
	meta.MemoryFlushCompactionCnt = meta.CompactionCount
	meta.MemoryFlushAt = time.Now()
	_ = a.store.UpsertSessionMeta(meta)

	// archive old messages
	if len(old) > 0 {
		_ = a.store.ArchiveMessages(sessionKey, old[len(old)-1].ID)
	}
	_ = a.store.ClearMessages(sessionKey)
	for _, m := range keep {
		_ = a.store.AddMessage(sessionKey, m.Role, m.Content)
	}
	if summary != "" {
		_ = a.store.AddMessage(sessionKey, "system", "[summary]\n"+summary)
	}
	log.Printf("🧹 Compaction done: session=%s, kept=%d, totalTokens=%d", sessionKey, len(keep), tokens)
}

func estimateTokens(messages []Message) int {
	// Simple estimate: character count / 4 + 4 tokens per message
	total := 0
	for _, m := range messages {
		total += len(m.Content)/4 + 4
	}
	return total
}

func estimateTokensFromStore(messages []storage.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)/4 + 4
	}
	return total
}

func buildSummary(msgs []storage.Message) string {
	if len(msgs) == 0 {
		return ""
	}
	lines := make([]string, 0, len(msgs))
	for _, m := range msgs {
		content := m.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		lines = append(lines, fmt.Sprintf("%s: %s", m.Role, content))
	}
	summary := strings.Join(lines, "\n")
	if len(summary) > 2000 {
		summary = summary[:2000] + "..."
	}
	return summary
}

func (a *Agent) callAPI(messages []Message) string {
	return a.callAPIWithDepth(messages, 0)
}

func (a *Agent) callAPIWithDepth(messages []Message, depth int) string {
	reqBody := ChatRequest{
		Model:       a.model,
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   1000,
	}
	if len(a.systemTools) == 0 {
		a.refreshToolSpecs()
	}

	// Debug: log tools count
	log.Printf("🔧 Tools count: %d", len(a.systemTools))
	if len(a.systemTools) > 0 {
		for i, t := range a.systemTools {
			log.Printf("🔧 Tool[%d]: Type=%s, Func=%+v", i, t.Type, t.Function)
		}
	}

	reqBody.Tools = a.systemTools

	body, _ := json.Marshal(reqBody)
	url := a.baseURL + "/chat/completions"

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Sprintf("API error: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Sprintf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return fmt.Sprintf("parse error: %v", err)
	}

	// handle tool call chain if returned (standard format)
	if len(chatResp.Choices) > 0 && len(chatResp.Choices[0].Message.ToolCalls) > 0 {
		// Filter out invalid tool calls (empty name)
		validCalls := make([]ToolCall, 0)
		for _, tc := range chatResp.Choices[0].Message.ToolCalls {
			if tc.Function.Name != "" && tc.Function.Arguments != "" {
				validCalls = append(validCalls, tc)
			}
		}
		if len(validCalls) > 0 {
			assistantMsg := chatResp.Choices[0].Message
			return a.handleToolCalls(messages, validCalls, &assistantMsg, depth)
		}
		// If all invalid, try custom format
	}

	// handle custom tool call format (MiniMax, etc.)
	if len(chatResp.Choices) > 0 {
		content := chatResp.Choices[0].Message.Content

		// Try to parse custom tool call format: minimax:tool_call
		toolCalls := parseCustomToolCalls(content)
		if len(toolCalls) > 0 {
			assistantMsg := Message{Role: "assistant", Content: content, ToolCalls: toolCalls}
			return a.handleToolCalls(messages, toolCalls, &assistantMsg, depth)
		}

		if a.store != nil {
			a.store.AddMessage("default", "assistant", "[redacted]")
		}
		return content
	}

	return "no response"
}

func (a *Agent) simpleResponse(messages []Message) string {
	var userMsg string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			userMsg = messages[i].Content
			break
		}
	}

	input := strings.TrimSpace(strings.ToLower(userMsg))
	response := ""

	switch {
	case strings.Contains(input, "hello") || strings.Contains(input, "hi"):
		response = "Hello! I am OpenClaw-Go.\n\nAvailable tools:\n- exec: run commands\n- read: read files\n- write: write files"
	case strings.Contains(input, "time"):
		response = time.Now().Format("2006-01-02 15:04:05")
	case strings.Contains(input, "stat"):
		stats, _ := a.store.Stats()
		response = fmt.Sprintf("Storage stats:\n- messages: %d\n- memories: %d\n- files: %d", stats["messages"], stats["memories"], stats["files"])
	case strings.Contains(input, "tools"):
		if a.registry != nil {
			toolList := a.registry.List()
			response = "Available tools:\n- " + strings.Join(toolList, "\n- ")
		} else {
			response = "tools not initialized"
		}
	case strings.Contains(input, "help") || strings.Contains(input, "aid"):
		response = "OpenClaw-Go\n\nCommands:\n- hello - greeting\n- time - time\n- stat - stats\n- tools - list tools\n- help - help"
	default:
		response = "I received:: " + userMsg
	}

	if a.store != nil {
		a.store.AddMessage("default", "assistant", response)
	}

	return response
}

func maskKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}
