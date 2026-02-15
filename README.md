# OpenClaw-Go (OCG) Documentation

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/SQLite-3.x-003B57?style=for-the-badge&logo=sqlite" alt="SQLite">
</p>

OpenClaw-Go (OCG) is a lightweight, high-performance Go implementation of OpenClaw, designed for local deployment with minimal resource usage.

## Features

- 🚀 **Fast Startup** - <1 second vs 5-10s for Node.js
- 💾 **Low Memory** - 50-100MB vs 200-500MB+
- 🔒 **Privacy** - 100% local, zero data transfer
- 💰 **Cost** - Built-in llama.cpp, zero API costs

## Table of Contents

1. [Quick Start](#quick-start)
2. [Architecture](#architecture)
3. [Configuration](#configuration)
4. [Tools](#tools)
5. [WebSocket API](#websocket-api)
6. [Pulse/Heartbeat System](#pulseheartbeat-system)
7. [Cron Jobs](#cron-jobs)
8. [Session Management](#session-management)
9. [Channel Adapters](#channel-adapters)

---

## Quick Start

### Build

```bash
cd /opt/openclaw-go
make build-all
```

### Run

```bash
# Start all services (ocg exits after everything is ready)
export OPENCLAW_UI_TOKEN=your_token
./bin/ocg start
```

### Access

- **Web UI**: http://localhost:55003
- **API**: http://localhost:55003/v1/chat/completions
- **WebSocket**: ws://localhost:55003/ws/chat

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Gateway (HTTP Server)                    │
│                    Port: 55003 / 18789                      │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐   │
│  │   Web UI    │  │  WebSocket  │  │  Channel Adapter │   │
│  └─────────────┘  └─────────────┘  └─────────────────┘   │
├─────────────────────────────────────────────────────────────┤
│                      RPC (Unix Socket)                       │
│                     /tmp/ocg-agent.sock                     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      Agent (LLM Engine)                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐   │
│  │   Sessions  │  │   Memory    │  │     Tools       │   │
│  └─────────────┘  └─────────────┘  └─────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  Embedding Service (HTTP)                   │
│                       Port: 50001                          │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              llama.cpp (Embedding Model)            │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENCLAW_UI_TOKEN` | - | UI authentication token |
| `OPENCLAW_API_KEY` | - | LLM API key |
| `OPENCLAW_BASE_URL` | - | LLM API base URL |
| `OPENCLAW_MODEL` | - | Model name |
| `OPENCLAW_FORCE_ENV_CONFIG` | false | Force env.config to override DB config |
| `OPENCLAW_AGENT_SOCK` | /tmp/ocg-agent.sock | Unix socket path |
| `EMBEDDING_SERVER_URL` | http://localhost:50001 | Embedding service |
| `HNSW_PATH` | vector.index | Vector index file |

### env.config

```bash
# LLM Configuration
OPENCLAW_API_KEY=sk-xxx
OPENCLAW_BASE_URL=https://api.openai.com/v1
OPENCLAW_MODEL=gpt-4
OPENCLAW_FORCE_ENV_CONFIG=true

# Gateway Configuration  
OPENCLAW_UI_TOKEN=your_secure_token
OPENCLAW_PORT=55003

# Storage
OPENCLAW_DB_PATH=ocg.db

# Embedding
EMBEDDING_SERVER_URL=http://localhost:50001
EMBEDDING_MODEL_PATH=/path/to/model.gguf
```

---

## Deployment

A one-shot `deploy.sh` script is included for Debian/Ubuntu hosts. It installs build dependencies, updates the repo, syncs llama.cpp, and builds binaries.

```bash
# As root (or sudo -E)
./deploy.sh
```

### Deploy Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LLAMA_JOBS` | 1 | Parallel build jobs for llama.cpp |
| `LLAMA_STATIC` | OFF | Build llama.cpp static binaries |
| `BUILD_TYPE` | Release | CMake build type |
| `USE_SWAP` | on | Auto-create swap if none exists |
| `SWAP_SIZE` | 4G | Swap size |
| `OCG_REF` | main | Git ref/branch for this repo |
| `LLAMA_REF` | master | Git ref/branch for llama.cpp |

---

## Tools

### Implemented Tools

| Tool | Status | Description |
|------|--------|-------------|
| `exec` | ✅ Complete | Execute shell commands |
| `read` | ✅ Complete | Read files (50KB limit) |
| `write` | ✅ Complete | Write files |
| `edit` | ✅ Complete | Edit files |
| `process` | ✅ Complete | Process management |
| `memory` | ✅ Complete | Vector memory search/store |
| `web` | ✅ Basic | Web search/fetch |
| `pulse` | ✅ Complete | Event heartbeat system |
| `browser` | ⚠️ Basic | Browser control |
| `sessions` | ⚠️ Basic | Session management |

### Using Tools

```bash
# Via HTTP API
curl -X POST http://localhost:55003/v1/chat/completions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"read /etc/hostname"}]}'

# Via WebSocket
ws://localhost:55003/ws/chat?token=$TOKEN
```

---

## WebSocket API

### Connection

```javascript
const ws = new WebSocket('ws://localhost:55003/ws/chat?token=YOUR_TOKEN');
```

### Send Message

```javascript
ws.send(JSON.stringify({
  type: 'chat',
  content: JSON.stringify({
    model: 'default',
    messages: [{role: 'user', content: 'Hello!'}]
  })
}));
```

### Receive Response

```javascript
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === 'done') {
    const data = JSON.parse(msg.content);
    console.log(data.content); // AI response
  }
};
```

### Fallback to HTTP

The UI automatically falls back to HTTP if WebSocket is unavailable:

```javascript
if (!ws || ws.readyState !== WebSocket.OPEN) {
  // Use HTTP instead
  await fetch('/v1/chat/completions', {...});
}
```

---

## Pulse/Heartbeat System

The pulse system provides event-driven automation with priority levels.

### Priority Levels

| Level | Name | Behavior |
|-------|------|----------|
| 0 | Critical | Broadcast to all channels immediately |
| 1 | High | Broadcast to specified channel |
| 2 | Normal | Process when idle |
| 3 | Low | Process when available |

### Adding Events

```python
# Via pulse tool
{
  "action": "add",
  "title": "Important Reminder",
  "content": "Check the reports",
  "priority": 1,
  "channel": "telegram"
}
```

### Status Check

```python
{
  "action": "status"
}
```

### Configuration

```go
PulseConfig{
    Interval:     1 * time.Second,  // Check interval
    Enabled:      true,              // Enable system
    LLMEnabled:   true,              // Use LLM for processing
    MaxQueueSize: 100,              // Event queue size
    CleanupHours: 24,                // Auto-cleanup after 24h
}
```

---

## Cron Jobs

Schedule tasks with precise timing.

### Job Types

- **at**: One-shot at specific time
- **every**: Fixed interval
- **cron**: Cron expression

### Session Targets

- **main**: Run in main session (system event)
- **isolated**: Run in separate session

### Example: Daily Briefing

```json
{
  "name": "Morning briefing",
  "schedule": {
    "kind": "cron",
    "expr": "0 7 * * *",
    "tz": "Asia/Shanghai"
  },
  "sessionTarget": "isolated",
  "payload": {
    "kind": "agentTurn",
    "message": "Generate today's briefing"
  },
  "delivery": {
    "mode": "announce",
    "channel": "telegram",
    "to": "USER_ID"
  }
}
```

### Wake Modes

- **now**: Execute immediately
- **next-heartbeat**: Wait for next heartbeat

---

## Session Management

### Session Keys

- `main` - Main conversation
- `telegram:USER_ID` - Telegram user session
- `discord:USER_ID` - Discord user session
- `cron:JOB_ID` - Cron job session

### Creating Sessions

```go
// Create session
session, _ := sm.CreateSession("session-key", "agent-1")

// Create channel session
session, _ := sm.GetOrCreateChannelSession("telegram", "123456", "agent-1")

// Add message
sm.AddMessage("telegram:123456", Message{
    Role:    "user",
    Content: "Hello",
})
```

### Listing Sessions

```go
infos := sm.ListSessionInfos()
for _, info := range infos {
    fmt.Printf("Session: %s, Messages: %d\n", info.Key, info.MessageCount)
}
```

---

## Channel Adapters

### Architecture

```
Gateway
    │
    ▼
ChannelAdapter
    │
    ├── Telegram (✅ Implemented)
    ├── WhatsApp (🔶 Planned)
    ├── Discord  (🔶 Planned)
    └── Slack    (🔶 Planned)
```

### Telegram Bot Setup

```bash
# Set bot token
export TELEGRAM_BOT_TOKEN=your_bot_token

# Start all services
./bin/ocg start
```

### Bot Commands

- `/start` - Start bot
- `/help` - Help
- `/stats` - Stats
- `/reset` - Reset greeting status

### Proactive Greeting

The bot sends greeting to new users automatically:

```go
bot.SetGreeting(true, "Hello! I'm OpenClaw-Go 🤖")
```

---

## Database Schema

### Tables

```sql
-- Messages
CREATE TABLE messages (
    id INTEGER PRIMARY KEY,
    session_key TEXT,
    role TEXT,
    content TEXT,
    created_at DATETIME
);

-- Memories (Vector)
CREATE TABLE memories (
    id INTEGER PRIMARY KEY,
    key TEXT UNIQUE,
    value TEXT,
    category TEXT,
    importance REAL,
    created_at DATETIME
);

-- Session Meta
CREATE TABLE session_meta (
    session_key TEXT PRIMARY KEY,
    total_tokens INTEGER,
    compaction_count INTEGER,
    last_summary TEXT,
    updated_at DATETIME
);

-- Events (Pulse)
CREATE TABLE events (
    id INTEGER PRIMARY KEY,
    title TEXT,
    content TEXT,
    priority INTEGER,
    status TEXT,
    channel TEXT,
    created_at DATETIME,
    processed_at DATETIME
);

-- Config
CREATE TABLE config (
    section TEXT,
    key TEXT,
    value TEXT,
    PRIMARY KEY (section, key)
);
```

---

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/v1/chat/completions` | POST | Chat API |
| `/ws/chat` | WS | WebSocket chat |
| `/storage/stats` | GET | Storage stats |
| `/memory/search` | GET | Search memory |
| `/memory/store` | POST | Store memory |
| `/process/start` | POST | Start process |
| `/telegram/webhook` | POST | Telegram webhook |

---

## Troubleshooting

### Service Not Starting

```bash
# Check logs
tail -f /tmp/ocg/logs/gateway.log

# Verify ports
ss -ltn | grep -E "55003|50001|18000"
```

### Database Issues

```bash
# Backup database
cp ocg.db ocg.db.backup

# Reinitialize
rm ocg.db
./bin/ocg start
```

### Memory Search Not Working

```bash
# Check embedding service
curl http://localhost:50001/health

# Rebuild vector index
# (Coming soon)
```

---

## Performance

| Metric | OCG | Official OpenClaw |
|--------|-----|-------------------|
| Startup | <1s | 5-10s |
| Memory | 50-100MB | 200-500MB |
| Requests/s | ~100 | ~50 |

---

## License

MIT License

---

## Contributing

Contributions welcome! Please see GitHub for details.
