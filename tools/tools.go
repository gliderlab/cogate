// Tools module - tool invocation framework
package tools

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// Tool defines the tool interface
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(args map[string]interface{}) (interface{}, error)
}

// Registry holds registered tools
type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register a tool
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
	log.Printf("✅ tool registered: %s", t.Name())
}

// Get returns a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List all tools
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// CallTool and return its result
func (r *Registry) CallTool(name string, args map[string]interface{}) (interface{}, error) {
	t, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	log.Printf("🔧 calling tool: %s, args: %v", name, args)
	result, err := t.Execute(args)
	if err != nil {
		log.Printf("❌ tool failed: %s - %v", name, err)
		return nil, err
	}

	log.Printf("✅ tool succeeded: %s", name)
	return result, nil
}

// GetToolSpecs returns OpenAI-format specs with function wrapper
func (r *Registry) GetToolSpecs() []map[string]interface{} {
	specs := make([]map[string]interface{}, 0)
	for _, t := range r.tools {
		specs = append(specs, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  t.Parameters(),
			},
		})
	}
	return specs
}

// ParseToolCalls parse OpenAI tool_calls response
func ParseToolCalls(response map[string]interface{}) []map[string]interface{} {
	toolCalls, ok := response["choices"].([]map[string]interface{})
	if !ok || len(toolCalls) == 0 {
		return nil
	}

	message, ok := toolCalls[0]["message"].(map[string]interface{})
	if !ok {
		return nil
	}

	calls, ok := message["tool_calls"].([]map[string]interface{})
	if !ok {
		return nil
	}

	return calls
}

// FormatToolResult formats tool result as a message
func FormatToolResult(toolName string, result interface{}) map[string]interface{} {
	var content string
	switch v := result.(type) {
	case string:
		content = v
	case []byte:
		content = string(v)
	default:
		b, _ := json.Marshal(v)
		content = string(b)
	}

	return map[string]interface{}{
		"role":         "tool",
		"tool_call_id": fmt.Sprintf("call_%s", toolName),
		"content":      content,
	}
}

// ErrorResult returns an error payload
func ErrorResult(toolName string, err error) map[string]interface{} {
	return map[string]interface{}{
		"role":         "tool",
		"tool_call_id": fmt.Sprintf("call_%s", toolName),
		"content":      fmt.Sprintf("error: %v", err),
	}
}

// ParseArgs parses JSON args
func ParseArgs(argsJSON string) (map[string]interface{}, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		// Try as array
		var arr []interface{}
		if jerr := json.Unmarshal([]byte(argsJSON), &arr); jerr == nil {
			return map[string]interface{}{"args": arr}, nil
		}
		return nil, fmt.Errorf("failed to parse args: %v", err)
	}
	return args, nil
}

// GetString gets a string arg
func GetString(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetInt gets an int arg
func GetInt(args map[string]interface{}, key string) int {
	if v, ok := args[key]; ok {
		switch f := v.(type) {
		case float64:
			return int(f)
		case int:
			return f
		case string:
			var i int
			fmt.Sscanf(f, "%d", &i)
			return i
		}
	}
	return 0
}

// GetFloat64 gets a float arg
func GetFloat64(args map[string]interface{}, key string) float64 {
	if v, ok := args[key]; ok {
		switch f := v.(type) {
		case float64:
			return f
		case int:
			return float64(f)
		case string:
			var x float64
			fmt.Sscanf(f, "%f", &x)
			return x
		}
	}
	return 0
}

// GetBool gets a bool arg
func GetBool(args map[string]interface{}, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// Truncate long text
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...\n(content truncated)"
}

// Summarize text (for AI responses)
func Summarize(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	lines := strings.Split(s, "\n")
	if len(lines) > 10 {
		// keep first 5 and last 5 lines
		return strings.Join(append(lines[:5], lines[len(lines)-5:]...), "\n") + "\n...(middle omitted)"
	}

	return s
}
