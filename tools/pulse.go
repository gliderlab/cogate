// Pulse tool - allows users to add events to the heartbeat system

package tools

import (
	"fmt"
	"strings"
)

// PulseAgentInterface defines the interface for pulse functionality
type PulseAgentInterface interface {
	AddPulseEvent(title string, content string, priority int, channel string) (int64, error)
	GetPulseStatus() (map[string]interface{}, error)
}

// PulseTool allows adding events to the pulse system
type PulseTool struct {
	agent PulseAgentInterface
}

// NewPulseTool creates a new pulse tool
func NewPulseTool(a PulseAgentInterface) *PulseTool {
	return &PulseTool{agent: a}
}

func (t *PulseTool) Name() string {
	return "pulse"
}

func (t *PulseTool) Description() string {
	return `Add events to the heartbeat system. Priority: 0=critical (broadcast all), 1=high (broadcast channel), 2=normal, 3=low. Use "status" action to check pulse system status.`
}

func (t *PulseTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action to perform: add, status",
				"enum":        []string{"add", "status"},
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Event title (for add action)",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Event content/description (for add action)",
			},
			"priority": map[string]interface{}{
				"type":        "integer",
				"description": "Priority level: 0=critical, 1=high, 2=normal, 3=low",
				"minimum":     0,
				"maximum":     3,
			},
			"channel": map[string]interface{}{
				"type":        "string",
				"description": "Target channel for broadcast (telegram, discord, etc.)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *PulseTool) Execute(args map[string]interface{}) (interface{}, error) {
	if t.agent == nil {
		return nil, fmt.Errorf("agent not initialized")
	}

	action, _ := args["action"].(string)
	action = strings.ToLower(strings.TrimSpace(action))

	switch action {
	case "add":
		return t.executeAdd(args)
	case "status":
		return t.executeStatus()
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (t *PulseTool) executeAdd(args map[string]interface{}) (interface{}, error) {
	title, _ := args["title"].(string)
	content, _ := args["content"].(string)
	priority, _ := args["priority"].(int)
	channel, _ := args["channel"].(string)

	// Default values
	if title == "" {
		title = "User Event"
	}
	if content == "" {
		content = "No description provided"
	}
	if priority < 0 || priority > 3 {
		priority = 2 // Default to normal
	}

	eventID, err := t.agent.AddPulseEvent(title, content, priority, channel)
	if err != nil {
		return nil, err
	}

	priorityLabels := []string{"Critical", "High", "Normal", "Low"}
	return map[string]interface{}{
		"success":       true,
		"event_id":      eventID,
		"title":         title,
		"priority":      priority,
		"priority_label": priorityLabels[priority],
		"channel":       channel,
		"message":       fmt.Sprintf("Event added with priority %s (%d)", priorityLabels[priority], priority),
	}, nil
}

func (t *PulseTool) executeStatus() (interface{}, error) {
	status, err := t.agent.GetPulseStatus()
	if err != nil {
		return nil, err
	}

	enabled := true
	if v, ok := status["enabled"]; ok {
		enabled = v.(bool)
	}

	if !enabled {
		return map[string]interface{}{
			"enabled": false,
			"message": "Pulse/Heartbeat system is not enabled",
		}, nil
	}

	running := false
	isProcessing := false
	if v, ok := status["running"].(bool); ok {
		running = v
	}
	if v, ok := status["is_processing"].(bool); ok {
		isProcessing = v
	}

	eventCounts := map[string]int{}
	if v, ok := status["event_counts"].(map[string]int); ok {
		eventCounts = v
	}

	return map[string]interface{}{
		"enabled":       true,
		"running":       running,
		"is_processing": isProcessing,
		"event_counts":  eventCounts,
	}, nil
}
