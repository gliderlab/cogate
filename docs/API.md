# OpenClaw-Go API Reference

## Table of Contents

1. [Authentication](#authentication)
2. [Chat API](#chat-api)
3. [Memory API](#memory-api)
4. [Process API](#process-api)
5. [WebSocket API](#websocket-api)
6. [Telegram Bot API](#telegram-bot-api)
7. [Pulse/Events API](#pulseevents-api)
8. [Cron API](#cron-api)

---

## Authentication

All API endpoints (except `/telegram/webhook`) require authentication.

### Methods

**Bearer Token (Recommended)**:
```bash
curl -H "Authorization: Bearer YOUR_TOKEN" ...
```

**X-OCG-UI-Token Header**:
```bash
curl -H "X-OCG-UI-Token: YOUR_TOKEN" ...
```

**WebSocket Query String**:
```javascript
new WebSocket('ws://host/ws/chat?token=YOUR_TOKEN')
```

---

## Chat API

### POST /v1/chat/completions

OpenAI-compatible chat completion endpoint.

**Request**:
```bash
curl -X POST http://localhost:55003/v1/chat/completions \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "system", "content": "You are helpful."},
      {"role": "user", "content": "Hello!"}
    ],
    "temperature": 0.7,
    "max_tokens": 1000
  }'
```

**Response**:
```json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1699000000,
  "model": "gpt-4",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I help you?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 20,
    "completion_tokens": 15,
    "total_tokens": 35
  }
}
```

### GET /health

Health check endpoint.

**Request**:
```bash
curl http://localhost:55003/health \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response**:
```json
{"status": "ok"}
```

---

## Memory API

### GET /memory/search

Search vector memory.

**Request**:
```bash
curl "http://localhost:55003/memory/search?query=hello&limit=5&minScore=0.3" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response**:
```json
{
  "items": [
    {
      "id": "mem-123",
      "text": "User said hello",
      "score": 0.95,
      "category": "conversation"
    }
  ]
}
```

### POST /memory/store

Store new memory.

**Request**:
```bash
curl -X POST http://localhost:55003/memory/store \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Important note",
    "category": "preference",
    "importance": 0.8
  }'
```

**Response**:
```json
{
  "id": "mem-456",
  "success": true
}
```

### GET /memory/get

Get memory by path.

**Request**:
```bash
curl "http://localhost:55003/memory/get?path=memory/notes.md" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

---

## Process API

### POST /process/start

Start a new process.

**Request**:
```bash
curl -X POST http://localhost:55003/process/start \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "command": "ls -la",
    "workdir": "/tmp",
    "env": "PATH=/usr/bin",
    "pty": false
  }'
```

**Response**:
```json
{
  "sessionId": "proc-123",
  "pid": 12345
}
```

### GET /process/list

List running processes.

**Request**:
```bash
curl http://localhost:55003/process/list \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### POST /process/write

Write to process stdin.

**Request**:
```bash
curl -X POST http://localhost:55003/process/write \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "proc-123",
    "data": "input text\n"
  }'
```

### POST /process/kill

Kill a process.

**Request**:
```bash
curl -X POST "http://localhost:55003/process/kill?sessionId=proc-123" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### GET /process/log

Get process output.

**Request**:
```bash
curl "http://localhost:55003/process/log?sessionId=proc-123&offset=0&limit=1000" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

---

## WebSocket API

### Connection

```
ws://localhost:55003/ws/chat?token=YOUR_TOKEN
```

### Message Format

**Send**:
```javascript
{
  "type": "chat",
  "content": "{\"model\":\"default\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello\"}]}"
}
```

**Receive**:
```javascript
{
  "type": "done",
  "content": "{\"content\":\"Hello! How can I help?\",\"finish\":true,\"totalTokens\":10}"
}
```

### Error Response

```javascript
{
  "type": "error",
  "content": "{\"error\":\"invalid request\",\"finish\":true}"
}
```

### Ping/Pong

```javascript
// Send
{"type": "ping"}

// Receive
{"type": "pong"}
```

---

## Telegram Bot API

### Webhook

**POST /telegram/webhook**

Receives Telegram updates.

```json
{
  "update_id": 123456789,
  "message": {
    "message_id": 1,
    "from": {"id": 123456789, "first_name": "John"},
    "chat": {"id": 123456789, "type": "private"},
    "text": "Hello bot"
  }
}
```

### Set Webhook

**POST /telegram/setWebhook**

```bash
curl -X POST http://localhost:55003/telegram/setWebhook \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://your-domain.com/telegram/webhook"}'
```

### Bot Status

**GET /telegram/status**

```bash
curl http://localhost:55003/telegram/status \
  -H "Authorization: Bearer YOUR_TOKEN"
```

---

## Pulse/Events API

### Add Event

```bash
curl -X POST http://localhost:55003/pulse/add \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "action": "add",
    "title": "Important Event",
    "content": "Event details",
    "priority": 1,
    "channel": "telegram"
  }'
```

**Priority Levels**:
- 0 = Critical (broadcast all)
- 1 = High (broadcast channel)
- 2 = Normal (process when idle)
- 3 = Low (process when available)

### Get Status

```bash
curl http://localhost:55003/pulse/status \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response**:
```json
{
  "enabled": true,
  "running": true,
  "is_processing": false,
  "event_counts": {
    "pending": 2,
    "completed": 10
  }
}
```

---

## Cron API

### List Jobs

**GET /cron/list**

```bash
curl http://localhost:55003/cron/list \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Add Job

**POST /cron/add**

```bash
curl -X POST http://localhost:55003/cron/add \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Daily Report",
    "schedule": {
      "kind": "cron",
      "expr": "0 9 * * *",
      "tz": "Asia/Shanghai"
    },
    "sessionTarget": "isolated",
    "payload": {
      "kind": "agentTurn",
      "message": "Generate daily report"
    },
    "delivery": {
      "mode": "announce",
      "channel": "telegram",
      "to": "USER_ID"
    }
  }'
```

### Update Job

**POST /cron/update**

```bash
curl -X POST http://localhost:55003/cron/update \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jobId": "job-123",
    "patch": {"enabled": false}
  }'
```

### Status

**GET /cron/status**

```bash
curl http://localhost:55003/cron/status \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Run Job Now

**POST /cron/run**

```bash
curl -X POST http://localhost:55003/cron/run \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"jobId": "job-123"}'
```

### Delete Job

**POST /cron/remove**

```bash
curl -X POST http://localhost:55003/cron/remove \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"jobId": "job-123"}'
```

---

## Error Responses

### 400 Bad Request
```json
{"error": "invalid request format"}
```

### 401 Unauthorized
```json
{"error": "unauthorized"}
```

### 404 Not Found
```json
{"error": "endpoint not found"}
```

### 500 Internal Server Error
```json
{"error": "internal server error"}
```

### 503 Service Unavailable
```json
{"error": "agent not connected"}
```
