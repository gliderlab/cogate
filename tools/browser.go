// Browser, Canvas, and Nodes tools (mock implementations)
package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BrowserTool struct {
	browserDir string
}

func NewBrowserTool() *BrowserTool {
	return &BrowserTool{
		browserDir: "/tmp/openclaw-browser",
	}
}

func (t *BrowserTool) Name() string {
	return "browser"
}

func (t *BrowserTool) Description() string {
	return "Control the browser for automation: screenshot, snapshot, navigation, click, input (mock)."
}

func (t *BrowserTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: status/start/stop/open/navigate/snapshot/screenshot/act",
			},
			"targetUrl": map[string]interface{}{
				"type":        "string",
				"description": "URL to open or navigate",
			},
			"targetId": map[string]interface{}{
				"type":        "string",
				"description": "Element ID (act)",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS selector",
			},
			"text": map[string]interface{}{
				"type":        "string",
				"description": "Text to input (act/type)",
			},
			"timeoutMs": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in ms",
				"default":     30000,
			},
		},
		"required": []string{"action"},
	}
}

func (t *BrowserTool) Execute(args map[string]interface{}) (interface{}, error) {
	action := GetString(args, "action")

	if action == "" {
		return nil, fmt.Errorf("action parameter is required")
	}

	// Ensure browser dir exists
	os.MkdirAll(t.browserDir, 0755)

	switch action {
	case "status":
		return t.status()
	case "start":
		return t.start()
	case "stop":
		return t.stop()
	case "open":
		url := GetString(args, "targetUrl")
		if url == "" {
			return nil, fmt.Errorf("open requires targetUrl")
		}
		return t.open(url)
	case "navigate":
		url := GetString(args, "targetUrl")
		if url == "" {
			return nil, fmt.Errorf("navigate requires targetUrl")
		}
		return t.navigate(url)
	case "snapshot":
		return t.snapshot()
	case "screenshot":
		return t.screenshot()
	case "act":
		return t.act(args)
	default:
		return nil, fmt.Errorf("unknown browser action: %s", action)
	}
}

func (t *BrowserTool) status() (interface{}, error) {
	return map[string]interface{}{
		"status":  "available",
		"browser": "openclaw-chromium",
		"actions": []string{"status", "start", "stop", "open", "navigate", "snapshot", "screenshot", "act"},
	}, nil
}

func (t *BrowserTool) start() (interface{}, error) {
	return map[string]interface{}{
		"action":  "started",
		"message": "browser started (mock)",
	}, nil
}

func (t *BrowserTool) stop() (interface{}, error) {
	return map[string]interface{}{
		"action":  "stopped",
		"message": "browser stopped",
	}, nil
}

func (t *BrowserTool) open(url string) (interface{}, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	return map[string]interface{}{
		"action":  "opened",
		"url":     url,
		"message": fmt.Sprintf("opened in browser: %s", url),
	}, nil
}

func (t *BrowserTool) navigate(url string) (interface{}, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	return map[string]interface{}{
		"action":  "navigated",
		"url":     url,
		"message": fmt.Sprintf("navigated to: %s", url),
	}, nil
}

func (t *BrowserTool) snapshot() (interface{}, error) {
	return map[string]interface{}{
		"snapshot": "<html><body><h1>Browser Snapshot</h1><p>No page loaded</p></body></html>",
		"elements": []map[string]string{},
	}, nil
}

func (t *BrowserTool) screenshot() (interface{}, error) {
	screenshotPath := filepath.Join(t.browserDir, fmt.Sprintf("screenshot_%d.png", time.Now().Unix()))

	// Placeholder file
	if err := os.WriteFile(screenshotPath, []byte("# Placeholder for screenshot"), 0644); err != nil {
		return nil, fmt.Errorf("screenshot failed: %v", err)
	}

	return map[string]interface{}{
		"action":     "screenshot",
		"screenshot": screenshotPath,
		"message":    "screenshot saved (placeholder)",
	}, nil
}

func (t *BrowserTool) act(args map[string]interface{}) (interface{}, error) {
	targetId := GetString(args, "targetId")
	selector := GetString(args, "selector")
	text := GetString(args, "text")

action := "click"
	if text != "" {
		action = "type"
	}

	return map[string]interface{}{
		"action":   action,
		"targetId": targetId,
		"selector": selector,
		"text":     text,
		"message":  fmt.Sprintf("performed %s action", action),
	}, nil
}

// Canvas Tool (mock)
type CanvasTool struct{}

func NewCanvasTool() *CanvasTool {
	return &CanvasTool{}
}

func (t *CanvasTool) Name() string {
	return "canvas"
}

func (t *CanvasTool) Description() string {
	return "Control a paired node's Canvas for display, screenshots, navigation (mock)."
}

func (t *CanvasTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: present/hide/navigate/snapshot/eval",
			},
			"node": map[string]interface{}{
				"type":        "string",
				"description": "Node ID or name",
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to display",
			},
			"javaScript": map[string]interface{}{
				"type":        "string",
				"description": "JavaScript to execute",
			},
		},
		"required": []string{"action"},
	}
}

func (t *CanvasTool) Execute(args map[string]interface{}) (interface{}, error) {
	action := GetString(args, "action")

	if action == "" {
		return nil, fmt.Errorf("action parameter is required")
	}

	switch action {
	case "present":
		url := GetString(args, "url")
		return map[string]interface{}{
			"action":  "present",
			"url":     url,
			"message": fmt.Sprintf("showing: %s", url),
		}, nil
	case "hide":
		return map[string]interface{}{
			"action":  "hide",
			"message": "Canvas content hidden",
		}, nil
	case "navigate":
		url := GetString(args, "url")
		return map[string]interface{}{
			"action":  "navigate",
			"url":     url,
			"message": fmt.Sprintf("navigated to: %s", url),
		}, nil
	case "snapshot":
		return map[string]interface{}{
			"action":   "snapshot",
			"image":    "",
			"elements": []map[string]string{},
		}, nil
	case "eval":
		js := GetString(args, "javaScript")
		return map[string]interface{}{
			"action": "eval",
			"result": fmt.Sprintf("executed JavaScript: %s", js),
		}, nil
	default:
		return nil, fmt.Errorf("unknown canvas action: %s", action)
	}
}

// Nodes Tool (mock)
type NodesTool struct{}

func NewNodesTool() *NodesTool {
	return &NodesTool{}
}

func (t *NodesTool) Name() string {
	return "nodes"
}

func (t *NodesTool) Description() string {
	return "Discover and control paired nodes: status, location, camera, run commands (mock)."
}

func (t *NodesTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: status/describe/pending/camera_snap/camera_list/location_get/run",
			},
			"node": map[string]interface{}{
				"type":        "string",
				"description": "Node ID or name",
			},
			"facing": map[string]interface{}{
				"type":        "string",
				"description": "Camera facing: front/back",
			},
			"command": map[string]interface{}{
				"type":        "array",
				"description": "Command list to run",
			},
		},
		"required": []string{"action"},
	}
}

func (t *NodesTool) Execute(args map[string]interface{}) (interface{}, error) {
	action := GetString(args, "action")

	if action == "" {
		return nil, fmt.Errorf("action parameter is required")
	}

	switch action {
	case "status":
		return t.nodeStatus()
	case "describe":
		node := GetString(args, "node")
		return t.nodeDescribe(node)
	case "pending":
		return t.pendingNodes()
	case "camera_list":
		return t.cameraList()
	case "camera_snap":
		facing := GetString(args, "facing")
		if facing == "" {
			facing = "back"
		}
		return t.cameraSnap(facing)
	case "location_get":
		return t.locationGet()
	case "run":
		cmd := getStringSlice(args, "command")
		node := GetString(args, "node")
		return t.runCommand(node, cmd)
	default:
		return nil, fmt.Errorf("unknown nodes action: %s", action)
	}
}

func (t *NodesTool) nodeStatus() (interface{}, error) {
	return map[string]interface{}{
		"status":  "no_nodes",
		"nodes":   []string{},
		"message": "no paired node",
	}, nil
}

func (t *NodesTool) nodeDescribe(node string) (interface{}, error) {
	if node == "" {
		return nil, fmt.Errorf("node parameter is required")
	}
	return map[string]interface{}{
		"node":    node,
		"status":  "unknown",
		"message": fmt.Sprintf("node %s status unknown", node),
	}, nil
}

func (t *NodesTool) pendingNodes() (interface{}, error) {
	return map[string]interface{}{
		"pending": []string{},
	}, nil
}

func (t *NodesTool) cameraList() (interface{}, error) {
	return map[string]interface{}{
		"cameras": []string{},
	}, nil
}

func (t *NodesTool) cameraSnap(facing string) (interface{}, error) {
	return map[string]interface{}{
		"action":  "camera_snap",
		"facing":  facing,
		"message": "no paired device",
	}, nil
}

func (t *NodesTool) locationGet() (interface{}, error) {
	return map[string]interface{}{
		"location": map[string]interface{}{
			"latitude":  0,
			"longitude": 0,
			"accuracy":  0,
		},
	}, nil
}

func (t *NodesTool) runCommand(node string, command []string) (interface{}, error) {
	if node == "" {
		return nil, fmt.Errorf("node parameter is required")
	}
	return map[string]interface{}{
		"node":    node,
		"command": command,
		"output":  "no paired device",
	}, nil
}

func getStringSlice(args map[string]interface{}, key string) []string {
	if v, ok := args[key]; ok {
		if arr, ok := v.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}
