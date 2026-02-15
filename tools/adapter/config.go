// Package adapter - Configuration utilities
package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ConfigManager manages adapter configuration
type ConfigManager struct {
	configDir  string
	configFile string
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configDir string) *ConfigManager {
	return &ConfigManager{
		configDir:  configDir,
		configFile: filepath.Join(configDir, "adapter.json"),
	}
}

// AdapterConfigV1 represents the adapter configuration version 1
type AdapterConfigV1 struct {
	Version     string                 `json:"version"`
	PluginDir   string                 `json:"pluginDir"`
	ConfigDir   string                 `json:"configDir"`
	AutoReload  bool                   `json:"autoReload"`
	ReloadSecs  int                    `json:"reloadIntervalSeconds"`
	MaxRetries  int                    `json:"maxRetries"`
	TimeoutSecs int                    `json:"defaultTimeoutSeconds"`
	Plugins     map[string]PluginConfigV1 `json:"plugins"`
}

// PluginConfigV1 represents a plugin configuration
type PluginConfigV1 struct {
	Enabled     bool                   `json:"enabled"`
	Type        string                 `json:"type"` // "builtin", "external", "wasm"
	Config      map[string]interface{} `json:"config"`
	Permissions map[string]bool        `json:"permissions,omitempty"`
}

// Save saves the configuration to file
func (c *ConfigManager) Save(config *AdapterConfigV1) error {
	// Ensure directory exists
	if err := os.MkdirAll(c.configDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.configFile, data, 0644)
}

// Load loads the configuration from file
func (c *ConfigManager) Load() (*AdapterConfigV1, error) {
	data, err := os.ReadFile(c.configFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config
			return DefaultConfigV1(), nil
		}
		return nil, err
	}

	var config AdapterConfigV1
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadOrCreate loads existing config or creates a new one
func (c *ConfigManager) LoadOrCreate() (*AdapterConfigV1, error) {
	config, err := c.Load()
	if err != nil {
		// Create new config
		config = DefaultConfigV1()
		if err := c.Save(config); err != nil {
			return nil, err
		}
	}
	return config, nil
}

// DefaultConfigV1 returns the default configuration
func DefaultConfigV1() *AdapterConfigV1 {
	return &AdapterConfigV1{
		Version:     "1.0.0",
		PluginDir:   "./plugins",
		ConfigDir:   "./config/plugins",
		AutoReload:  false,
		ReloadSecs:  60,
		MaxRetries:  3,
		TimeoutSecs: 30,
		Plugins:     make(map[string]PluginConfigV1),
	}
}

// PluginManifest represents a plugin manifest file
type PluginManifest struct {
	Info     PluginInfo   `json:"info"`
	Manifest ManifestData `json:"manifest"`
}

// ManifestData contains manifest data
type ManifestData struct {
	Version       string   `json:"version"`
	APIVersion    string   `json:"apiVersion"`
	Capabilities  []string `json:"capabilities"`
	Dependencies  []string `json:"dependencies,omitempty"`
	Entrypoint    string   `json:"entrypoint,omitempty"`
}

// CreatePluginManifest creates a plugin manifest
func CreatePluginManifest(info PluginInfo, capabilities []string) PluginManifest {
	return PluginManifest{
		Info: info,
		Manifest: ManifestData{
			Version:    "1.0.0",
			APIVersion: "1.0.0",
			Capabilities: capabilities,
		},
	}
}

// SaveManifest saves a plugin manifest to file
func SaveManifest(manifest PluginManifest, filePath string) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// LoadManifest loads a plugin manifest from file
func LoadManifest(filePath string) (*PluginManifest, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// Capability constants
const (
	CapabilityRead      = "read"
	CapabilityWrite     = "write"
	CapabilityExec      = "exec"
	CapabilityNetwork   = "network"
	CapabilityMemory    = "memory"
	CapabilityFilesystem = "filesystem"
	CapabilityProcess   = "process"
	CapabilityWeb       = "web"
	CapabilityBrowser   = "browser"
	CapabilityCanvas    = "canvas"
	CapabilityNodes     = "nodes"
	CapabilityCron      = "cron"
	CapabilityGateway   = "gateway"
	CapabilityMessaging = "messaging"
)
