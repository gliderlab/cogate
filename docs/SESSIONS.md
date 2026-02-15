# Session Management

OpenClaw-Go supports multiple concurrent sessions with isolated contexts.

## Concepts

### Session

A session represents an independent conversation context with:
- Unique key (`sessionKey`)
- Message history
- Token count
- Metadata

### Session Key Format

| Format | Example | Use Case |
|--------|---------|----------|
| `main` | `main` | Primary conversation |
| `telegram:USER_ID` | `telegram:5408141074` | Telegram user |
| `discord:USER_ID` | `discord:123456789` | Discord user |
| `cron:JOB_ID` | `cron:job-1708000000` | Cron job |
| `whatsapp:PHONE` | `whatsapp:+1234567890` | WhatsApp user |

## Session Manager

### Creating a Session Manager

```go
store, _ := storage.New("ocg.db")
sm := agent.NewSessionManager(store, "default-agent")
```

### Creating Sessions

```go
// Create new session
session, err := sm.CreateSession("session-key", "agent-1")

// Get or create
session, err := sm.GetOrCreateSession("session-key", "agent-1")

// Create for channel
session, err := sm.GetOrCreateChannelSession("telegram", "5408141074", "agent-1")
```

### Managing Messages

```go
// Add message
err := sm.AddMessage("main", agent.Message{
    Role:    "user",
    Content: "Hello!",
})

// Get messages
messages, err := sm.GetMessages("main")

// Clear session
err := sm.ClearSession("main")
```

### Listing Sessions

```go
// Get all session infos
infos := sm.ListSessionInfos()

for _, info := range infos {
    fmt.Printf("Key: %s, Messages: %d, Tokens: %d\n", 
        info.Key, info.MessageCount, info.TotalTokens)
}
```

## Session Structure

```go
type Session struct {
    ID              string
    Key             string           // Unique identifier
    AgentID         string           // Associated agent
    Messages        []Message        // Conversation history
    CreatedAt       time.Time
    UpdatedAt       time.Time
    TotalTokens     int
    CompactionCount int
    ContextTokens   int
    IsActive        bool
    Metadata        map[string]interface{}
}
```

## SessionInfo (Lightweight)

```go
type SessionInfo struct {
    Key             string    `json:"key"`
    AgentID         string    `json:"agentId"`
    MessageCount    int       `json:"messageCount"`
    TotalTokens     int       `json:"totalTokens"`
    CreatedAt       time.Time `json:"createdAt"`
    UpdatedAt       time.Time `json:"updatedAt"`
    IsActive        bool      `json:"isActive"`
}
```

## Use Cases

### 1. Multi-User Support

```go
func handleTelegramMessage(userID int64, text string) {
    sessionKey := fmt.Sprintf("telegram:%d", userID)
    
    session, _ := sm.GetOrCreateSession(sessionKey, "default")
    
    // Add user message
    sm.AddMessage(sessionKey, agent.Message{
        Role:    "user",
        Content: text,
    })
    
    // Process with LLM...
    
    // Add assistant response
    sm.AddMessage(sessionKey, agent.Message{
        Role:    "assistant", 
        Content: response,
    })
}
```

### 2. Isolated Cron Jobs

```go
// Each cron job gets its own session
jobID := "job-123"
sessionKey := fmt.Sprintf("cron:%s", jobID)

session, _ := sm.GetOrCreateSession(sessionKey, "worker-agent")

// Clear previous context for fresh start
sm.ClearSession(sessionKey)
```

### 3. Context Isolation

```go
// Main conversation
mainSession, _ := sm.GetOrCreateSession("main", "assistant")

// Debug session (isolated)
debugSession, _ := sm.GetOrCreateSession("debug", "assistant")

// These don't interfere with each other
```

## Session Persistence

### Database Storage

Sessions are stored in `session_meta` table:

```sql
CREATE TABLE session_meta (
    session_key TEXT PRIMARY KEY,
    total_tokens INTEGER DEFAULT 0,
    compaction_count INTEGER DEFAULT 0,
    last_summary TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Loading Sessions

```go
// Load sessions from database on startup
err := sm.LoadSessions()
```

### Saving Sessions

```go
// Sessions auto-save periodically
// Or manually:
sm.saveSession(session)
```

## Context Management

### Token Limits

```go
const (
    MaxContextTokens = 128000  // GPT-4 context
    WarningThreshold = 100000  // Warn before limit
)
```

### Compaction

When context approaches limit:

```go
func (s *Session) Compact() {
    // Summarize old messages
    summary := summarize(s.Messages[:len(s.Messages)/2])
    
    // Keep recent messages
    s.Messages = append([]Message{{
        Role:    "system",
        Content: "Summary: " + summary,
    }}, s.Messages[len(s.Messages)/2:]...)
    
    s.CompactionCount++
}
```

## Session Routing

### Message Flow

```
Incoming Message
       │
       ▼
┌──────────────────┐
│  Determine Key  │
│  (channel+id)    │
└──────────────────┘
       │
       ▼
┌──────────────────┐
│ Get/Create       │
│ Session          │
└──────────────────┘
       │
       ▼
┌──────────────────┐
│ Add to History  │
│ Process Message │
└──────────────────┘
       │
       ▼
┌──────────────────┐
│ Save to DB      │
│ Return Response  │
└──────────────────┘
```

## Best Practices

1. **Use descriptive keys**: `telegram:123` not `s1`
2. **Clear old sessions**: Don't accumulate indefinitely
3. **Monitor token usage**: Check `TotalTokens` regularly
4. **Separate concerns**: Use isolated sessions for cron jobs

## API Endpoints

### List Sessions

```bash
GET /sessions/list
```

Response:
```json
{
  "sessions": [
    {
      "key": "main",
      "agentId": "default",
      "messageCount": 50,
      "totalTokens": 15000,
      "createdAt": "2026-02-15T10:00:00Z",
      "updatedAt": "2026-02-15T12:00:00Z",
      "isActive": true
    }
  ]
}
```

### Get Session Messages

```bash
GET /sessions/messages?key=main
```

### Clear Session

```bash
POST /sessions/clear
Content-Type: application/json

{"key": "main"}
```

## Troubleshooting

### Session not found

```bash
# List all sessions
curl http://localhost:55003/sessions/list
```

### Token limit exceeded

```bash
# Check session tokens
curl "http://localhost:55003/sessions/messages?key=main" | jq '.messageCount'

# Clear old messages
curl -X POST http://localhost:55003/sessions/clear \
  -H "Content-Type: application/json" \
  -d '{"key": "main"}'
```

### Memory issues

```go
// Set max messages per session
sm.MaxMessages = 1000

// Enable auto-compaction
sm.AutoCompact = true
sm.CompactThreshold = 80000
```
