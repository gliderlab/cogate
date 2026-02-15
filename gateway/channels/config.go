package channels

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ChannelConfig represents the configuration for a channel
type ChannelConfig struct {
	Enabled     bool                   `json:"enabled"`
	Type        ChannelType            `json:"type"`
	Name        string                 `json:"name"`
	Config      map[string]interface{} `json:"config"`
	Permissions map[string]bool        `json:"permissions,omitempty"`
}

// ChannelAdapterConfigV1 represents the adapter configuration
type ChannelAdapterConfigV1 struct {
	Version    string                   `json:"version"`
	Enabled    bool                     `json:"enabled"`
	Webhook    WebhookConfig            `json:"webhook"`
	Polling    PollingConfig            `json:"polling"`
	Channels   map[string]ChannelConfig `json:"channels"`
}

// WebhookConfig holds webhook server configuration
type WebhookConfig struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Path    string `json:"path"`
	Secret  string `json:"secret,omitempty"`
}

// PollingConfig holds polling configuration
type PollingConfig struct {
	Enabled  bool `json:"enabled"`
	Interval int  `json:"intervalSeconds"`
	Limit    int  `json:"limit"`
}

// DefaultChannelAdapterConfig returns the default adapter configuration
func DefaultChannelAdapterConfigV1() *ChannelAdapterConfigV1 {
	return &ChannelAdapterConfigV1{
		Version: "1.0.0",
		Enabled: true,
		Webhook: WebhookConfig{
			Enabled: true,
			Host:    "127.0.0.1",
			Port:    8787,
			Path:    "/webhook",
		},
		Polling: PollingConfig{
			Enabled:  false,
			Interval: 30,
			Limit:    100,
		},
		Channels: make(map[string]ChannelConfig),
	}
}

// ConfigManager manages channel configuration files
type ConfigManager struct {
	configDir string
	configFile string
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configDir string) *ConfigManager {
	return &ConfigManager{
		configDir:  configDir,
		configFile: filepath.Join(configDir, "channels.json"),
	}
}

// Save saves the configuration to file
func (c *ConfigManager) Save(config *ChannelAdapterConfigV1) error {
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
func (c *ConfigManager) Load() (*ChannelAdapterConfigV1, error) {
	data, err := os.ReadFile(c.configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultChannelAdapterConfigV1(), nil
		}
		return nil, err
	}

	var config ChannelAdapterConfigV1
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadOrCreate loads existing config or creates a new one
func (c *ConfigManager) LoadOrCreate() (*ChannelAdapterConfigV1, error) {
	config, err := c.Load()
	if err != nil {
		config = DefaultChannelAdapterConfigV1()
		if err := c.Save(config); err != nil {
			return nil, err
		}
	}
	return config, nil
}

// AddChannel adds or updates a channel configuration
func (c *ConfigManager) AddChannel(name string, config ChannelConfig) error {
	cfg, err := c.LoadOrCreate()
	if err != nil {
		return err
	}
	
	cfg.Channels[name] = config
	return c.Save(cfg)
}

// RemoveChannel removes a channel configuration
func (c *ConfigManager) RemoveChannel(name string) error {
	cfg, err := c.LoadOrCreate()
	if err != nil {
		return err
	}
	
	delete(cfg.Channels, name)
	return c.Save(cfg)
}

// GetChannel gets a channel configuration
func (c *ConfigManager) GetChannel(name string) (*ChannelConfig, error) {
	cfg, err := c.LoadOrCreate()
	if err != nil {
		return nil, err
	}

	if ch, ok := cfg.Channels[name]; ok {
		return &ch, nil
	}
	return nil, nil
}

// ChannelManifest represents a channel plugin manifest
type ChannelManifest struct {
	Info   ChannelInfo `json:"info"`
	Schema SchemaInfo  `json:"schema"`
}

// SchemaInfo represents the schema information for a channel
type SchemaInfo struct {
	Version    string   `json:"version"`
	APIVersion string   `json:"apiVersion"`
	Endpoints  []string `json:"endpoints"`
}

// CreateTelegramManifest creates a manifest for the Telegram channel
func CreateTelegramManifest() ChannelManifest {
	return ChannelManifest{
		Info: ChannelInfo{
			Name:        "Telegram Bot",
			Type:        ChannelTelegram,
			Version:     "1.0.0",
			Description: "Telegram Bot API integration with webhook and polling support",
			Author:      "OpenClaw-Go",
			Capabilities: []string{
				"text_messages",
				"webhook",
				"long_polling",
				"media",
				"inline_buttons",
				"reactions",
			},
		},
		Schema: SchemaInfo{
			Version:    "1.0.0",
			APIVersion: "1.0.0",
			Endpoints:  []string{"/webhook", "/sendMessage", "/getUpdates"},
		},
	}
}

// SaveManifest saves a channel manifest to file
func SaveManifest(manifest ChannelManifest, filePath string) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// LoadManifest loads a channel manifest from file
func LoadManifest(filePath string) (*ChannelManifest, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var manifest ChannelManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}
