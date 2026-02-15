# Tools System

OpenClaw-Go provides a flexible tool system based on the **Adapter Pattern** for extending agent capabilities.

## Architecture - Adapter Pattern

```
┌─────────────────────────────────────────────────────────────────┐
│                         Agent                                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Tool Registry (Adapter)                     │   │
│  │  ┌─────────────────────────────────────────────────┐    │   │
│  │  │           Tool Adapter (Core)                   │    │   │
│  │  │  ┌─────────────────────────────────────────┐  │    │   │
│  │  │  │         Plugin Loader                    │  │    │   │
│  │  │  └─────────────────────────────────────────┘  │    │   │
│  │  └─────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                  │
│         ┌──────────────────────┼──────────────────────┐       │
│         │                      │                      │         │
│         ▼                      ▼                      ▼         │
│  ┌─────────────┐      ┌─────────────┐      ┌─────────────┐  │
│  │  Built-in   │      │   Plugin    │      │   Remote    │  │
│  │   Tools     │      │   Tools     │      │   Tools     │  │
│  │ (exec,read) │      │  (dynamic)  │      │  (RPC)      │  │
│  └─────────────┘      └─────────────┘      └─────────────┘  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Adapter Pattern Components

### 1. Tool Interface (Target)

```go
// Target interface - what clients expect
type Tool interface {
    Name() string           // Tool name
    Description() string   // Help text
    Parameters() map[string]interface{}  // JSON Schema
    Execute(args map[string]interface{}) (interface{}, error)
}
```

### 2. Tool Adapter (Adapter)

```go
// Adapter - wraps tools and provides unified interface
type ToolAdapter struct {
    plugins  map[string]PluginLoader
    registry *PluginRegistry
    config   AdapterConfig
}

// AdapterConfig - configuration for the adapter
type AdapterConfig struct {
    PluginDir      string
    AutoReload     bool
    ReloadInterval int
    MaxRetries     int
    Timeout        int
}
```

### 3. Plugin Loader (Adaptee)

```go
// Plugin interface - for external plugins
type PluginLoader interface {
    PluginInfo() PluginInfo       // Metadata
    Initialize(config map[string]interface{}) error
    Execute(args map[string]interface{}) (interface{}, error)
    Shutdown() error
    HealthCheck() error
}
```

### 4. Registry

```go
// Registry manages tool registration
type Registry struct {
    tools map[string]Tool
    mu    sync.RWMutex
}

func (r *Registry) Register(tool Tool) error
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) List() []string
func (r *Registry) Execute(name string, args map[string]interface{}) (interface{}, error)
```

## Implementation

### Tool Adapter

```go
// ToolAdapter provides plugin-based tool loading
type ToolAdapter struct {
    plugins  map[string]PluginLoader
    registry *PluginRegistry
    config   AdapterConfig
    mu       sync.RWMutex
}

// NewToolAdapter creates a new adapter
func NewToolAdapter(cfg AdapterConfig) *ToolAdapter {
    return &ToolAdapter{
        plugins:  make(map[string]PluginLoader),
        registry: NewPluginRegistry(),
        config:   cfg,
    }
}

// RegisterPlugin registers a plugin
func (a *ToolAdapter) RegisterPlugin(name string, plugin PluginLoader) error {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    if err := plugin.Initialize(nil); err != nil {
        return err
    }
    
    a.plugins[name] = plugin
    a.registry.Register(&PluginWrapper{name: name, plugin: plugin})
    return nil
}

// Execute runs a tool
func (a *ToolAdapter) Execute(name string, args map[string]interface{}) (interface{}, error) {
    a.mu.RLock()
    plugin, ok := a.plugins[name]
    a.mu.RUnlock()
    
    if !ok {
        return nil, fmt.Errorf("tool not found: %s", name)
    }
    
    return plugin.Execute(args)
}
```

### Plugin Registry

```go
// PluginRegistry manages registered plugins
type PluginRegistry struct {
    plugins map[string]PluginLoader
    mu      sync.RWMutex
}

func NewPluginRegistry() *PluginRegistry {
    return &PluginRegistry{
        plugins: make(map[string]PluginLoader),
    }
}

func (r *PluginRegistry) Register(name string, plugin PluginLoader) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.plugins[name] = plugin
}

func (r *PluginRegistry) Get(name string) (PluginLoader, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    p, ok := r.plugins[name]
    return p, ok
}

func (r *PluginRegistry) List() []string {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    names := make([]string, 0, len(r.plugins))
    for name := range r.plugins {
        names = append(names, name)
    }
    return names
}
```

## Built-in Tools

### Core Tools

| Tool | Status | Description |
|------|--------|-------------|
| `exec` | ✅ Complete | Execute shell commands |
| `read` | ✅ Complete | Read files (50KB limit) |
| `write` | ✅ Complete | Write files |
| `edit` | ✅ Complete | Edit files (diff-based) |
| `process` | ✅ Complete | Process management |

### Memory Tools

| Tool | Status | Description |
|------|--------|-------------|
| `memory` | ✅ Complete | Vector search |
| `memory_get` | ✅ Complete | Get memory by path |
| `memory_store` | ✅ Complete | Store memory |

### System Tools

| Tool | Status | Description |
|------|--------|-------------|
| `pulse` | ✅ Complete | Heartbeat events |
| `session_status` | ⚠️ Basic | Session info |
| `agents_list` | ⚠️ Basic | List agents |

### Web Tools

| Tool | Status | Description |
|------|--------|-------------|
| `web_search` | ⚠️ Basic | Web search |
| `web_fetch` | ⚠️ Basic | Fetch URL content |

### Unimplemented

| Tool | Status | Description |
|------|--------|-------------|
| `browser` | ❌ Mock | Browser control |
| `canvas` | ❌ Mock | Canvas control |
| `nodes` | ❌ Mock | Node management |
| `cron` | ❌ Mock | Cron jobs |
| `message` | ❌ Mock | Message send |

## Creating Tools

### Direct Implementation

```go
type HelloTool struct{}

func (t *HelloTool) Name() string        { return "hello" }
func (t *HelloTool) Description() string { return "Say hello" }

func (t *HelloTool) Parameters() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "name": map[string]interface{}{
                "type": "string",
                "description": "Name to greet",
            },
        },
        "required": []string{"name"},
    }
}

func (t *HelloTool) Execute(args map[string]interface{}) (interface{}, error) {
    name := args["name"].(string)
    return fmt.Sprintf("Hello, %s!", name), nil
}

// Register
registry.Register(&HelloTool{})
```

### Plugin-based (Adapter Pattern)

```go
// 1. Create plugin
type HelloPlugin struct {
    config map[string]interface{}
}

func (p *HelloPlugin) PluginInfo() PluginInfo {
    return PluginInfo{
        Name:        "hello",
        Version:     "1.0.0",
        Description: "Hello world plugin",
    }
}

func (p *HelloPlugin) Initialize(config map[string]interface{}) error {
    p.config = config
    return nil
}

func (p *HelloPlugin) Execute(args map[string]interface{}) (interface{}, error) {
    name := args["name"].(string)
    return fmt.Sprintf("Hello, %s!", name), nil
}

func (p *HelloPlugin) Shutdown() error   { return nil }
func (p *HelloPlugin) HealthCheck() error { return nil }

// 2. Register as plugin
adapter := NewToolAdapter(DefaultAdapterConfig())
adapter.RegisterPlugin("hello", &HelloPlugin{})
```

## Registry Usage

```go
// Create registry
registry := tools.NewDefaultRegistry()

// Register built-in tools
registry.Register(&ExecTool{})
registry.Register(&ReadTool{})
registry.Register(&WriteTool{})

// List tools
for _, name := range registry.List() {
    fmt.Println(name)
}

// Execute tool
result, err := registry.Execute("exec", map[string]interface{}{
    "command": "ls -la",
})
```

## Adapter Configuration

```go
config := AdapterConfig{
    PluginDir:      "./plugins",
    AutoReload:     true,
    ReloadInterval: 60,  // seconds
    MaxRetries:     3,
    Timeout:        30,   // seconds
}

adapter := NewToolAdapter(config)
```

## File Structure

```
tools/
├── tools.go           # Tool interface & Registry
├── exec.go           # exec tool implementation
├── read.go           # read tool implementation
├── write.go          # write tool implementation
├── edit.go           # edit tool implementation
├── process.go        # process tool implementation
├── memory.go         # memory tool implementation
├── web.go            # web search/fetch tools
├── sessions.go       # session tools
├── browser.go        # browser tool (stub)
├── pulse.go         # pulse tool
├── package.go        # Tool package initialization
├── adapter/
│   ├── adapter.go   # ToolAdapter implementation
│   ├── config.go    # Configuration
│   ├── init.go      # Package init
│   └── ADAPTER.md  # Adapter documentation
└── plugins/
    └── (plugin files)
```

## Benefits of Adapter Pattern

1. **Unified Interface** - All tools accessible via same API
2. **Plugin Support** - Dynamic tool loading
3. **Hot Reload** - Load/refresh tools without restart
4. **Extensibility** - Easy to add new tools
5. **Loose Coupling** - Tools independent of core

## Security

### Current Limitations

- `exec`: No command whitelist
- `read`: 50KB file limit
- `write`: No path restrictions

### Recommendations

```go
// Add security wrapper
type SecureAdapter struct {
    adapter  *ToolAdapter
    allowlist []string
}

func (s *SecureAdapter) Execute(name string, args map[string]interface{}) (interface{}, error) {
    // Check allowlist
    if !s.isAllowed(name) {
        return nil, fmt.Errorf("tool not allowed: %s", name)
    }
    return s.adapter.Execute(name, args)
}

func (s *SecureAdapter) isAllowed(name string) bool {
    for _, allowed := range s.allowlist {
        if name == allowed {
            return true
        }
    }
    return false
}
```

## Comparison with Channels

| Aspect | Tools Adapter | Channels Adapter |
|--------|---------------|-----------------|
| Pattern | Adapter | Adapter |
| Purpose | Extend capabilities | Multi-platform messaging |
| Examples | exec, read, memory | Telegram, Discord, Slack |
| Loading | Built-in + Plugins | Built-in only |
| Status | ✅ Implemented | ✅ Telegram, 🔶 Others |
