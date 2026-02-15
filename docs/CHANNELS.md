# Channel Adapters

The Channel Adapter system provides a plugin-based architecture for supporting multiple messaging platforms.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Gateway                               │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────┐    │
│  │              Channel Adapter                         │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────┐  │    │
│  │  │  Telegram   │  │  WhatsApp   │  │  Slack  │  │    │
│  │  │  (✅)       │  │  (🔶)       │  │  (🔶)   │  │    │
│  │  └─────────────┘  └─────────────┘  └─────────┘  │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

## Channel Interface

```go
type ChannelLoader interface {
    ChannelInfo() ChannelInfo      // Channel metadata
    Initialize(config) error      // Initialize
    Start() error                // Start listening
    Stop() error                 // Stop
    SendMessage(req) (resp, error)  // Send message
    HandleWebhook(w, r)          // Webhook handler
    HealthCheck() error         // Health check
}

type ChannelInfo struct {
    Name        string
    Type        ChannelType
    Version     string
    Description string
    Author      string
    Capabilities []string
}
```

## Implemented Channels

### ✅ Telegram

**Status**: Production Ready

**Features**:
- Text messages
- Commands (/start, /help, /stats)
- Webhook & Polling
- Media support
- Inline buttons
- Reactions

**Setup**:
```bash
export TELEGRAM_BOT_TOKEN=your_bot_token
./bin/ocg-gateway
```

**Configuration**:
```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "botToken": "123:ABC",
      "dmPolicy": "pairing",
      "groups": {
        "*": {
          "requireMention": true
        }
      }
    }
  }
}
```

**Commands**:
| Command | Description |
|---------|-------------|
| `/start` | Start bot |
| `/help` | Help info |
| `/stats` | System stats |
| `/reset` | Reset greeting |

**Webhook Endpoint**:
```
POST /telegram/webhook
```

**API**:
```bash
# Set webhook
curl -X POST /telegram/setWebhook \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"url": "https://your-domain.com/telegram/webhook"}'

# Get status
curl /telegram/status \
  -H "Authorization: Bearer $TOKEN"
```

### 🔶 WhatsApp

**Status**: Planned

**Features** (planned):
- Text messages
- Media messages
- Polls
- Group messages

### 🔶 Discord

**Status**: Planned

**Features** (planned):
- Text messages
- Embeds
- Slash commands
- Reactions
- Threads

### 🔶 Slack

**Status**: Planned

**Features** (planned):
- Messages
- Slash commands
- Interactive messages
- Webhooks

### 🔶 Signal

**Status**: Planned

## Channel Features Comparison

| Feature | Telegram | WhatsApp | Discord | Slack |
|---------|----------|----------|---------|-------|
| Text | ✅ | 🔶 | 🔶 | 🔶 |
| Media | ✅ | 🔶 | 🔶 | 🔶 |
| Commands | ✅ | 🔶 | 🔶 | 🔶 |
| Buttons | ✅ | 🔶 | 🔶 | 🔶 |
| Polls | ❌ | 🔶 | 🔶 | ❌ |
| Threads | ✅ | ❌ | 🔶 | 🔶 |
| Reactions | ✅ | ❌ | 🔶 | 🔶 |
| Webhooks | ✅ | 🔶 | 🔶 | 🔶 |

## Creating Custom Channel

### Example: Custom Channel

```go
package channels

type CustomChannel struct {
    token   string
    apiURL  string
    client  *http.Client
    running bool
}

func NewCustomChannel(token string) *CustomChannel {
    return &CustomChannel{
        token:  token,
        apiURL: "https://api.custom.com",
        client: &http.Client{Timeout: 30 * time.Second},
    }
}

func (c *CustomChannel) ChannelInfo() ChannelInfo {
    return ChannelInfo{
        Name:        "Custom Channel",
        Type:        ChannelType("custom"),
        Version:     "1.0.0",
        Description: "Custom messaging channel",
        Capabilities: []string{"text", "media"},
    }
}

func (c *CustomChannel) Initialize(config map[string]interface{}) error {
    if token, ok := config["token"].(string); ok {
        c.token = token
    }
    return nil
}

func (c *CustomChannel) Start() error {
    c.running = true
    // Start polling or webhooks
    return nil
}

func (c *CustomChannel) Stop() error {
    c.running = false
    return nil
}

func (c *CustomChannel) SendMessage(req SendMessageRequest) (SendMessageResponse, error) {
    // Implement sending
    return SendMessageResponse{MessageID: "123"}, nil
}

func (c *CustomChannel) HandleWebhook(w http.ResponseWriter, r *http.Request) {
    // Handle incoming webhook
}

func (c *CustomChannel) HealthCheck() error {
    if !c.running {
        return fmt.Errorf("channel not running")
    }
    return nil
}
```

### Register Channel

```go
adapter := channels.NewChannelAdapter(config)
adapter.RegisterChannel(NewCustomChannel(token))
adapter.StartChannel(ChannelCustom)
```

## Message Handling

### Incoming Messages

```go
type IncomingMessage struct {
    ID        string
    Channel   ChannelType
    From      User
    Chat      Chat
    Text      string
    Media     []byte
    Timestamp time.Time
}
```

### Outgoing Messages

```go
type SendMessageRequest struct {
    To        string
    Text      string
    Media     string
    Buttons   []Button
    ReplyTo   string
}

type SendMessageResponse struct {
    MessageID string
    Error    error
}
```

## Delivery Modes

### Direct

Send directly to channel:

```go
resp, err := channel.SendMessage(SendMessageRequest{
    To:   "user123",
    Text: "Hello!",
})
```

### Broadcast

Send to multiple recipients:

```go
for _, user := range users {
    channel.SendMessage(SendMessageRequest{
        To:   user,
        Text: "Broadcast message",
    })
}
```

## Configuration

### Global Config

```json
{
  "channels": {
    "enabled": true,
    "defaultChannel": "telegram"
  }
}
```

### Per-Channel Config

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "botToken": "xxx",
      "dmPolicy": "pairing",
      "groupPolicy": "allowlist"
    }
  }
}
```

## Webhook Security

### Verification

```go
func (c *CustomChannel) verifyWebhook(r *http.Request) bool {
    // Verify webhook signature
    signature := r.Header.Get("X-Signature")
    expected := computeSignature(r.Body, secret)
    return signature == expected
}
```

### Rate Limiting

```go
type RateLimiter struct {
    requests map[string][]time.Time
    mu       sync.Mutex
    limit    int
    window   time.Duration
}

func (r *RateLimiter) Allow(key string) bool {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    now := time.Now()
    cutoff := now.Add(-r.window)
    
    // Filter old requests
    var recent []time.Time
    for _, t := range r.requests[key] {
        if t.After(cutoff) {
            recent = append(recent, t)
        }
    }
    
    if len(recent) >= r.limit {
        return false
    }
    
    r.requests[key] = append(recent, now)
    return true
}
```

## Error Handling

### Channel Errors

```go
type ChannelError struct {
    Channel ChannelType
    Code    string
    Message string
}

func (e *ChannelError) Error() string {
    return fmt.Sprintf("[%s] %s: %s", e.Channel, e.Code, e.Message)
}
```

### Common Errors

| Error | Description | Solution |
|-------|-------------|----------|
| `AUTH_FAILED` | Invalid bot token | Check token |
| `RATE_LIMIT` | Too many requests | Wait and retry |
| `NOT_FOUND` | User/channel not found | Check ID |
| `PERMISSION_DENIED` | No access | Check permissions |

## Monitoring

### Health Check

```bash
curl /channels/health
```

Response:
```json
{
  "telegram": {
    "status": "ok",
    "lastMessage": "2026-02-15T10:00:00Z"
  }
}
```

### Stats

```bash
curl /channels/stats
```

Response:
```json
{
  "messages_sent": 100,
  "messages_received": 50,
  "errors": 2
}
```

## Best Practices

1. **Use Webhooks** over Polling for real-time
2. **Implement Retry** for failed messages
3. **Rate Limit** to avoid throttling
4. **Validate Input** for security
5. **Log Errors** for debugging

## Migration from Official OpenClaw

Official channels can be adapted:

| Official | OCG |
|----------|-----|
| `discord` | `channels.NewDiscord()` (planned) |
| `slack` | `channels.NewSlack()` (planned) |
| `signal` | `channels.NewSignal()` (planned) |
