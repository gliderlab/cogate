// Package adapter - Tool plugin adapter system
// Provides plugin-based tool loading with hot-reload support
package adapter

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"sync"
)

// PluginInfo contains metadata about a plugin
type PluginInfo struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Description string                 `json:"description"`
	Author      string                 `json:"author"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
}

// PluginLoader defines the interface for tool plugins
type PluginLoader interface {
	// PluginInfo returns metadata about the plugin
	PluginInfo() PluginInfo

	// Initialize is called when the plugin is loaded
	Initialize(config map[string]interface{}) error

	// Execute runs the tool with given arguments
	Execute(args map[string]interface{}) (interface{}, error)

	// Shutdown is called when the plugin is unloaded
	Shutdown() error

	// HealthCheck verifies the plugin is working
	HealthCheck() error
}

// PluginBase provides common functionality for plugins
type PluginBase struct {
	mu     sync.RWMutex
	config map[string]interface{}
	name   string
}

func (p *PluginBase) GetConfig(key string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.config[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (p *PluginBase) SetConfig(key string, value interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.config == nil {
		p.config = make(map[string]interface{})
	}
	p.config[key] = value
}

// ToolSpec defines the OpenAI-compatible tool specification
type ToolSpec struct {
	Type     string `json:"type"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	} `json:"function"`
}

// Result represents a tool execution result
type Result struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Duration  int64       `json:"duration_ms"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NewResult creates a successful result
func NewResult(data interface{}) *Result {
	return &Result{
		Success: true,
		Data:    data,
	}
}

// NewErrorResult creates an error result
func NewErrorResult(err error) *Result {
	return &Result{
		Success: false,
		Error:   err.Error(),
	}
}

// Context holds the execution context for tools
type Context struct {
	AgentName   string
	SessionID   string
	Workspace   string
	GatewayURL  string
	UserID      string
	Channel     string
	Timestamp   int64
	Extra       map[string]interface{}
}

// GetContextString retrieves a string from context
func (c *Context) GetContextString(key string) string {
	if c.Extra == nil {
		return ""
	}
	if v, ok := c.Extra[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ToolAdapter is the main adapter that manages plugins
type ToolAdapter struct {
	mu       sync.RWMutex
	plugins  map[string]PluginLoader
	registry *PluginRegistry
	config   AdapterConfig
}

// AdapterConfig holds adapter configuration
type AdapterConfig struct {
	PluginDir      string `json:"pluginDir"`
	AutoReload     bool   `json:"autoReload"`
	ReloadInterval int    `json:"reloadIntervalSeconds"`
	MaxRetries     int    `json:"maxRetries"`
	Timeout        int    `json:"defaultTimeoutSeconds"`
}

// DefaultAdapterConfig returns default configuration
func DefaultAdapterConfig() AdapterConfig {
	return AdapterConfig{
		PluginDir:      "./plugins",
		AutoReload:     false,
		ReloadInterval: 60,
		MaxRetries:     3,
		Timeout:        30,
	}
}

// NewToolAdapter creates a new tool adapter
func NewToolAdapter(cfg AdapterConfig) *ToolAdapter {
	return &ToolAdapter{
		plugins:  make(map[string]PluginLoader),
		registry: NewPluginRegistry(),
		config:   cfg,
	}
}

// RegisterPlugin registers a plugin with the adapter
func (a *ToolAdapter) RegisterPlugin(name string, plugin PluginLoader) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if plugin already registered
	if _, exists := a.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	info := plugin.PluginInfo()
	log.Printf("✅ registered plugin: %s v%s", info.Name, info.Version)

	a.plugins[name] = plugin
	a.registry.Add(info)

	return nil
}

// UnregisterPlugin removes a plugin from the adapter
func (a *ToolAdapter) UnregisterPlugin(name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	plugin, exists := a.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// Call shutdown
	if err := plugin.Shutdown(); err != nil {
		log.Printf("⚠️ plugin %s shutdown error: %v", name, err)
	}

	delete(a.plugins, name)
	a.registry.Remove(name)

	return nil
}

// ExecuteTool runs a tool by name
func (a *ToolAdapter) ExecuteTool(name string, args map[string]interface{}, ctx *Context) *Result {
	a.mu.RLock()
	plugin, exists := a.plugins[name]
	a.mu.RUnlock()

	if !exists {
		return NewErrorResult(fmt.Errorf("tool not found: %s", name))
	}

	// Add context to args
	if ctx != nil {
		args["_context"] = map[string]interface{}{
			"agent":    ctx.AgentName,
			"session":  ctx.SessionID,
			"workspace": ctx.Workspace,
			"user":     ctx.UserID,
			"channel":  ctx.Channel,
		}
	}

	result, err := plugin.Execute(args)
	if err != nil {
		return NewErrorResult(err)
	}
	return NewResult(result)
}

// GetToolSpec returns the OpenAI-compatible tool specification
func (a *ToolAdapter) GetToolSpec(name string) (*ToolSpec, error) {
	a.mu.RLock()
	plugin, exists := a.plugins[name]
	a.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	info := plugin.PluginInfo()
	spec := &ToolSpec{
		Type: "function",
	}

	spec.Function.Name = info.Name
	spec.Function.Description = info.Description
	spec.Function.Parameters = info.Schema

	return spec, nil
}

// GetAllToolSpecs returns all tool specifications
func (a *ToolAdapter) GetAllToolSpecs() []ToolSpec {
	a.mu.RLock()
	defer a.mu.RUnlock()

	specs := make([]ToolSpec, 0, len(a.plugins))
	for _, plugin := range a.plugins {
		info := plugin.PluginInfo()
		specs = append(specs, ToolSpec{
			Type: "function",
			Function: struct {
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				Parameters  map[string]interface{} `json:"parameters"`
			}{
				Name:        info.Name,
				Description: info.Description,
				Parameters:  info.Schema,
			},
		})
	}

	return specs
}

// ListTools returns the list of registered tool names
func (a *ToolAdapter) ListTools() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	names := make([]string, 0, len(a.plugins))
	for name := range a.plugins {
		names = append(names, name)
	}
	return names
}

// GetPluginInfo returns info about a specific plugin
func (a *ToolAdapter) GetPluginInfo(name string) (*PluginInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	plugin, exists := a.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", name)
	}

	info := plugin.PluginInfo()
	return &info, nil
}

// GetRegistry returns the plugin registry
func (a *ToolAdapter) GetRegistry() *PluginRegistry {
	return a.registry
}

// Shutdown unloads all plugins
func (a *ToolAdapter) Shutdown() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for name, plugin := range a.plugins {
		if err := plugin.Shutdown(); err != nil {
			log.Printf("⚠️ plugin %s shutdown error: %v", name, err)
		}
	}

	a.plugins = make(map[string]PluginLoader)
	return nil
}

// PluginRegistry maintains a registry of all loaded plugins
type PluginRegistry struct {
	mu     sync.RWMutex
	plugins map[string]PluginInfo
}

func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins: make(map[string]PluginInfo),
	}
}

func (r *PluginRegistry) Add(info PluginInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[info.Name] = info
}

func (r *PluginRegistry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.plugins, name)
}

func (r *PluginRegistry) Get(name string) (PluginInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.plugins[name]
	return info, ok
}

func (r *PluginRegistry) List() []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(r.plugins))
	for _, info := range r.plugins {
		infos = append(infos, info)
	}
	return infos
}

// CreateBuiltinAdapter creates an adapter with all built-in tools
func CreateBuiltinAdapter(workspace string) *ToolAdapter {
	adapter := NewToolAdapter(DefaultAdapterConfig())

	// Note: Built-in tools will be registered here
	// For now, this is a placeholder for the migration path

	log.Printf("📦 Built-in adapter created (tools will be migrated to plugins)")
	return adapter
}

// ConvertToolToPlugin converts a legacy tool to a plugin
func ConvertToolToPlugin(name, description string, executeFunc func(args map[string]interface{}) (interface{}, error)) PluginLoader {
	return &convertedTool{
		name:        name,
		description: description,
		executeFunc: executeFunc,
	}
}

type convertedTool struct {
	name        string
	description string
	executeFunc func(args map[string]interface{}) (interface{}, error)
}

func (t *convertedTool) PluginInfo() PluginInfo {
	return PluginInfo{
		Name:        t.name,
		Version:     "1.0.0",
		Description: t.description,
		Author:      "OpenClaw-Go",
		Schema:      map[string]interface{}{},
	}
}

func (t *convertedTool) Initialize(config map[string]interface{}) error {
	return nil
}

func (t *convertedTool) Execute(args map[string]interface{}) (interface{}, error) {
	return t.executeFunc(args)
}

func (t *convertedTool) Shutdown() error {
	return nil
}

func (t *convertedTool) HealthCheck() error {
	return nil
}

// JSON Marshal helpers for tool results
func (r *Result) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

func (r *Result) ToString() string {
	if r.Success {
		if data, ok := r.Data.(string); ok {
			return data
		}
		b, _ := json.Marshal(r.Data)
		return string(b)
	}
	return r.Error
}

// HasTool checks if a tool is registered
func (a *ToolAdapter) HasTool(name string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, exists := a.plugins[name]
	return exists
}

// GetToolCount returns the number of registered tools
func (a *ToolAdapter) GetToolCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.plugins)
}

// ReflectArgs converts args to the expected type using reflection
func ReflectArgs(args map[string]interface{}, expected interface{}) error {
	// Simple reflection-based conversion
	if data, err := json.Marshal(args); err == nil {
		return json.Unmarshal(data, expected)
	}
	return nil
}

// ValidateArgs validates that required args are present
func ValidateArgs(args map[string]interface{}, required []string) error {
	for _, key := range required {
		if _, ok := args[key]; !ok {
			return fmt.Errorf("missing required argument: %s", key)
		}
	}
	return nil
}

// GetToolDocumentation returns markdown documentation for all tools
func (a *ToolAdapter) GetToolDocumentation() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	docs := "# Tool Documentation\n\n"
	for _, plugin := range a.plugins {
		info := plugin.PluginInfo()
		docs += fmt.Sprintf("## %s\n", info.Name)
		docs += fmt.Sprintf("Version: %s\n\n", info.Version)
		docs += fmt.Sprintf("Description: %s\n\n", info.Description)
		if len(info.Tags) > 0 {
			docs += fmt.Sprintf("Tags: %v\n\n", info.Tags)
		}
	}

	return docs
}

// Type-safe result getters
func (r *Result) GetString(key string) string {
	if r.Success && r.Data != nil {
		if data, ok := r.Data.(map[string]interface{}); ok {
			if v, ok := data[key].(string); ok {
				return v
			}
		}
	}
	return ""
}

func (r *Result) GetInt(key string) int {
	if r.Success && r.Data != nil {
		if data, ok := r.Data.(map[string]interface{}); ok {
			if v, ok := data[key].(float64); ok {
				return int(v)
			}
		}
	}
	return 0
}

func (r *Result) GetBool(key string) bool {
	if r.Success && r.Data != nil {
		if data, ok := r.Data.(map[string]interface{}); ok {
			if v, ok := data[key].(bool); ok {
				return v
			}
		}
	}
	return false
}

// MakePlugin is a helper to create a plugin from functions
func MakePlugin(name, version, description string, executeFunc interface{}) (PluginLoader, error) {
	// Validate executeFunc is a function
	rv := reflect.ValueOf(executeFunc)
	if rv.Kind() != reflect.Func {
		return nil, fmt.Errorf("executeFunc must be a function")
	}

	// Create a basic plugin wrapper
	return &dynamicPlugin{
		name:        name,
		version:     version,
		description: description,
		executeFunc: executeFunc,
	}, nil
}

type dynamicPlugin struct {
	name        string
	version     string
	description string
	executeFunc interface{}
}

func (p *dynamicPlugin) PluginInfo() PluginInfo {
	return PluginInfo{
		Name:        p.name,
		Version:     p.version,
		Description: p.description,
	}
}

func (p *dynamicPlugin) Initialize(config map[string]interface{}) error {
	return nil
}

func (p *dynamicPlugin) Execute(args map[string]interface{}) (interface{}, error) {
	// Call the dynamic function
	rv := reflect.ValueOf(p.executeFunc)
	results := rv.Call([]reflect.Value{reflect.ValueOf(args)})
	if len(results) > 0 {
		if err, ok := results[0].Interface().(error); ok && err != nil {
			return nil, err
		}
		if len(results) > 1 {
			return results[1].Interface(), nil
		}
	}
	return nil, nil
}

func (p *dynamicPlugin) Shutdown() error {
	return nil
}

func (p *dynamicPlugin) HealthCheck() error {
	return nil
}
