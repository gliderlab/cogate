# Pulse/Heartbeat System

The Pulse system provides event-driven automation with priority-based processing.

## Overview

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   User      │────▶│   Database   │────▶│   Pulse    │
│  (Events)   │     │   (Queue)    │     │  (Checker) │
└─────────────┘     └──────────────┘     └─────────────┘
                                               │
                    ┌──────────────────────────┼──────────────────────────┐
                    │                          │                          │
                    ▼                          ▼                          ▼
            ┌──────────────┐          ┌──────────────┐          ┌──────────────┐
            │  Priority 0 │          │  Priority 1  │          │  Priority 2  │
            │  (Critical)  │          │    (High)    │          │  (Normal)   │
            │ Broadcast All│          │   Broadcast  │          │  LLM Process│
            └──────────────┘          └──────────────┘          └──────────────┘
```

## Priority Levels

| Level | Name | Behavior | Interrupt Processing? |
|-------|------|----------|----------------------|
| 0 | Critical | Broadcast to ALL channels immediately | Yes |
| 1 | High | Broadcast to specified channel | No* |
| 2 | Normal | Process with LLM when idle | No |
| 3 | Low | Process when available | No |

*Only interrupted by Priority 0

## Configuration

### Default Config

```go
PulseConfig{
    Interval:     1 * time.Second,  // Check every second (60 times/minute)
    Enabled:      true,             // System enabled
    LLMEnabled:   true,            // Use LLM for processing
    MaxQueueSize: 100,             // Max events in queue
    CleanupHours: 24,              // Auto-cleanup after 24 hours
}
```

### Custom Config

```go
config := &agent.PulseConfig{
    Interval:     500 * time.Millisecond,  // Check twice per second
    Enabled:      true,
    LLMEnabled:   true,
    MaxQueueSize: 200,
    CleanupHours: 12,
}

ai := agent.New(agent.Config{
    // ... other config
    PulseEnabled: true,
    PulseConfig:  config,
})
```

## Usage

### Adding Events

#### Via Tool Call

```json
{
  "action": "add",
  "title": "Daily Standup",
  "content": "Remember to join the standup meeting at 10am",
  "priority": 2,
  "channel": ""
}
```

#### Via Code

```go
eventID, err := agent.AddPulseEvent(
    "Daily Standup",
    "Remember to join the standup meeting at 10am",
    2,  // Priority
    ""   // Channel (empty = all for priority 0)
)
```

### Priority Examples

#### Priority 0 - Critical (Emergency Broadcast)

```json
{
  "action": "add",
  "title": "🚨 System Alert",
  "content": "Server CPU at 95%!",
  "priority": 0,
  "channel": ""
}
```

**Behavior**: Immediately broadcasts to ALL configured channels (Telegram, Discord, Slack, etc.)

#### Priority 1 - High (Channel Broadcast)

```json
{
  "action": "add",
  "title": "📢 Announcement",
  "content": "New feature released!",
  "priority": 1,
  "channel": "telegram"
}
```

**Behavior**: Sends to specified channel only

#### Priority 2 - Normal (LLM Processing)

```json
{
  "action": "add",
  "title": "📝 Daily Summary",
  "content": "Summarize today's conversations",
  "priority": 2,
  "channel": ""
}
```

**Behavior**: 
1. If agent is idle → Process immediately with LLM
2. If agent is busy → Wait until idle
3. LLM analyzes the event and responds

#### Priority 3 - Low (Background Task)

```json
{
  "action": "add",
  "title": "🧹 Cleanup",
  "content": "Clean up old temporary files",
  "priority": 3,
  "channel": ""
}
```

**Behavior**: Process when agent is completely free

### Checking Status

```json
{
  "action": "status"
}
```

**Response**:
```json
{
  "enabled": true,
  "running": true,
  "is_processing": false,
  "current_event": null,
  "event_counts": {
    "pending": 2,
    "processing": 0,
    "completed": 15,
    "completed_with_errors": 1
  }
}
```

## Event Database Schema

```sql
CREATE TABLE events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    content TEXT,
    priority INTEGER DEFAULT 2,      -- 0-3
    status TEXT DEFAULT 'pending',    -- pending, processing, completed, dismissed
    channel TEXT,                     -- telegram, discord, etc. (empty = all)
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    processed_at DATETIME
);

CREATE INDEX idx_events_priority ON events(priority);
CREATE INDEX idx_events_status ON events(status);
```

## Processing Flow

```
1. Pulse Tick (every second)
   │
   ▼
2. Get next pending event (ORDER BY priority, created_at)
   │
   ▼
3. Check if should process:
   │
   ├── If priority == 0 (Critical) → ALWAYS process
   │
   ├── If priority == 1 (High) → Process if not processing critical
   │
   └── If priority >= 2 → Process ONLY if idle
   │
   ▼
4. Update status to 'processing'
   │
   ▼
5. Execute based on priority:
   │
   ├── Priority 0 → Broadcast to ALL channels
   │
   ├── Priority 1 → Broadcast to specified channel
   │
   └── Priority 2-3 → Process with LLM (if enabled)
   │
   ▼
6. Update status:
   │
   ├── Success → 'completed'
   │
   ├── Success with errors → 'completed_with_errors'
   │
   └── Failure → 'error'
   │
   ▼
7. Calculate next run (for recurring) or cleanup
```

## Use Cases

### 1. Emergency Notifications

```go
// System monitoring triggers critical event
agent.AddPulseEvent(
    "🔥 Server Down",
    "Production server at 10.0.0.1 is not responding",
    0,  // Critical - broadcast to all
    ""
)
```

### 2. Scheduled Reminders

```go
// Calendar integration
agent.AddPulseEvent(
    "📅 Meeting in 15 minutes",
    "Team standup starts in 15 minutes",
    1,  // High - notify on telegram
    "telegram"
)
```

### 3. AI Analysis

```go
// Background AI processing
agent.AddPulseEvent(
    "📊 Weekly Report",
    "Generate weekly analytics report",
    2,  // Normal - process when idle
    ""
)
```

### 4. Maintenance Tasks

```go
// Cleanup tasks
agent.AddPulseEvent(
    "🧹 Log Cleanup",
    "Compress and archive old logs",
    3,  // Low - process when available
    ""
)
```

## Interruption Behavior

```
Timeline:
──────────────────────────────────────────────────────────▶

Normal Task (Priority 2)
├──────────────────────────────────────────────────────────┤
                   ▲
                   │ Priority 0 event arrives
                   │
                   ▼
        ┌─────────────────────┐
        │ Interrupt!          │
        │ Process Priority 0  │
        └─────────────────────┘
                   │
                   ▼
        ┌─────────────────────┐
        │ Resume Priority 2   │
        │ (if still pending)   │
        └─────────────────────┘
```

## Integration with Other Systems

### Webhook Triggered Events

```python
# External system sends webhook to add event
import requests

requests.post('http://localhost:55003/pulse/add', 
    headers={'Authorization': 'Bearer TOKEN'},
    json={
        'action': 'add',
        'title': 'Webhook Triggered',
        'content': 'Data: {data}',
        'priority': 1
    })
```

### Scheduled Events (via Cron)

```json
{
  "name": "Morning Pulse Check",
  "schedule": {"kind": "cron", "expr": "0 8 * * *"},
  "payload": {
    "kind": "agentTurn",
    "message": "Check system status and add pulse event if issues found"
  }
}
```

## Best Practices

1. **Use Priority 0 sparingly** - Only for true emergencies
2. **Batch non-urgent events** - Combine multiple notifications
3. **Set appropriate channels** - Don't broadcast everything to all
4. **Monitor event queue** - Check status regularly
5. **Cleanup old events** - Set CleanupHours appropriately

## Troubleshooting

### Events not being processed

```bash
# Check pulse status
curl http://localhost:55003/pulse/status

# Check event queue
sqlite3 ocg.db "SELECT * FROM events WHERE status='pending';"
```

### Too many events

```bash
# Increase cleanup frequency
# Or manually clean old events
sqlite3 ocg.db "DELETE FROM events WHERE status='completed' AND processed_at < datetime('now', '-1 day');"
```

### LLM not processing

- Check `LLMEnabled` config
- Verify LLM API key is set
- Check agent logs for errors
