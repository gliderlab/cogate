// Process Tool - process management tool (built into Gateway)
package processtool

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

func (t *ProcessTool) Execute(args map[string]interface{}) (interface{}, error) {
	action := getString(args, "action")

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
	command := getString(args, "command")
	workdir := getString(args, "workdir")
	envList := getString(args, "env")
	usePty := getBool(args, "pty")

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
		// PTY mode: pty.Start already started the process
		ptyFile, err = pty.Start(cmd)
		if err != nil {
			return nil, fmt.Errorf("PTY start failed: %v", err)
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
			return nil, fmt.Errorf("start failed: %v", err)
		}
	}

	// Generate sessionId
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

	log.Printf("✅ Process started: %s (PID: %d, PTY: %v)", sessionId, cmd.Process.Pid, usePty)

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
			log.Printf("🔚 Process ended: %s (exit code: %d)", sessionId, cmd.ProcessState.ExitCode())
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
	sessionId := getString(args, "sessionId")
	offset := getInt(args, "offset")
	limit := getInt(args, "limit")

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

	return ProcessLogResult{
		SessionID: sessionId,
		Offset:    offset,
		Content:   output,
		Truncated: len(output) < len(content[offset:]),
	}, nil
}

func (t *ProcessTool) write(args map[string]interface{}) (interface{}, error) {
	sessionId := getString(args, "sessionId")
	data := getString(args, "data")
	eof := getBool(args, "eof")

	if sessionId == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	if data == "" {
		return nil, fmt.Errorf("data is required")
	}

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
		return nil, fmt.Errorf("stdin not available")
	}

	if err != nil {
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
	sessionId := getString(args, "sessionId")

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
		return nil, fmt.Errorf("kill failed: %v", err)
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

func getString(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(args map[string]interface{}, key string) int {
	if v, ok := args[key]; ok {
		switch f := v.(type) {
		case float64:
			return int(f)
		case int:
			return f
		case string:
			var i int
			fmt.Sscanf(f, "%d", &i)
			return i
		}
	}
	return 0
}

func getBool(args map[string]interface{}, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
