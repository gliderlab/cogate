// Pulse/Heartbeat system for OpenClaw-Go
// Runs every second (60 times per minute) to check for important events

package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gliderlab/cogate/storage"
)

// PulseConfig holds configuration for the heartbeat system
type PulseConfig struct {
	Interval       time.Duration // Check interval (default 1 second)
	Enabled        bool          // Enable/disable pulse
	LLMEnabled     bool          // Enable LLM processing
	MaxQueueSize   int           // Maximum events in queue
	CleanupHours   int           // Hours after which to clear old events
}

// DefaultPulseConfig returns default configuration
func DefaultPulseConfig() *PulseConfig {
	return &PulseConfig{
		Interval:     1 * time.Second,
		Enabled:      true,
		LLMEnabled:   true,
		MaxQueueSize: 100,
		CleanupHours: 24,
	}
}

// PulseEvent represents an event to be processed by the heartbeat system
type PulseEvent struct {
	Event    *storage.Event
	Response string
	Errors   []string
}

// PulseHandler handles the heartbeat/pulse system
type PulseHandler struct {
	storage    *storage.Storage
	config     *PulseConfig
	mu         sync.RWMutex
	running    bool
	stopCh     chan struct{}
	eventCh    chan *PulseEvent
	// Processing state
	isProcessing bool
	currentEvent *storage.Event
	// Callbacks
	onEvent      func(*PulseEvent)
	onBroadcast  func(string, int) error // (message, priority)
	onLLMProcess func(string) (string, error)
}

// NewPulseHandler creates a new pulse handler
func NewPulseHandler(storage *storage.Storage, config *PulseConfig) *PulseHandler {
	if config == nil {
		config = DefaultPulseConfig()
	}
	return &PulseHandler{
		storage: storage,
		config:  config,
		stopCh:  make(chan struct{}),
		eventCh: make(chan *PulseEvent, config.MaxQueueSize),
	}
}

// SetBroadcastCallback sets the callback for broadcasting messages
func (p *PulseHandler) SetBroadcastCallback(cb func(string, int) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onBroadcast = cb
}

// SetLLMCallback sets the callback for LLM processing
func (p *PulseHandler) SetLLMCallback(cb func(string) (string, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onLLMProcess = cb
}

// SetEventCallback sets the callback for event processing
func (p *PulseHandler) SetEventCallback(cb func(*PulseEvent)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onEvent = cb
}

// Start starts the heartbeat system
func (p *PulseHandler) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	log.Printf("[Pulse] Starting heartbeat system (interval: %v)", p.config.Interval)

	// Start the heartbeat loop
	go p.heartbeatLoop()
}

// Stop stops the heartbeat system
func (p *PulseHandler) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	close(p.stopCh)
	p.mu.Unlock()

	log.Printf("[Pulse] Stopped heartbeat system")
}

// IsRunning returns whether the pulse is running
func (p *PulseHandler) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// IsProcessing returns whether currently processing an event
func (p *PulseHandler) IsProcessing() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.isProcessing
}

// heartbeatLoop runs the main heartbeat loop
func (p *PulseHandler) heartbeatLoop() {
	ticker := time.NewTicker(p.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.tick()
		}
	}
}

// tick performs one heartbeat check
func (p *PulseHandler) tick() {
	// Check for pending events
	event, err := p.storage.GetNextEvent()
	if err != nil {
		log.Printf("[Pulse] Error getting next event: %v", err)
		return
	}

	if event == nil {
		// No pending events, do cleanup periodically
		if time.Now().Second() == 0 { // Every minute
			p.storage.ClearOldEvents(p.config.CleanupHours)
		}
		return
	}

	// Check if we should process this event
	if !p.shouldProcessEvent(event) {
		return
	}

	// Process the event
	p.processEvent(event)
}

// shouldProcessEvent determines if an event should be processed now
func (p *PulseHandler) shouldProcessEvent(event *storage.Event) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// If already processing, only critical events can interrupt
	if p.isProcessing && event.Priority > storage.PriorityCritical {
		return false
	}

	// Priority 0 (Critical) - always process immediately
	// Priority 1 (High) - process when not processing critical
	// Priority 2 (Normal) - process when idle
	// Priority 3 (Low) - process when explicitly idle

	switch event.Priority {
	case storage.PriorityCritical:
		return true
	case storage.PriorityHigh:
		return !p.isProcessing || p.currentEvent == nil
	case storage.PriorityNormal:
		return !p.isProcessing
	case storage.PriorityLow:
		return !p.isProcessing
	}

	return false
}

// processEvent handles processing a single event
func (p *PulseHandler) processEvent(event *storage.Event) {
	p.mu.Lock()
	p.isProcessing = true
	p.currentEvent = event
	p.mu.Unlock()

	log.Printf("[Pulse] Processing event: id=%d, priority=%d, title=%s", 
		event.ID, event.Priority, event.Title)

	// Mark as processing
	p.storage.UpdateEventStatus(event.ID, "processing")

	var response string
	var errors []string

	// Handle based on priority
	switch event.Priority {
	case storage.PriorityCritical:
		// Broadcast to all channels immediately
		msg := fmt.Sprintf("🔴 CRITICAL: %s\n\n%s", event.Title, event.Content)
		p.mu.RLock()
		if p.onBroadcast != nil {
			if err := p.onBroadcast(msg, 0); err != nil {
				errors = append(errors, err.Error())
			}
		}
		p.mu.RUnlock()
		response = "Broadcasted to all channels"

	case storage.PriorityHigh:
		// Broadcast to specified channel(s)
		msg := fmt.Sprintf("⚠️ %s\n\n%s", event.Title, event.Content)
		p.mu.RLock()
		channel := event.Channel
		if p.onBroadcast != nil {
			// If channel specified, use it; otherwise broadcast to all
			if channel != "" {
				if err := p.onBroadcast(msg, 1); err != nil {
					errors = append(errors, err.Error())
				}
			} else {
				if err := p.onBroadcast(msg, 1); err != nil {
					errors = append(errors, err.Error())
				}
			}
		}
		p.mu.RUnlock()
		response = "Broadcasted to channel"

	case storage.PriorityNormal, storage.PriorityLow:
		// Process with LLM if enabled
		if p.config.LLMEnabled {
			p.mu.RLock()
			llmCb := p.onLLMProcess
			p.mu.RUnlock()

			if llmCb != nil {
				input := fmt.Sprintf("Event: %s\n\nDescription: %s\n\nPlease analyze and respond:", 
					event.Title, event.Content)
				resp, err := llmCb(input)
				if err != nil {
					errors = append(errors, err.Error())
				} else {
					response = resp
				}
			}
		}
	}

	// Create pulse event result
	pulseEvent := &PulseEvent{
		Event:    event,
		Response: response,
		Errors:   errors,
	}

	// Trigger callback
	p.mu.RLock()
	eventCb := p.onEvent
	p.mu.RUnlock()
	if eventCb != nil {
		eventCb(pulseEvent)
	}

	// Update status
	if len(errors) > 0 {
		p.storage.UpdateEventStatus(event.ID, "completed_with_errors")
	} else {
		p.storage.UpdateEventStatus(event.ID, "completed")
	}

	// Reset processing state
	p.mu.Lock()
	p.isProcessing = false
	p.currentEvent = nil
	p.mu.Unlock()
}

// AddEvent adds a new event to the pulse system
func (p *PulseHandler) AddEvent(title, content string, priority int, channel string) (int64, error) {
	if priority < 0 || priority > 3 {
		priority = 2 // Default to normal
	}
	return p.storage.AddEvent(title, content, storage.EventPriority(priority), channel)
}

// GetStatus returns the current status of the pulse system
func (p *PulseHandler) GetStatus() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	counts, _ := p.storage.GetEventCount()

	return map[string]interface{}{
		"running":        p.running,
		"is_processing": p.isProcessing,
		"current_event":  p.currentEvent,
		"event_counts":   counts,
		"config":         p.config,
	}
}

// ParsePriority parses a priority string to EventPriority
func ParsePriority(s string) storage.EventPriority {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "0", "critical", "crit", "c":
		return storage.PriorityCritical
	case "1", "high", "important", "h":
		return storage.PriorityHigh
	case "2", "normal", "n":
		return storage.PriorityNormal
	case "3", "low", "l":
		return storage.PriorityLow
	default:
		return storage.PriorityNormal
	}
}

// EventToJSON converts an event to JSON
func EventToJSON(event *storage.Event) string {
	data, _ := json.Marshal(event)
	return string(data)
}
