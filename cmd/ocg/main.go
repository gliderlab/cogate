package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type ProcessSpec struct {
	Name    string
	BinName string
	PidFile string
}

var (
	defaultPidDir = "/tmp/ocg"
	pidFiles      = map[string]string{
		"embedding": "ocg-embedding.pid",
		"agent":     "ocg-agent.pid",
		"gateway":   "ocg-gateway.pid",
	}
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "start":
		startCmd(args)
	case "stop":
		stopCmd(args)
	case "status":
		statusCmd(args)
	case "restart":
		restartCmd(args)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func startCmd(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to env.config")
	pidDir := fs.String("pid-dir", defaultPidDir, "Directory for pid files")
	fs.Parse(args)

	cfgPath, cfgDir := resolveConfigPath(*configPath)
	ensureDir(*pidDir)

	envConfig := readEnvConfig(cfgPath)
	binDir := resolveBinDir()

	embeddingSpec := ProcessSpec{"embedding", "ocg-embedding", filepath.Join(*pidDir, pidFiles["embedding"])}
	agentSpec := ProcessSpec{"agent", "ocg-agent", filepath.Join(*pidDir, pidFiles["agent"])}
	gatewaySpec := ProcessSpec{"gateway", "ocg-gateway", filepath.Join(*pidDir, pidFiles["gateway"])}

	if isRunning(embeddingSpec.PidFile) {
		fmt.Printf("%s already running (pid file: %s)\n", embeddingSpec.Name, embeddingSpec.PidFile)
	} else if err := startProcess(binDir, cfgDir, envConfig, embeddingSpec); err != nil {
		fatalf("Failed to start %s: %v", embeddingSpec.Name, err)
	}

	if err := waitForEmbeddingReady(cfgPath, 30*time.Second); err != nil {
		fatalf("Embedding service not ready: %v", err)
	}

	if isRunning(agentSpec.PidFile) {
		fmt.Printf("%s already running (pid file: %s)\n", agentSpec.Name, agentSpec.PidFile)
	} else if err := startProcess(binDir, cfgDir, envConfig, agentSpec); err != nil {
		fatalf("Failed to start %s: %v", agentSpec.Name, err)
	}

	if err := waitForAgentReady(cfgPath, 20*time.Second); err != nil {
		fatalf("Agent not ready: %v", err)
	}

	if isRunning(gatewaySpec.PidFile) {
		fmt.Printf("%s already running (pid file: %s)\n", gatewaySpec.Name, gatewaySpec.PidFile)
	} else if err := startProcess(binDir, cfgDir, envConfig, gatewaySpec); err != nil {
		fatalf("Failed to start %s: %v", gatewaySpec.Name, err)
	}

	if err := waitForGatewayReady(cfgPath, 20*time.Second); err != nil {
		fatalf("Gateway not ready: %v", err)
	}

	fmt.Println("✅ OCG services started")
}

func stopCmd(args []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	pidDir := fs.String("pid-dir", defaultPidDir, "Directory for pid files")
	fs.Parse(args)

	specs := []ProcessSpec{
		{"gateway", "ocg-gateway", filepath.Join(*pidDir, pidFiles["gateway"])},
		{"agent", "ocg-agent", filepath.Join(*pidDir, pidFiles["agent"])},
		{"embedding", "ocg-embedding", filepath.Join(*pidDir, pidFiles["embedding"])},
	}

	for _, spec := range specs {
		if err := stopProcess(spec); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", spec.Name, err)
		}
	}

	fmt.Println("✅ OCG services stopped")
}

func statusCmd(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to env.config")
	pidDir := fs.String("pid-dir", defaultPidDir, "Directory for pid files")
	fs.Parse(args)

	cfgPath, _ := resolveConfigPath(*configPath)

	specs := []ProcessSpec{
		{"embedding", "ocg-embedding", filepath.Join(*pidDir, pidFiles["embedding"])},
		{"agent", "ocg-agent", filepath.Join(*pidDir, pidFiles["agent"])},
		{"gateway", "ocg-gateway", filepath.Join(*pidDir, pidFiles["gateway"])},
	}

	for _, spec := range specs {
		pid, running := readPid(spec.PidFile)
		state := "stopped"
		if running {
			state = "running"
		}
		fmt.Printf("%-10s %s", spec.Name, state)
		if running {
			fmt.Printf(" (pid %d)", pid)
		}
		fmt.Println()
	}

	if err := printHealth(cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "Health check error: %v\n", err)
	}
}

func restartCmd(args []string) {
	stopCmd(args)
	startCmd(args)
}

func startProcess(binDir, cfgDir string, envConfig map[string]string, spec ProcessSpec) error {
	binPath := filepath.Join(binDir, spec.BinName)
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("binary not found: %s", binPath)
	}

	logDir := filepath.Join(filepath.Dir(spec.PidFile), "logs")
	ensureDir(logDir)
	logPath := filepath.Join(logDir, fmt.Sprintf("%s.log", spec.Name))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.Command(binPath)
	cmd.Dir = cfgDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = mergeEnv(envConfig)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if err := os.WriteFile(spec.PidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("write pid file: %w", err)
	}

	fmt.Printf("Started %s (pid %d)\n", spec.Name, cmd.Process.Pid)
	return nil
}

func stopProcess(spec ProcessSpec) error {
	pid, running := readPid(spec.PidFile)
	if !running {
		return fmt.Errorf("not running")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	steps := []struct {
		sig  syscall.Signal
		wait time.Duration
	}{
		{syscall.SIGTERM, 3 * time.Second},
		{syscall.SIGINT, 3 * time.Second},
		{syscall.SIGKILL, 2 * time.Second},
	}

	for _, step := range steps {
		_ = proc.Signal(step.sig)
		if waitForExit(pid, step.wait) {
			break
		}
	}

	_ = os.Remove(spec.PidFile)
	fmt.Printf("Stopped %s (pid %d)\n", spec.Name, pid)
	return nil
}

func waitForExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return !pidAlive(pid)
}

func waitForEmbeddingReady(cfgPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cfg := readEnvConfig(cfgPath)
		url := cfg["EMBEDDING_SERVER_URL"]
		if url == "" {
			url = cfg["EMBEDDING_SERVER_HOST"]
		}
		if url != "" {
			if strings.HasPrefix(url, "http") {
				if httpOK(url + "/health") {
					return nil
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("embedding health check timeout")
}

func waitForAgentReady(cfgPath string, timeout time.Duration) error {
	cfg := readEnvConfig(cfgPath)
	agentSock := cfg["OPENCLAW_AGENT_SOCK"]
	if agentSock == "" {
		agentSock = "/tmp/ocg-agent.sock"
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", agentSock, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("agent socket not ready: %s", agentSock)
}

func waitForGatewayReady(cfgPath string, timeout time.Duration) error {
	cfg := readEnvConfig(cfgPath)
	port := 55003
	if v := cfg["OPENCLAW_PORT"]; v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			port = p
		}
	}
	host := cfg["OPENCLAW_HOST"]
	if host == "" {
		host = "127.0.0.1"
	}
	url := fmt.Sprintf("http://%s:%d/health", host, port)
	token := cfg["OPENCLAW_UI_TOKEN"]

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if httpAuthOK(url, token) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("gateway health check timeout: %s", url)
}

func printHealth(cfgPath string) error {
	cfg := readEnvConfig(cfgPath)
	if cfg["EMBEDDING_SERVER_URL"] != "" {
		fmt.Printf("embedding health: %v\n", httpOK(cfg["EMBEDDING_SERVER_URL"]+"/health"))
	}

	port := 55003
	if v := cfg["OPENCLAW_PORT"]; v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			port = p
		}
	}
	host := cfg["OPENCLAW_HOST"]
	if host == "" {
		host = "127.0.0.1"
	}
	token := cfg["OPENCLAW_UI_TOKEN"]
	url := fmt.Sprintf("http://%s:%d/health", host, port)
	fmt.Printf("gateway health: %v\n", httpAuthOK(url, token))
	return nil
}

func httpOK(url string) bool {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func httpAuthOK(url, token string) bool {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func mergeEnv(envConfig map[string]string) []string {
	env := os.Environ()
	for k, v := range envConfig {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

func readEnvConfig(path string) map[string]string {
	config := make(map[string]string)
	f, err := os.Open(path)
	if err != nil {
		return config
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		config[key] = value
	}
	return config
}

func resolveConfigPath(requested string) (string, string) {
	if requested != "" {
		return requested, filepath.Dir(requested)
	}

	if _, err := os.Stat("env.config"); err == nil {
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, "env.config"), cwd
	}

	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		candidate := filepath.Join(exeDir, "env.config")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, exeDir
		}
		parent := filepath.Dir(exeDir)
		candidate = filepath.Join(parent, "env.config")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, parent
		}
	}

	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "env.config"), cwd
}

func resolveBinDir() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Dir(exe)
	}
	return "./bin"
}

func ensureDir(path string) {
	_ = os.MkdirAll(path, 0755)
}

func readPid(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return pid, pidAlive(pid)
}

func isRunning(pidFile string) bool {
	_, running := readPid(pidFile)
	return running
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func printUsage() {
	fmt.Println("Usage: ocg <command> [options]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  start   Start embedding, agent, gateway then exit")
	fmt.Println("  stop    Stop all OCG processes (escalating signals)")
	fmt.Println("  status  Show running state and health")
	fmt.Println("  restart Stop then start")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --config <path>   Path to env.config")
	fmt.Println("  --pid-dir <dir>   PID directory (default /tmp/ocg)")
}
