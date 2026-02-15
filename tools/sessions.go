// Sessions tools - session management
package tools

import (
	"fmt"
	"time"
)

type SessionsListTool struct{}

func NewSessionsListTool() *SessionsListTool {
	return &SessionsListTool{}
}

func (t *SessionsListTool) Name() string {
	return "sessions_list"
}

func (t *SessionsListTool) Description() string {
	return "List active sessions; supports optional kind filters."
}

func (t *SessionsListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"kinds": map[string]interface{}{
				"type":        "array",
				"description": "Optional filter by kinds (agent/main/isolated)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max results",
				"default":     20,
			},
			"activeMinutes": map[string]interface{}{
				"type":        "integer",
				"description": "Only sessions active within N minutes",
			},
		},
	}
}

func (t *SessionsListTool) Execute(args map[string]interface{}) (interface{}, error) {
	// Simplified mock; real impl should query gateway
	return map[string]interface{}{
		"sessions": []map[string]interface{}{
			{
				"key":     "main",
				"kind":    "main",
				"active":  true,
				"created": time.Now().Add(-time.Hour).Format(time.RFC3339),
			},
		},
		"count": 1,
	}, nil
}

// Sessions Send Tool - send message to another session
type SessionsSendTool struct{}

func NewSessionsSendTool() *SessionsSendTool {
	return &SessionsSendTool{}
}

func (t *SessionsSendTool) Name() string {
	return "sessions_send"
}

func (t *SessionsSendTool) Description() string {
	return "Send a message to a given session."
}

func (t *SessionsSendTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"sessionKey": map[string]interface{}{
				"type":        "string",
				"description": "Target session key",
			},
			"label": map[string]interface{}{
				"type":        "string",
				"description": "Optional session label",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Message content",
			},
		},
		"required": []string{"message"},
	}
}

func (t *SessionsSendTool) Execute(args map[string]interface{}) (interface{}, error) {
	sessionKey := GetString(args, "sessionKey")
	label := GetString(args, "label")
	message := GetString(args, "message")

	if message == "" {
		return nil, fmt.Errorf("message is required")
	}

	if sessionKey == "" && label == "" {
		return nil, fmt.Errorf("sessionKey or label is required")
	}

	// Mock response; real impl should call gateway
	return map[string]interface{}{
		"action":  "sent",
		"target":  sessionKey,
		"message": "Message sent (mock)",
	}, nil
}

// Sessions Spawn Tool - start a sub-session
type SessionsSpawnTool struct{}

func NewSessionsSpawnTool() *SessionsSpawnTool {
	return &SessionsSpawnTool{}
}

func (t *SessionsSpawnTool) Name() string {
	return "sessions_spawn"
}

func (t *SessionsSpawnTool) Description() string {
	return "Start a sub-agent task in an isolated session."
}

func (t *SessionsSpawnTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "Task description",
			},
			"label": map[string]interface{}{
				"type":        "string",
				"description": "Optional task label",
			},
			"agentId": map[string]interface{}{
				"type":        "string",
				"description": "Optional agent ID",
			},
			"model": map[string]interface{}{
				"type":        "string",
				"description": "Optional model override",
			},
			"timeoutSeconds": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout seconds",
				"default":     300,
			},
		},
		"required": []string{"task"},
	}
}

func (t *SessionsSpawnTool) Execute(args map[string]interface{}) (interface{}, error) {
	task := GetString(args, "task")
	label := GetString(args, "label")
	agentId := GetString(args, "agentId")
	model := GetString(args, "model")
	timeout := GetInt(args, "timeoutSeconds")

	if task == "" {
		return nil, fmt.Errorf("task is required")
	}

	// Mock response; real impl should call gateway
	return map[string]interface{}{
		"action":         "spawned",
		"task":           task,
		"label":          label,
		"agentId":        agentId,
		"model":          model,
		"timeoutSeconds": timeout,
		"message":        "Task started (mock)",
	}, nil
}

// Sessions History Tool - fetch history
type SessionsHistoryTool struct{}

func NewSessionsHistoryTool() *SessionsHistoryTool {
	return &SessionsHistoryTool{}
}

func (t *SessionsHistoryTool) Name() string {
	return "sessions_history"
}

func (t *SessionsHistoryTool) Description() string {
	return "Get message history of a session."
}

func (t *SessionsHistoryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"sessionKey": map[string]interface{}{
				"type":        "string",
				"description": "Session key",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max messages",
				"default":     50,
			},
			"includeTools": map[string]interface{}{
				"type":        "boolean",
				"description": "Include tool calls",
				"default":     false,
			},
		},
		"required": []string{"sessionKey"},
	}
}

func (t *SessionsHistoryTool) Execute(args map[string]interface{}) (interface{}, error) {
	sessionKey := GetString(args, "sessionKey")
	limit := GetInt(args, "limit")
	_ = limit // reserved parameter
	includeTools := GetBool(args, "includeTools")

	if sessionKey == "" {
		return nil, fmt.Errorf("sessionKey is required")
	}

	// Mock empty history
	return map[string]interface{}{
		"sessionKey":   sessionKey,
		"messages":     []map[string]interface{}{},
		"count":        0,
		"includeTools": includeTools,
	}, nil
}

// Session Status Tool
type SessionStatusTool struct{}

func NewSessionStatusTool() *SessionStatusTool {
	return &SessionStatusTool{}
}

func (t *SessionStatusTool) Name() string {
	return "session_status"
}

func (t *SessionStatusTool) Description() string {
	return "Get current session status (usage, time, etc.)."
}

func (t *SessionStatusTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"sessionKey": map[string]interface{}{
				"type":        "string",
				"description": "Optional session key",
			},
			"model": map[string]interface{}{
				"type":        "string",
				"description": "Optional model override",
			},
		},
	}
}

func (t *SessionStatusTool) Execute(args map[string]interface{}) (interface{}, error) {
	sessionKey := GetString(args, "sessionKey")
	model := GetString(args, "model")

	return map[string]interface{}{
		"sessionKey": sessionKey,
		"model":      model,
		"status": map[string]interface{}{
			"running": true,
			"since":   time.Now().Add(-time.Hour).Format(time.RFC3339),
		},
		"usage": map[string]interface{}{
			"inputTokens":  0,
			"outputTokens": 0,
		},
	}, nil
}

// Agents List Tool
 type AgentsListTool struct{}

func NewAgentsListTool() *AgentsListTool {
	return &AgentsListTool{}
}

func (t *AgentsListTool) Name() string {
	return "agents_list"
}

func (t *AgentsListTool) Description() string {
	return "List agent IDs available for sessions_spawn."
}

func (t *AgentsListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{}
}

func (t *AgentsListTool) Execute(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"agents": []string{"main"},
		"count":  1,
	}, nil
}
