// Write Tool - create or overwrite files
package tools

import (
	"os"
	"path/filepath"
)

type WriteTool struct{}

func (t *WriteTool) Name() string {
	return "write"
}

func (t *WriteTool) Description() string {
	return "Create a new file or overwrite an existing file. Parent dirs auto-created."
}

func (t *WriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path (new or overwrite)",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "File content",
			},
			"append": map[string]interface{}{
				"type":        "boolean",
				"description": "Append instead of overwrite (default overwrite)",
				"default":     false,
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteTool) Execute(args map[string]interface{}) (interface{}, error) {
	path := GetString(args, "path")
	content := GetString(args, "content")
	appendMode := GetBool(args, "append")

	if path == "" {
		return nil, &WriteError{Message: "path is required"}
	}
	// Empty content allowed (file truncate)

	// Validate path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, &WriteError{Message: "invalid path: " + err.Error()}
	}

	// Reject directory
	info, err := os.Stat(absPath)
	if err == nil && info.IsDir() {
		return nil, &WriteError{Message: "path is a directory; cannot overwrite with a file"}
	}

	// Create parent dirs
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, &WriteError{Message: "cannot create directory: " + err.Error()}
	}

	// Write file
	var f *os.File
	if appendMode {
		f, err = os.OpenFile(absPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	} else {
		f, err = os.Create(absPath)
	}
	if err != nil {
		return nil, &WriteError{Message: "cannot create file: " + err.Error()}
	}
	defer f.Close()

	n, err := f.WriteString(content)
	if err != nil {
		return nil, &WriteError{Message: "write failed: " + err.Error()}
	}

	return WriteResult{
		Path:    absPath,
		Bytes:   n,
		Append:  appendMode,
		Created: err == nil && info == nil,
	}, nil
}

type WriteResult struct {
	Path    string `json:"path"`
	Bytes   int    `json:"bytes"`
	Append  bool   `json:"append"`
	Created bool   `json:"created,omitempty"`
}

type WriteError struct {
	Message string
}

func (e *WriteError) Error() string {
	return e.Message
}
