// WebSocket handler for real-time chat

package gateway

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gliderlab/cogate/rpcproto"
	"nhooyr.io/websocket"
)

// WebSocket message types
const (
	MsgTypeChat    = "chat"
	MsgTypeChunk   = "chunk"
	MsgTypeDone    = "done"
	MsgTypeError   = "error"
	MsgTypePing    = "ping"
	MsgTypePong    = "pong"
	MsgTypeHistory = "history"
)

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content,omitempty"`
}

// WSChatRequest represents a chat request via WebSocket
type WSChatRequest struct {
	Model    string             `json:"model"`
	Messages []rpcproto.Message `json:"messages"`
}

// WSChatResponse represents a chat response via WebSocket
type WSChatResponse struct {
	Content   string `json:"content"`
	Finish    bool   `json:"finish"`
	Error     string `json:"error,omitempty"`
	TotalTokens int   `json:"totalTokens,omitempty"`
}

// WebSocketHub manages WebSocket connections
type WebSocketHub struct {
	gateway   *Gateway
	clients   map[*websocket.Conn]bool
	broadcast chan []byte
	register  chan *websocket.Conn
	unregister chan *websocket.Conn
	mu        sync.RWMutex
}

func NewWebSocketHub(g *Gateway) *WebSocketHub {
	return &WebSocketHub{
		gateway:    g,
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *WebSocketHub) Run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.clients[conn] = true
			h.mu.Unlock()
			log.Printf("[WS] Client connected, total: %d", len(h.clients))

		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close(websocket.StatusNormalClosure, "")
			}
			h.mu.Unlock()
			log.Printf("[WS] Client disconnected, total: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for conn := range h.clients {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				conn.Write(ctx, websocket.MessageText, message)
				cancel()
			}
			h.mu.RUnlock()
		}
	}
}

// HandleWebSocket handles WebSocket upgrade requests
func (g *Gateway) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check authentication
	token := strings.TrimSpace(g.cfg.UIAuthToken)
	if token == "" {
		http.Error(w, "unauthorized (ui token not set)", http.StatusUnauthorized)
		return
	}

	// Check multiple sources for token
	authValid := false

	// 1. Authorization header
	header := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		header = strings.TrimSpace(header[len("Bearer "):])
	}
	if header == token {
		authValid = true
	}

	// 2. X-OCG-UI-Token header
	if !authValid {
		alt := r.Header.Get("X-OCG-UI-Token")
		if alt == token {
			authValid = true
		}
	}

	// 3. Query string token (for WebSocket)
	if !authValid {
		queryToken := r.URL.Query().Get("token")
		if queryToken == token {
			authValid = true
		}
	}

	if !authValid {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		log.Printf("[WS] Accept error: %v", err)
		return
	}

	// Create context with timeout for ping/pong handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle the connection
	g.handleWSConnection(ctx, conn)
}

func (g *Gateway) handleWSConnection(ctx context.Context, conn *websocket.Conn) {
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Message loop
	for {
		_, msgBytes, err := conn.Read(ctx)
		if err != nil {
			log.Printf("[WS] Read error: %v", err)
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			g.sendWSError(conn, "invalid message format")
			continue
		}

		switch msg.Type {
		case MsgTypeChat:
			g.handleWSChat(ctx, conn, msg.Content)
		case MsgTypePing:
			// Respond with pong
			pong := WSMessage{Type: MsgTypePong}
			if data, err := json.Marshal(pong); err == nil {
				conn.Write(context.Background(), websocket.MessageText, data)
			}
		default:
			log.Printf("[WS] Unknown message type: %s", msg.Type)
		}
	}
}

func (g *Gateway) handleWSChat(ctx context.Context, conn *websocket.Conn, content json.RawMessage) {
	// Content can be either a JSON object or a stringified JSON object
	// Try to parse as WSChatRequest first
	var req WSChatRequest
	if err := json.Unmarshal(content, &req); err != nil {
		// Try parsing as string (front-end sends stringified JSON)
		var contentStr string
		if err := json.Unmarshal(content, &contentStr); err != nil {
			g.sendWSError(conn, "invalid request: "+err.Error())
			return
		}
		// Parse the stringified JSON
		if err := json.Unmarshal([]byte(contentStr), &req); err != nil {
			g.sendWSError(conn, "invalid request content: "+err.Error())
			return
		}
	}

	client, err := g.clientOrError()
	if err != nil {
		g.sendWSError(conn, "agent not connected")
		return
	}

	// Log the incoming message
	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		log.Printf("[WS] Received message: role=%s len=%d", last.Role, len(last.Content))
	}

	// Send request to agent via RPC
	var reply rpcproto.ChatReply
	args := rpcproto.ChatArgs{Messages: req.Messages}
	if err := client.Call("Agent.Chat", args, &reply); err != nil {
		g.sendWSError(conn, "chat error: "+err.Error())
		return
	}

	// Send the response as a single message (since agent doesn't stream)
	resp := WSChatResponse{
		Content:     reply.Content,
		Finish:      true,
		TotalTokens: countTokens([]byte(reply.Content)),
	}

	msg := WSMessage{
		Type:    MsgTypeDone,
		Content: json.RawMessage{},
	}

	// Marshal with the response data
	respBytes, _ := json.Marshal(resp)
	msg.Content = respBytes

	data, err := json.Marshal(msg)
	if err != nil {
		g.sendWSError(conn, "marshal error: "+err.Error())
		return
	}

	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		log.Printf("[WS] Write error: %v", err)
	}
}

func (g *Gateway) sendWSError(conn *websocket.Conn, errMsg string) {
	resp := WSChatResponse{
		Error:  errMsg,
		Finish: true,
	}
	msg := WSMessage{
		Type:    MsgTypeError,
		Content: json.RawMessage{},
	}
	respBytes, _ := json.Marshal(resp)
	msg.Content = respBytes
	data, _ := json.Marshal(msg)
	conn.Write(context.Background(), websocket.MessageText, data)
}
