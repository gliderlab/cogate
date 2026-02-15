# Telegram Bot Integration Guide

## Overview

OpenClaw-Go (OCG) supports Telegram Bot integration via the `ocg-gateway` service. The bot connects to your Telegram account and routes messages through the AI agent for processing.

## Architecture

```
Telegram User → Telegram API → OCG Gateway (/telegram/webhook) → Agent (RPC) → AI Model → Response
```

## Configuration

### 1. Set Telegram Bot Token

You need to obtain a Telegram Bot Token from @BotFather:

1. Start a chat with @BotFather on Telegram
2. Send `/newbot` to create a new bot
3. Follow the instructions to get your token

### 2. Configure Environment Variable

Set the `TELEGRAM_BOT_TOKEN` environment variable before starting the gateway:

```bash
export TELEGRAM_BOT_TOKEN="your_bot_token_here"
./bin/ocg-gateway
```

Or add it to `env.config`:

```bash
echo "TELEGRAM_BOT_TOKEN=your_bot_token_here" >> /opt/openclaw-go/env.config
```

## API Endpoints

### Webhook Endpoint

```
POST /telegram/webhook
```

This endpoint receives incoming updates from Telegram. It should be exposed publicly and registered with Telegram's API.

### Set Webhook

```
POST /telegram/setWebhook
Authorization: Bearer <ui_token>

{
  "token": "your_bot_token",
  "webhookUrl": "https://your-domain.com/telegram/webhook"
}
```

### Status Check

```
GET /telegram/status
Authorization: Bearer <ui_token>
```

Response:
```json
{
  "initialized": true,
  "token_configured": true,
  "token_set": true
}
```

## Usage

### With ngrok (for testing)

1. Start the gateway:
```bash
export TELEGRAM_BOT_TOKEN="your_token"
./bin/ocg-gateway
```

2. In another terminal, start ngrok:
```bash
ngrok http 55003
```

3. Set the webhook via API:
```bash
curl -X POST "http://localhost:55003/telegram/setWebhook" \
  -H "Authorization: Bearer your_ui_token" \
  -H "Content-Type: application/json" \
  -d '{"webhookUrl":"https://your-ngrok-url.ngrok.io/telegram/webhook"}'
```

4. Send a message to your bot on Telegram!

### Commands

The bot responds to the following commands:

- `/start` - Start the bot and get a greeting
- `/help` - Show help message
- `/stats` - Show bot statistics
- Any other message - Processed by the AI agent

## Production Deployment

For production, you'll need:

1. A publicly accessible domain/URL
2. SSL certificate (Telegram requires HTTPS)
3. Configure your reverse proxy (nginx, Traefik, etc.) to forward `/telegram/webhook` to the gateway

Example nginx configuration:

```nginx
location /telegram/webhook {
    proxy_pass http://127.0.0.1:55003/telegram/webhook;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
```

## Troubleshooting

### Bot not initialized

Check that `TELEGRAM_BOT_TOKEN` is set:
```bash
echo $TELEGRAM_BOT_TOKEN
```

### Webhook not setting

Verify your webhook URL is publicly accessible:
```bash
curl -I https://your-domain.com/telegram/webhook
```

### Messages not received

Check the gateway logs for errors:
```bash
tail -f /var/log/openclaw-gateway.log
```

## Security Considerations

1. Keep your bot token secure
2. The webhook endpoint should be protected behind your reverse proxy
3. Consider adding IP whitelist for Telegram's webhook IPs
4. Use HTTPS in production
