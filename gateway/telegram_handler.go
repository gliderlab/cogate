package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gliderlab/cogate/gateway/channels"
)

// handleTelegramWebhook handles incoming Telegram bot webhook requests
func (g *Gateway) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	if g.channelAdapter == nil || !g.channelAdapter.HasChannel(channels.ChannelTelegram) {
		http.Error(w, "Telegram Channel not initialized", http.StatusServiceUnavailable)
		return
	}
	
	g.channelAdapter.HandleWebhook(channels.ChannelTelegram, w, r)
}

// handleTelegramSetWebhook configures the Telegram bot webhook URL
func (g *Gateway) handleTelegramSetWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get webhook URL from request
	body, _ := io.ReadAll(r.Body)
	var req struct {
		WebhookURL string `json:"webhookUrl"`
	}
	json.Unmarshal(body, &req)

	if req.WebhookURL == "" {
		http.Error(w, "webhookUrl is required", http.StatusBadRequest)
		return
	}

	// Check if Telegram channel exists
	if g.channelAdapter == nil || !g.channelAdapter.HasChannel(channels.ChannelTelegram) {
		// Create Telegram bot if token is provided
		telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN")
		if telegramToken == "" {
			http.Error(w, "TELEGRAM_BOT_TOKEN not configured", http.StatusBadRequest)
			return
		}

		bot := channels.NewTelegramBot(telegramToken, &GatewayAgentRPC{client: g.client})
		if err := g.channelAdapter.RegisterChannel(bot); err != nil {
			http.Error(w, fmt.Sprintf("Failed to register Telegram channel: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Get the Telegram bot and set webhook
	botInfo, err := g.channelAdapter.GetChannelInfo(channels.ChannelTelegram)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get Telegram channel info: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Telegram webhook configured: %s", req.WebhookURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "Webhook configured successfully",
		"url":     req.WebhookURL,
		"channel": botInfo.Name,
		"type":    botInfo.Type,
	})
}

// handleTelegramStatus returns the current Telegram channel status
func (g *Gateway) handleTelegramStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"enabled":   false,
		"registered": false,
		"token_set": os.Getenv("TELEGRAM_BOT_TOKEN") != "",
	}

	if g.channelAdapter != nil && g.channelAdapter.HasChannel(channels.ChannelTelegram) {
		status["enabled"] = true
		status["registered"] = true
		if info, err := g.channelAdapter.GetChannelInfo(channels.ChannelTelegram); err == nil {
			status["name"] = info.Name
			status["version"] = info.Version
			status["capabilities"] = info.Capabilities
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
