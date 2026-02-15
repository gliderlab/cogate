# Channel Adapter System

OpenClaw-Go's channel adapter system provides plugin-based communication channel management and supports Telegram, WhatsApp, Slack, Discord, and more.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Gateway (Main Service)                   │
├─────────────────────────────────────────────────────────────┤
│              ChannelAdapter (Core Adapter)                  │
├───────────────┬───────────────┬───────────────────────────┤
│ Telegram     │ WhatsApp      │ Slack / Discord / ...     │
│ Channel     │ Channel       │ Other Channels             │
│ (Implemented)│ (Planned)     │ (Extensible)               │
└───────────────┴───────────────┴───────────────────────────┘
```

## File Structure

```
/opt/openclaw-go/gateway/
├── gateway.go              # Main gateway (channel adapter integrated)
├── telegram_handler.go     # Telegram webhook handler
└── channels/               # Channel adapter module
    ├── channel_adapter.go  # Core adapter (ChannelAdapter, ChannelLoader)
    ├── config.go           # Config management
    ├── bot.go              # Telegram channel implementation
    └── init.go             # Package init
```

## Quick Start

### 1. Configure environment variables

```bash
export TELEGRAM_BOT_TOKEN="your_bot_token_from_botfather"
export OPENCLAW_PORT=55003
```

### 2. Start the Gateway

```bash
./bin/ocg-gateway
```

The gateway will automatically:
- Initialize the channel adapter
- Register the Telegram channel (if a token is configured)
- Start the Telegram webhook listener

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/telegram/webhook` | POST | Telegram webhook (public) |
| `/telegram/setWebhook` | POST | Configure webhook |
| `/telegram/status` | GET | Channel status |

## Core Interface

### ChannelLoader Interface

```go
type ChannelLoader interface {
    ChannelInfo() ChannelInfo      // Channel metadata
    Initialize(config) error       // Initialize
    Start() error                 // Start listener
    Stop() error                  // Stop listener
    SendMessage(req) (resp, error) // Send message
    HandleWebhook(w, r)          // Webhook handler
    HealthCheck() error           // Health check
}
```

### Implemented Channels

| Channel | Status | Features |
|---------|--------|----------|
| Telegram | ✅ Implemented | Messaging, webhook, command handling |

## Building a New Channel Plugin

### 1. Implement the ChannelLoader interface

```go
package mychannel

import "github.com/gliderlab/cogate/gateway/channels"

type MyChannel struct{}

func (c *MyChannel) ChannelInfo() channels.ChannelInfo {
    return channels.ChannelInfo{
        Name:        "My Channel",
        Type:        channels.ChannelType("mychannel"),
        Version:     "1.0.0",
        Description: "My custom channel",
    }
}

func (c *MyChannel) Initialize(config map[string]interface{}) error {
    // Initialize config
    return nil
}

func (c *MyChannel) Start() error {
    // Start listener
    return nil
}

func (c *MyChannel) Stop() error {
    // Stop listener
    return nil
}

func (c *MyChannel) SendMessage(req *channels.SendMessageRequest) (*channels.SendMessageResponse, error) {
    // Send message
    return &channels.SendMessageResponse{OK: true}, nil
}

func (c *MyChannel) HandleWebhook(w http.ResponseWriter, r *http.Request) {
    // Webhook handler
}

func (c *MyChannel) HealthCheck() error {
    return nil
}
```

### 2. Register the channel

```go
adapter := channels.NewChannelAdapter(channels.DefaultChannelAdapterConfig(), agentRPC)
adapter.RegisterChannel(&MyChannel{})
adapter.StartAllChannels()
```

## Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `TELEGRAM_BOT_TOKEN` | Telegram Bot Token |
| `TELEGRAM_WEBHOOK_PORT` | Webhook port (default 8787) |
| `TELEGRAM_WEBHOOK_HOST` | Webhook host |

### Channel Configuration Example

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "config": {
        "token": "your_token",
        "webhookUrl": "https://your-domain.com/webhook"
      }
    }
  }
}
```

## Usage Examples

### Send a message

```go
adapter := g.channelAdapter
adapter.SendMessage(channels.ChannelTelegram, &channels.SendMessageRequest{
    ChatID: 123456789,
    Text:   "Hello from OpenClaw-Go!",
})
```

### Check channel status

```go
channels := adapter.ListChannels()
// Returns: [telegram]
```

### Health check

```go
results := adapter.HealthCheck()
// Returns: map[channelType]error
```

## Channel Type Constants

```go
const (
    ChannelTelegram ChannelType = "telegram"
    ChannelWhatsApp ChannelType = "whatsapp"
    ChannelSlack    ChannelType = "slack"
    ChannelDiscord  ChannelType = "discord"
    ChannelWebChat  ChannelType = "webchat"
)
```

## Best Practices

1. **Error handling**: always return meaningful error messages
2. **Health checks**: implement HealthCheck for monitoring
3. **Config separation**: use JSON config files for channel settings
4. **Graceful shutdown**: clean up resources in Stop()
