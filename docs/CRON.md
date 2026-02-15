# Cron Jobs System

The Cron system provides precise scheduling for background tasks.

## Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Cron Scheduler                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐   │
│  │ Job Queue│──▶│ Scheduler│──▶│ Executor │──▶│ Delivery │   │
│  └──────────┘   └──────────┘   └──────────┘   └──────────┘   │
│       │                                            │           │
│       ▼                                            ▼           │
│  ┌──────────┐                              ┌──────────────┐   │
│  │  Storage │                              │  Channels    │   │
│  │ (JSON)   │                              │ Telegram/    │   │
│  └──────────┘                              │ Discord/etc  │   │
│                                             └──────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Schedule Types

### 1. At (One-shot)

Run once at specific time.

```json
{
  "schedule": {
    "kind": "at",
    "at": "2026-02-15T10:00:00Z"
  }
}
```

**Use Cases**:
- One-time reminders
- Scheduled reports
- Delayed tasks

### 2. Every (Interval)

Run at fixed intervals.

```json
{
  "schedule": {
    "kind": "every",
    "everyMs": 3600000  // 1 hour in milliseconds
  }
}
```

**Use Cases**:
- Periodic health checks
- Regular backups
- Status polling

### 3. Cron (Expression)

Run on cron schedule.

```json
{
  "schedule": {
    "kind": "cron",
    "expr": "0 9 * * *",        // Every day at 9am
    "tz": "Asia/Shanghai"        // Timezone
  }
}
```

**Format**: `minute hour day month weekday`

| Field     | Values          | Special Characters |
|-----------|-----------------|-------------------|
| minute    | 0-59            | * , - /           |
| hour      | 0-23            | * , - /           |
| day       | 1-31            | * , - / ?         |
| month     | 1-12 or JAN-DEC| * , - /           |
| weekday   | 0-6 or SUN-SAT | * , - / ?         |

**Examples**:

```bash
# Every minute
* * * * *

# Every hour
0 * * * *

# Every day at 9am
0 9 * * *

# Every Monday at 9am
0 9 * * 1

# Every 15 minutes
*/15 * * * *

# Every day at 9:30am
30 9 * * *

# First day of every month at midnight
0 0 1 * *
```

## Session Targets

### Main Session

Runs in the main conversation session via system event.

```json
{
  "sessionTarget": "main",
  "payload": {
    "kind": "systemEvent",
    "text": "Morning check triggered"
  },
  "wakeMode": "now"
}
```

**Behavior**:
- Adds system event to main session
- Executes on next heartbeat
- Has full conversation context
- No separate output delivery

### Isolated Session

Runs in a separate cron session.

```json
{
  "sessionTarget": "isolated",
  "payload": {
    "kind": "agentTurn",
    "message": "Generate daily report"
  },
  "wakeMode": "now"
}
```

**Behavior**:
- Creates new session: `cron:JOB_ID`
- No conversation context (fresh start)
- Can use different model
- Supports delivery

## Wake Modes

### Now

Execute immediately when scheduled time arrives.

```json
{
  "wakeMode": "now"
}
```

**Use Cases**: Time-critical tasks, reminders

### Next Heartbeat

Wait for next heartbeat to execute.

```json
{
  "wakeMode": "next-heartbeat"
}
```

**Use Cases**: Batch processing, reduce API calls

## Delivery Modes

### Announce

Deliver output to specified channel.

```json
{
  "delivery": {
    "mode": "announce",
    "channel": "telegram",
    "to": "USER_ID",
    "bestEffort": true
  }
}
```

**Behavior**:
- Sends result directly to channel
- Posts summary to main session
- Respects `bestEffort` setting

### None

No delivery, internal only.

```json
{
  "delivery": {
    "mode": "none"
  }
}
```

**Use Cases**: Silent monitoring, cleanup tasks

## Complete Job Example

### Morning Briefing (Daily at 7am)

```json
{
  "name": "Morning Briefing",
  "schedule": {
    "kind": "cron",
    "expr": "0 7 * * *",
    "tz": "Asia/Shanghai"
  },
  "sessionTarget": "isolated",
  "wakeMode": "now",
  "payload": {
    "kind": "agentTurn",
    "message": "Generate morning briefing: weather, calendar, top emails, news",
    "model": "gpt-4",
    "thinking": "medium"
  },
  "delivery": {
    "mode": "announce",
    "channel": "telegram",
    "to": "5408141074"
  }
}
```

### One-shot Reminder

```json
{
  "name": "Meeting Reminder",
  "schedule": {
    "kind": "at",
    "at": "2026-02-15T14:00:00Z"
  },
  "sessionTarget": "main",
  "wakeMode": "now",
  "payload": {
    "kind": "systemEvent",
    "text": "Reminder: Team meeting starts in 15 minutes"
  },
  "deleteAfterRun": true
}
```

### Periodic Health Check

```json
{
  "name": "System Health",
  "schedule": {
    "kind": "every",
    "everyMs": 300000  // 5 minutes
  },
  "sessionTarget": "isolated",
  "payload": {
    "kind": "agentTurn",
    "message": "Check system health: CPU, memory, disk, services"
  },
  "delivery": {
    "mode": "announce",
    "channel": "telegram",
    "to": "ADMIN_ID"
  }
}
```

## Job State

Each job tracks:

```json
{
  "state": {
    "nextRunAtMs": 1708000000000,    // Next scheduled run
    "lastRunAtMs": 1707999900000,    // Last run timestamp
    "lastStatus": "ok",               // ok, error, skipped
    "lastDurationMs": 5000,           // Last run duration
    "consecutiveErrors": 0            // Error count
  }
}
```

## Storage

Jobs stored in: `~/.openclaw/cron/jobs.json`

```json
[
  {
    "id": "job-1708000000000",
    "name": "Morning Briefing",
    "enabled": true,
    "schedule": {
      "kind": "cron",
      "expr": "0 7 * * *"
    },
    "sessionTarget": "isolated",
    "payload": {...},
    "delivery": {...},
    "state": {...}
  }
]
```

## CLI Equivalents

### Add Job

```bash
# One-shot reminder
openclaw cron add \
  --name "Reminder" \
  --at "2026-02-15T14:00:00Z" \
  --session main \
  --system-event "Meeting in 15 min" \
  --wake now

# Daily briefing
openclaw cron add \
  --name "Morning Brief" \
  --cron "0 7 * * *" \
  --tz "Asia/Shanghai" \
  --session isolated \
  --message "Generate briefing" \
  --announce \
  --channel telegram \
  --to "USER_ID"
```

### List Jobs

```bash
openclaw cron list
```

### Run Job Now

```bash
openclaw cron run JOB_ID
```

### Delete Job

```bash
openclaw cron remove JOB_ID
```

### Check History

```bash
openclaw cron runs --id JOB_ID --limit 10
```

## Best Practices

1. **Use appropriate session target**
   - Main: Quick notifications, context-aware
   - Isolated: Heavy tasks, reports

2. **Set correct wake mode**
   - Now: Time-critical
   - Next-heartbeat: Batch with other tasks

3. **Configure delivery properly**
   - Announce: User-facing output
   - None: Internal tasks

4. **Monitor job health**
   - Check consecutive errors
   - Review run history

5. **Clean up one-shot jobs**
   - Set `deleteAfterRun: true` for reminders

## Troubleshooting

### Job not running

```bash
# Check if cron is enabled
curl http://localhost:55003/cron/status

# Check job schedule
curl http://localhost:55003/cron/list

# Check database
sqlite3 ~/.openclaw/cron/jobs.json
```

### Wrong delivery target

- Use explicit format: `channel:ID`
- For Telegram topics: `-1001234567890:topic:123`

### Timezone issues

- Always specify `tz` for cron jobs
- Use IANA timezone names: `America/New_York`, `Asia/Shanghai`
