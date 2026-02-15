# Tool Adapter System (Plugin-Based Tools)

OpenClaw-Go's tool adapter system provides plugin-based tool management with dynamic loading, hot reload, and loose coupling.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                   ToolAdapter (Core Adapter)           │
├─────────────────────────────────────────────────────────┤
│  - plugins map[name]PluginLoader                        │
│  - PluginRegistry                                       │
│  - AdapterConfig                                        │
└──────────────┬──────────────────┬──────────────────────┘
               │                  │
    ┌──────────▼──────┐   ┌───────▼────────┐
    │  PluginLoader   │   │ JSONPluginLoader│
    │  (.so files)    │   │ (.json configs) │
    └─────────────────┘   └─────────────────┘
```

## Quick Start

### 1. Create an adapter

```go
import "github.com/gliderlab/cogate/tools/adapter"

// Create with default config
adapter := adapter.NewToolAdapter(adapter.DefaultAdapterConfig())

// Or create with custom config
cfg := adapter.AdapterConfig{
    PluginDir:      "./plugins",
    AutoReload:     true,
    ReloadInterval: 60,
    MaxRetries:     3,
    Timeout:        30,
}
adapter := adapter.NewToolAdapter(cfg)
```

### 2. Register built-in tools

```go
// Option 1: register a function directly
adapter.RegisterPlugin("my_tool", adapter.ConvertToolToPlugin(
    "my_tool",
    "My custom tool description",
    func(args map[string]interface{}) (interface{}, error) {
        return map[string]interface{}{"result": "done"}, nil
    },
))

// Option 2: use MakePlugin
plugin, _ := adapter.MakePlugin("math", "1.0.0", "Math operations", 
    func(args map[string]interface{}) (interface{}, error) {
        a := args["a"].(float64)
        b := args["b"].(float64)
        return map[string]interface{}{"sum": a + b}, nil
    },
)
adapter.RegisterPlugin("math", plugin)
```

### 3. Execute a tool

```go
// Create an execution context
ctx := &adapter.Context{
    AgentName:  "main",
    SessionID:  "session-123",
    Workspace:  "/path/to/workspace",
    UserID:     "user-456",
    Channel:    "telegram",
}

// Execute the tool
result := adapter.ExecuteTool("my_tool", map[string]interface{}{
    "param1": "value1",
    "param2": 123,
}, ctx)

if result.Success {
    fmt.Println(result.Data)
} else {
    fmt.Println("Error:", result.Error)
}
```

## Plugin Development

### 1. Implement the PluginLoader interface

```go
package myplugin

import "github.com/gliderlab/cogate/tools/adapter"

type MyPlugin struct{}

func (p *MyPlugin) PluginInfo() adapter.PluginInfo {
    return adapter.PluginInfo{
        Name:        "my_plugin",
        Version:     "1.0.0",
        Description: "My custom plugin",
        Author:      "Developer Name",
        Schema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "param1": map[string]interface{}{
                    "type":        "string",
                    "description": "First parameter",
                },
            },
            "required": []string{"param1"},
        },
    }
}

func (p *MyPlugin) Initialize(config map[string]interface{}) error {
    // Initialize plugin resources
    return nil
}

func (p *MyPlugin) Execute(args map[string]interface{}) (interface{}, error) {
    // Execute tool logic
    return map[string]interface{}{"status": "ok"}, nil
}

func (p *MyPlugin) Shutdown() error {
    // Cleanup resources
    return nil
}

func (p *MyPlugin) HealthCheck() error {
    return nil
}
```

### 2. Build as a shared library (.so)

```go
// main.go for plugin
package main

import "github.com/gliderlab/cogate/tools/adapter"

type MyPlugin struct{}

func (p *MyPlugin) PluginInfo() adapter.PluginInfo {
    return adapter.PluginInfo{
        Name:        "my_plugin",
        Version:     "1.0.0",
        Description: "My shared library plugin",
    }
}

func (p *MyPlugin) Initialize(config map[string]interface{}) error { return nil }
func (p *MyPlugin) Execute(args map[string]interface{}) (interface{}, error) { return nil, nil }
func (p *MyPlugin) Shutdown() error { return nil }
func (p *MyPlugin) HealthCheck() error { return nil }

// Export symbol
var ToolPlugin = &MyPlugin{}
```

Build:
```bash
go build -buildmode=plugin -o my_plugin.so main.go
```

### 3. Load from a shared library

```go
loader := plugin.NewPluginLoader(adapter, "./plugins")
if err := loader.LoadPlugin("./plugins/my_plugin.so"); err != nil {
    log.Fatal(err)
}
```

## JSON Plugin Configuration

Create `config/plugins/my_plugin.json`:

```json
{
  "name": "my_plugin",
  "version": "1.0.0",
  "description": "My JSON configured plugin",
  "author": "Developer",
  "tags": ["tools", "custom"],
  "type": "builtin",
  "config": {
    "option1": "value1",
    "option2": 123
  }
}
```

Load JSON plugins:
```go
jsonLoader := plugin.NewJSONPluginLoader(adapter, "./config/plugins")
if err := jsonLoader.LoadAllPlugins(); err != nil {
    log.Fatal(err)
}
```

## Built-in Tool Mapping

| Module | JSON type | Description |
|--------|-----------|-------------|
| `tools.read` | builtin | File read |
| `tools.write` | builtin | File write |
| `tools.exec` | builtin | Command execution |
| `tools.memory` | builtin | Vector memory |
| `tools.process` | builtin | Process management |
| `tools.web` | builtin | Web tools |
| `tools.telegram` | builtin | Telegram integration |

## Tool Specs

Get OpenAI-compatible tool specs:

```go
specs := adapter.GetAllToolSpecs()
for _, spec := range specs {
    fmt.Printf("Tool: %s\n", spec.Function.Name)
    fmt.Printf("Description: %s\n", spec.Function.Description)
}
```

## Configuration Management

```go
configMgr := adapter.NewConfigManager("./config")

// Load or create config
config, err := configMgr.LoadOrCreate()
if err != nil {
    log.Fatal(err)
}

// Update config
config.AutoReload = true
config.MaxRetries = 5

// Save config
if err := configMgr.Save(config); err != nil {
    log.Fatal(err)
}
```

## API Reference

### ToolAdapter Methods

| Method | Description |
|--------|-------------|
| `RegisterPlugin(name, plugin)` | Register a plugin |
| `UnregisterPlugin(name)` | Unregister a plugin |
| `ExecuteTool(name, args, ctx)` | Execute a tool |
| `GetToolSpec(name)` | Get a tool spec |
| `GetAllToolSpecs()` | Get all tool specs |
| `ListTools()` | List all tools |
| `HasTool(name)` | Check whether a tool exists |
| `GetToolCount()` | Get tool count |
| `GetPluginInfo(name)` | Get plugin info |
| `GetToolDocumentation()` | Get tool docs |
| `Shutdown()` | Shutdown all plugins |

### PluginLoader Methods

| Method | Description |
|--------|-------------|
| `LoadPlugin(filePath)` | Load a single plugin |
| `LoadAllPlugins()` | Load all plugins |
| `UnloadPlugin(name)` | Unload a plugin |
| `ReloadPlugin(name, filePath)` | Reload a plugin |

## Best Practices

1. **Plugin naming**: use lowercase letters and hyphens (e.g., `my-tool`, `file-reader`)
2. **Versioning**: follow semantic versioning (e.g., `1.0.0`, `1.2.1`)
3. **Error handling**: always return meaningful errors
4. **Health checks**: implement HealthCheck for monitoring
5. **Config separation**: use JSON config files for plugin settings

## Example Project Structure

```
project/
├── adapters/
│   └── adapter.go
├── plugins/
│   ├── my_plugin.so
│   └── another_plugin.so
├── config/
│   └── plugins/
│       ├── my_plugin.json
│       └── another_plugin.json
└── main.go
```
