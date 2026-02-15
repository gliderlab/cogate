// Process Tool - manage running processes with optional PTY
package tools

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

type ProcessInfo struct {
	ID        string
	Cmd       *exec.Cmd
	Buffer    *bytes.Buffer
	Pty       *os.File
	StdinPipe io.WriteCloser
	Mutex     sync.Mutex
	CreatedAt time.Time
}

var (
	processes = make(map[string]*ProcessInfo)
	procMutex sync.Mutex
)

type ProcessTool struct{}

func (t *ProcessTool) Name() string {
	return "process"
}

func (t *ProcessTool) Description() string {
	return "Manage processes: start (PTY supported), list, tail logs, write stdin, kill."
}

func (t *ProcessTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: start, list, log, write, kill",
			},
			"sessionId": map[string]interface{}{
				"type":        "string",
				"description": "Process session ID",
			},
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command to execute (required for start)",
			},
			"workdir": map[string]interface{}{
				"type":        "string",
				"description": "Working directory",
			},
			"env": map[string]interface{}{
				"type":        "string",
				"description": "Environment variables (newline separated)",
			},
			"pty": map[string]interface{}{
				"type":        "boolean",
				"description": "Use PTY (interactive terminal)",
				"default":     false,
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "Log start offset",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Log length limit",
			},
			"data": map[string]interface{}{
				"type":        "string",
				"description": "Data to write to stdin",
			},
			"eof": map[string]interface{}{
				"type":        "boolean",
				"description": "Close stdin after write",
			},
		},
		"required": []string{"action"},
	}
}

func (t *ProcessTool) Execute(args map[string]interface{}) (interface{}, error) {
	action := GetString(args, "action")

	switch action {
	case "start":
		return t.start(args)
	case "list":
		return t.list()
	case "log":
		return t.log(args)
	case "write":
		return t.write(args)
	case "kill":
		return t.kill(args)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (t *ProcessTool) start(args map[string]interface{}) (interface{}, error) {
	command := GetString(args, "command")
	workdir := GetString(args, "workdir")
	envList := GetString(args, "env")
	usePty := GetBool(args, "pty")

	if command == "" {
		return nil, fmt.Errorf("command is required")
	}

	// Parse command
	var cmd *exec.Cmd
	if strings.Contains(command, " ") {
		parts := strings.Fields(command)
		if len(parts) > 1 {
			cmd = exec.Command(parts[0], parts[1:]...)
		} else {
			cmd = exec.Command(command)
		}
	} else {
		cmd = exec.Command(command)
	}

	// Working directory
	if workdir != "" {
		cmd.Dir = workdir
	}

	// Environment variables
	if envList != "" {
		envs := strings.Split(envList, "\n")
		envs = append(envs, "PATH=/usr/local/bin:/usr/bin:/bin")
		cmd.Env = envs
	}

	var (
		buf       bytes.Buffer
		stdinPipe io.WriteCloser
		ptyFile   *os.File
		err       error
	)

	if usePty {
		// PTY mode
		ptyFile, err = pty.Start(cmd)
		if err != nil {
			return nil, fmt.Errorf("failed to start PTY: %v", err)
		}
	} else {
		// Non-PTY mode
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		stdinPipe, err = cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stdin pipe: %v", err)
		}
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("failed to start: %v", err)
		}
	}

	// Create sessionId
	sessionId := fmt.Sprintf("proc_%d", time.Now().UnixNano())

	procMutex.Lock()
	processes[sessionId] = &ProcessInfo{
		ID:        sessionId,
		Cmd:       cmd,
		Buffer:    &buf,
		Pty:       ptyFile,
		StdinPipe: stdinPipe,
		CreatedAt: time.Now(),
	}
	procMutex.Unlock()

	log.Printf("✅ process started: %s (PID: %d, PTY: %v)", sessionId, cmd.Process.Pid, usePty)

	// Read PTY output asynchronously
	if usePty {
		go func() {
			readBuf := make([]byte, 1024)
			for {
				n, err := ptyFile.Read(readBuf)
				if err != nil {
					break
				}
				procMutex.Lock()
				p, ok := processes[sessionId]
				if ok {
					p.Mutex.Lock()
					p.Buffer.Write(readBuf[:n])
					p.Mutex.Unlock()
				}
				procMutex.Unlock()
			}
		}()
	}

	// Wait asynchronously for completion
	go func() {
		cmd.Wait()
		procMutex.Lock()
		if _, ok := processes[sessionId]; ok {
			log.Printf("🔚 process exited: %s (exit code: %d)", sessionId, cmd.ProcessState.ExitCode())
		}
		procMutex.Unlock()
	}()

	return ProcessStartResult{
		SessionID: sessionId,
		PID:       cmd.Process.Pid,
		Command:   command,
		Pty:       usePty,
		Success:   true,
	}, nil
}

func (t *ProcessTool) list() (interface{}, error) {
	procMutex.Lock()
	defer procMutex.Unlock()

	items := make([]map[string]interface{}, 0)
	for id, p := range processes {
		var status string
		if p.Cmd.ProcessState == nil {
			status = "running"
		} else if p.Cmd.ProcessState.Exited() {
			status = "exited"
		} else {
			status = "running"
		}

		items = append(items, map[string]interface{}{
			"sessionId": id,
			"pid":       p.Cmd.Process.Pid,
			"status":    status,
			"pty":       p.Pty != nil,
			"createdAt": p.CreatedAt.Format(time.RFC3339),
		})
	}

	return map[string]interface{}{
		"processes": items,
		"count":     len(items),
	}, nil
}

func (t *ProcessTool) log(args map[string]interface{}) (interface{}, error) {
	sessionId := GetString(args, "sessionId")
	offset := GetInt(args, "offset")
	limit := GetInt(args, "limit")

	if sessionId == "" {
		return nil, fmt.Errorf("sessionId is required")
	}

	procMutex.Lock()
	p, ok := processes[sessionId]
	procMutex.Unlock()

	if !ok {
		return nil, fmt.Errorf("process not found: %s", sessionId)
	}

	p.Mutex.Lock()
	content := p.Buffer.String()
	p.Mutex.Unlock()

	if offset < 0 {
		offset = 0
	}
	if offset > len(content) {
		offset = len(content)
	}

	output := content[offset:]
	if limit > 0 && limit < len(output) {
		output = output[:limit]
	}

	maxLen := 8000
	if len(output) > maxLen {
		output = output[:maxLen]
	}

	return ProcessLogResult{
		SessionID: sessionId,
		Offset:    offset,
		Content:   output,
		Truncated: len(output) < len(content[offset:]),
	}, nil
}

func (t *ProcessTool) write(args map[string]interface{}) (interface{}, error) {
	sessionId := GetString(args, "sessionId")
	data := GetString(args, "data")
	eof := GetBool(args, "eof")

	if sessionId == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	// Allow empty data when using EOF

	procMutex.Lock()
	p, ok := processes[sessionId]
	procMutex.Unlock()

	if !ok {
		return nil, fmt.Errorf("process not found: %s", sessionId)
	}

	var n int
	var err error

	if p.Pty != nil {
		n, err = p.Pty.Write([]byte(data))
	} else if p.StdinPipe != nil {
		n, err = p.StdinPipe.Write([]byte(data))
	} else {
		p.Mutex.Unlock()
		return nil, fmt.Errorf("stdin not available")
	}

	if err != nil {
		p.Mutex.Unlock()
		return nil, fmt.Errorf("write failed: %v", err)
	}

	if eof {
		if p.Pty != nil {
			p.Pty.Close()
			p.Pty = nil
		}
		if p.StdinPipe != nil {
			p.StdinPipe.Close()
			p.StdinPipe = nil
		}
	}

	return map[string]interface{}{
		"sessionId": sessionId,
		"written":   n,
		"eof":       eof,
	}, nil
}

func (t *ProcessTool) kill(args map[string]interface{}) (interface{}, error) {
	sessionId := GetString(args, "sessionId")

	if sessionId == "" {
		return nil, fmt.Errorf("sessionId is required")
	}

	procMutex.Lock()
	p, ok := processes[sessionId]
	procMutex.Unlock()

	if !ok {
		return nil, fmt.Errorf("process not found: %s", sessionId)
	}

	// Close PTY/stdin
	if p.Pty != nil {
		p.Pty.Close()
	}
	if p.StdinPipe != nil {
		p.StdinPipe.Close()
	}

	if err := p.Cmd.Process.Kill(); err != nil {
		return nil, fmt.Errorf("failed to kill: %v", err)
	}

	procMutex.Lock()
	delete(processes, sessionId)
	procMutex.Unlock()

	return map[string]interface{}{
		"sessionId": sessionId,
		"killed":    true,
	}, nil
}

type ProcessStartResult struct {
	SessionID string `json:"sessionId"`
	PID       int    `json:"pid"`
	Command   string `json:"command"`
	Pty       bool   `json:"pty,omitempty"`
	Success   bool   `json:"success"`
}

type ProcessLogResult struct {
	SessionID string `json:"sessionId"`
	Offset    int    `json:"offset"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated,omitempty"`
}
