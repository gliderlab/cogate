// Edit Tool - precise in-file replacements
package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type EditTool struct{}

func (t *EditTool) Name() string {
	return "edit"
}

func (t *EditTool) Description() string {
	return "Precisely replace a text snippet. oldText must match exactly and appear only once."
}

func (t *EditTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path",
			},
			"oldText": map[string]interface{}{
				"type":        "string",
				"description": "Exact text to replace (must match once)",
			},
			"newText": map[string]interface{}{
				"type":        "string",
				"description": "Replacement text",
			},
		},
		"required": []string{"path", "oldText", "newText"},
	}
}

func (t *EditTool) Execute(args map[string]interface{}) (interface{}, error) {
	path := GetString(args, "path")
	oldText := GetString(args, "oldText")
	newText := GetString(args, "newText")

	if path == "" {
		return nil, &EditError{Message: "path is required"}
	}
	if oldText == "" {
		return nil, &EditError{Message: "oldText is required"}
	}
	// allow empty newText to support deletion

	// Validate path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, &EditError{Message: "invalid path: " + err.Error()}
	}

	// Ensure file exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &EditError{Message: "file not found: " + absPath}
		}
		return nil, &EditError{Message: "cannot access file: " + err.Error()}
	}

	if info.IsDir() {
		return nil, &EditError{Message: "path is a directory"}
	}

	// Read file
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, &EditError{Message: "read failed: " + err.Error()}
	}

	original := string(content)

	// Count occurrences
	count := strings.Count(original, oldText)
	switch count {
	case 0:
		return nil, &EditError{Message: "oldText not found"}
	case 1:
		modified := strings.Replace(original, oldText, newText, 1)
		if err := os.WriteFile(absPath, []byte(modified), 0644); err != nil {
			return nil, &EditError{Message: "write failed: " + err.Error()}
		}
		return EditResult{
			Path:      absPath,
			Changed:   true,
			MatchInfo: fmt.Sprintf("replaced 1 occurrence"),
		}, nil
	default:
		return nil, &EditError{Message: fmt.Sprintf("oldText appears %d times; specify more precisely", count)}
	}
}

type EditResult struct {
	Path      string `json:"path"`
	Changed   bool   `json:"changed"`
	MatchInfo string `json:"match_info"`
}

type EditError struct {
	Message string
}

func (e *EditError) Error() string {
	return e.Message
}
