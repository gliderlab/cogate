// Read Tool - read file content
package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ReadTool struct{}

func (t *ReadTool) Name() string {
	return "read"
}

func (t *ReadTool) Description() string {
	return "Read file content. Supports text files and images (base64). Max 50KB per call."
}

func (t *ReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "Start line (optional, 1-based)",
				"default":     1,
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max lines (optional, default all)",
				"default":     0,
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadTool) Execute(args map[string]interface{}) (interface{}, error) {
	path := GetString(args, "path")
	offset := GetInt(args, "offset")
	limit := GetInt(args, "limit")

	if path == "" {
		return nil, &ReadError{Message: "path is required"}
	}

	// Validate path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, &ReadError{Message: "invalid path: " + err.Error()}
	}

	// Check file exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &ReadError{Message: "file does not exist: " + absPath}
		}
		return nil, &ReadError{Message: "cannot access file: " + err.Error()}
	}

	if info.IsDir() {
		return nil, &ReadError{Message: "This is a directory, not a file"}
	}

	// Size limit 50KB
	if info.Size() > 50*1024 {
		return nil, &ReadError{Message: "File too large (>50KB)"}
	}

	// Read file content
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, &ReadError{Message: "read failed: " + err.Error()}
	}

	result := ReadResult{
		Path:  absPath,
		Size:  len(content),
		Bytes: content,
	}

	// Detect binary file
	if isBinaryFile(content) {
		result.IsBinary = true
		result.Content = fmt.Sprintf("[binary file %s]", formatSize(len(content)))
	} else {
		text := string(content)
		lines := strings.Split(text, "\n")

		if offset > 0 && offset <= len(lines) {
			lines = lines[offset-1:]
		}
		if limit > 0 && limit < len(lines) {
			lines = lines[:limit]
		}

		result.IsBinary = false
		result.Content = strings.Join(lines, "\n")
	}

	return result, nil
}

func isBinaryFile(content []byte) bool {
	for _, b := range content {
		if b == 0 {
			return true
		}
	}
	return false
}

func formatSize(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

type ReadResult struct {
	Path     string `json:"path"`
	Size     int    `json:"size"`
	Content  string `json:"content"`
	IsBinary bool   `json:"is_binary"`
	Bytes    []byte `json:"-"`
}

type ReadError struct {
	Message string
}

func (e *ReadError) Error() string {
	return e.Message
}
