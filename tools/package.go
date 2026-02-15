// Package tools - OpenClaw-Go tool invocation framework
//
// Provides exec, read, write, process, edit, memory, web, browser, sessions tools
// Also provides adapter-based plugin system for dynamic tool loading
package tools

import (
	"github.com/gliderlab/cogate/memory"
	"github.com/gliderlab/cogate/tools/adapter"
)

// NewDefaultRegistry creates the default registry and registers all tools
func NewDefaultRegistry() *Registry {
	registry := NewRegistry()

	// Register all tools (pointer receivers)
	registry.Register(&ExecTool{})
	registry.Register(&ReadTool{})
	registry.Register(&WriteTool{})
	registry.Register(&EditTool{})
	registry.Register(&ProcessTool{})
	registry.Register(&WebSearchTool{})
	registry.Register(&WebFetchTool{})
	registry.Register(&BrowserTool{})
	registry.Register(&CanvasTool{})
	registry.Register(&NodesTool{})
	registry.Register(&SessionsListTool{})
	registry.Register(&SessionsSendTool{})
	registry.Register(&SessionsSpawnTool{})
	registry.Register(&SessionsHistoryTool{})
	registry.Register(&SessionStatusTool{})
	registry.Register(&AgentsListTool{})
	// Memory tools require storage; initialize separately
	registry.Register(&MemoryTool{Store: nil})
	registry.Register(&MemoryGetTool{Store: nil})
	registry.Register(&MemoryStoreTool{Store: nil})

	return registry
}

// NewMemoryRegistry creates a registry with memory store
func NewMemoryRegistry(store *memory.VectorMemoryStore) *Registry {
	registry := NewRegistry()

	registry.Register(&ExecTool{})
	registry.Register(&ReadTool{})
	registry.Register(&WriteTool{})
	registry.Register(&EditTool{})
	registry.Register(&ProcessTool{})
	registry.Register(&WebSearchTool{})
	registry.Register(&WebFetchTool{})
	registry.Register(&BrowserTool{})
	registry.Register(&CanvasTool{})
	registry.Register(&NodesTool{})
	registry.Register(&SessionsListTool{})
	registry.Register(&SessionsSendTool{})
	registry.Register(&SessionsSpawnTool{})
	registry.Register(&SessionsHistoryTool{})
	registry.Register(&SessionStatusTool{})
	registry.Register(&AgentsListTool{})
	registry.Register(&MemoryTool{Store: store})
	registry.Register(&MemoryGetTool{Store: store})
	registry.Register(&MemoryStoreTool{Store: store})

	return registry
}

// NewAdapterRegistry creates a new plugin-based adapter registry
// This enables dynamic tool loading and plugin support
func NewAdapterRegistry(workspace string) *adapter.ToolAdapter {
	cfg := adapter.DefaultAdapterConfig()
	cfg.PluginDir = workspace + "/plugins"
	return adapter.NewToolAdapter(cfg)
}

// RegisterBuiltinWithAdapter registers all built-in tools with the adapter
func RegisterBuiltinWithAdapter(a *adapter.ToolAdapter) {
	// Register each tool as a plugin wrapper
	a.RegisterPlugin("read", &ReadToolWrapper{})
	a.RegisterPlugin("write", &WriteToolWrapper{})
	a.RegisterPlugin("edit", &EditToolWrapper{})
	a.RegisterPlugin("exec", &ExecToolWrapper{})
	a.RegisterPlugin("process", &ProcessToolWrapper{})
	a.RegisterPlugin("web_search", &WebSearchToolWrapper{})
	a.RegisterPlugin("web_fetch", &WebFetchToolWrapper{})
	a.RegisterPlugin("memory", &MemoryToolWrapper{})
}

// Tool wrapper types for adapter integration

type ReadToolWrapper struct{}
func (w *ReadToolWrapper) PluginInfo() adapter.PluginInfo {
	return adapter.PluginInfo{
		Name:        "read",
		Version:     "1.0.0",
		Description: "Read file contents (max 50KB)",
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{"type": "string", "description": "File path to read"},
			},
			"required": []string{"path"},
		},
	}
}
func (w *ReadToolWrapper) Initialize(cfg map[string]interface{}) error { return nil }
func (w *ReadToolWrapper) Execute(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "read_tool_wrapper"}, nil
}
func (w *ReadToolWrapper) Shutdown() error { return nil }
func (w *ReadToolWrapper) HealthCheck() error { return nil }

type WriteToolWrapper struct{}
func (w *WriteToolWrapper) PluginInfo() adapter.PluginInfo {
	return adapter.PluginInfo{
		Name:        "write",
		Version:     "1.0.0",
		Description: "Write content to a file",
	}
}
func (w *WriteToolWrapper) Initialize(cfg map[string]interface{}) error { return nil }
func (w *WriteToolWrapper) Execute(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "write_tool_wrapper"}, nil
}
func (w *WriteToolWrapper) Shutdown() error { return nil }
func (w *WriteToolWrapper) HealthCheck() error { return nil }

type EditToolWrapper struct{}
func (w *EditToolWrapper) PluginInfo() adapter.PluginInfo {
	return adapter.PluginInfo{
		Name:        "edit",
		Version:     "1.0.0",
		Description: "Edit file contents",
	}
}
func (w *EditToolWrapper) Initialize(cfg map[string]interface{}) error { return nil }
func (w *EditToolWrapper) Execute(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "edit_tool_wrapper"}, nil
}
func (w *EditToolWrapper) Shutdown() error { return nil }
func (w *EditToolWrapper) HealthCheck() error { return nil }

type ExecToolWrapper struct{}
func (w *ExecToolWrapper) PluginInfo() adapter.PluginInfo {
	return adapter.PluginInfo{
		Name:        "exec",
		Version:     "1.0.0",
		Description: "Execute shell commands",
	}
}
func (w *ExecToolWrapper) Initialize(cfg map[string]interface{}) error { return nil }
func (w *ExecToolWrapper) Execute(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "exec_tool_wrapper"}, nil
}
func (w *ExecToolWrapper) Shutdown() error { return nil }
func (w *ExecToolWrapper) HealthCheck() error { return nil }

type ProcessToolWrapper struct{}
func (w *ProcessToolWrapper) PluginInfo() adapter.PluginInfo {
	return adapter.PluginInfo{
		Name:        "process",
		Version:     "1.0.0",
		Description: "Manage background processes",
	}
}
func (w *ProcessToolWrapper) Initialize(cfg map[string]interface{}) error { return nil }
func (w *ProcessToolWrapper) Execute(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "process_tool_wrapper"}, nil
}
func (w *ProcessToolWrapper) Shutdown() error { return nil }
func (w *ProcessToolWrapper) HealthCheck() error { return nil }

type WebSearchToolWrapper struct{}
func (w *WebSearchToolWrapper) PluginInfo() adapter.PluginInfo {
	return adapter.PluginInfo{
		Name:        "web_search",
		Version:     "1.0.0",
		Description: "Search the web",
	}
}
func (w *WebSearchToolWrapper) Initialize(cfg map[string]interface{}) error { return nil }
func (w *WebSearchToolWrapper) Execute(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "web_search_tool_wrapper"}, nil
}
func (w *WebSearchToolWrapper) Shutdown() error { return nil }
func (w *WebSearchToolWrapper) HealthCheck() error { return nil }

type WebFetchToolWrapper struct{}
func (w *WebFetchToolWrapper) PluginInfo() adapter.PluginInfo {
	return adapter.PluginInfo{
		Name:        "web_fetch",
		Version:     "1.0.0",
		Description: "Fetch web page content",
	}
}
func (w *WebFetchToolWrapper) Initialize(cfg map[string]interface{}) error { return nil }
func (w *WebFetchToolWrapper) Execute(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "web_fetch_tool_wrapper"}, nil
}
func (w *WebFetchToolWrapper) Shutdown() error { return nil }
func (w *WebFetchToolWrapper) HealthCheck() error { return nil }

type MemoryToolWrapper struct{}
func (w *MemoryToolWrapper) PluginInfo() adapter.PluginInfo {
	return adapter.PluginInfo{
		Name:        "memory",
		Version:     "1.0.0",
		Description: "Vector memory storage and retrieval",
	}
}
func (w *MemoryToolWrapper) Initialize(cfg map[string]interface{}) error { return nil }
func (w *MemoryToolWrapper) Execute(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "memory_tool_wrapper"}, nil
}
func (w *MemoryToolWrapper) Shutdown() error { return nil }
func (w *MemoryToolWrapper) HealthCheck() error { return nil }
