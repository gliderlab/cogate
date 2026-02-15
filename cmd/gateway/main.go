package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/rpc"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gliderlab/cogate/gateway"
)

type Config struct {
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
	Port    int    `json:"port"`
	DBPath  string `json:"dbPath"`
}

func main() {
	log.Println("Starting OpenClaw Gateway...")

	envConfig := readEnvConfig("env.config")

	// Parse bind host
	host := os.Getenv("OPENCLAW_HOST")
	if host == "" {
		host = envConfig["OPENCLAW_HOST"]
	}
	if host == "" {
		host = "0.0.0.0"
	}

	// Parse port
	port := os.Getenv("OPENCLAW_PORT")
	p, _ := strconv.Atoi(port)
	if p == 0 {
		if v, ok := envConfig["OPENCLAW_PORT"]; ok {
			p, _ = strconv.Atoi(v)
		}
	}

	// Fallback to config.json (optional)
	cfgFile := "config.json"
	if p == 0 {
		if _, err := os.Stat(cfgFile); err == nil {
			data, _ := os.ReadFile(cfgFile)
			var c Config
			if err := json.Unmarshal(data, &c); err == nil {
				if c.Port > 0 {
					p = c.Port
				}
				log.Printf("Loaded port from config.json")
			}
		}
	}

	if p == 0 {
		p = 55003
	}

	// Agent socket (Unix, no port)
	agentSock := os.Getenv("OPENCLAW_AGENT_SOCK")
	if agentSock == "" {
		agentSock = envConfig["OPENCLAW_AGENT_SOCK"]
	}
	if agentSock == "" {
		agentSock = "/tmp/ocg-agent.sock"
	}

	// 1) Start embedding service
	embeddingCmd, embeddingHost, embeddingPort, err := startEmbeddingService()
	if err != nil {
		log.Printf("Failed to start embedding service: %v", err)
	} else {
		log.Printf("Embedding service started: %s:%s", embeddingHost, embeddingPort)
		writeEnvConfig("env.config", map[string]string{
			"EMBEDDING_SERVER_HOST": embeddingHost,
			"EMBEDDING_SERVER_PORT": embeddingPort,
			"EMBEDDING_SERVER_URL":  fmt.Sprintf("http://%s:%s", embeddingHost, embeddingPort),
		})
	}

	// 2) Start Agent
	agentCmd, err := startAgent(agentSock)
	if err != nil {
		log.Fatalf("Failed to start Agent: %v", err)
	}

	client, err := waitForAgent(agentSock, 20*time.Second)
	if err != nil {
		_ = agentCmd.Process.Kill()
		log.Fatalf("Failed to connect to Agent: %v", err)
	}

	uiToken := os.Getenv("OPENCLAW_UI_TOKEN")
	if uiToken == "" {
		uiToken = envConfig["OPENCLAW_UI_TOKEN"]
	}

	srv := gateway.New(gateway.Config{
		Host:        host,
		Port:        p,
		AgentAddr:   agentSock,
		UIAuthToken: uiToken,
	})
	srv.SetClient(client)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Gateway start failed: %v", err)
		}
	}()

	log.Printf("Gateway listening on http://%s:%d", host, p)
	log.Println("Waiting for messages...")

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c

	log.Println("Gateway shutting down...")
	srv.Stop()
	if embeddingCmd != nil && embeddingCmd.Process != nil {
		embeddingCmd.Process.Signal(syscall.SIGTERM)
		embeddingCmd.Process.Kill()
	}
	_ = agentCmd.Process.Signal(syscall.SIGTERM)
	_ = agentCmd.Process.Kill()
	os.Exit(0)
}

func startEmbeddingService() (*exec.Cmd, string, string, error) {
	exePath, _ := os.Executable()
	binDir := filepath.Dir(exePath)
	embeddingPath := filepath.Join(binDir, "ocg-embedding")

	if _, err := os.Stat(embeddingPath); err != nil {
		return nil, "", "", fmt.Errorf("ocg-embedding not found at %s", embeddingPath)
	}

	cmd := exec.Command(embeddingPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, "", "", err
	}

	// Wait for embedding server to update env.config
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		cfg := readEnvConfig("env.config")
		host := cfg["EMBEDDING_SERVER_HOST"]
		port := cfg["EMBEDDING_SERVER_PORT"]
		if port != "" {
			if host == "" {
				host = "0.0.0.0"
			}
			return cmd, host, port, nil
		}
	}

	return cmd, "0.0.0.0", "unknown", nil
}

func startAgent(agentAddr string) (*exec.Cmd, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	binDir := filepath.Dir(exePath)
	agentPath := filepath.Join(binDir, "ocg-agent")
	if runtime.GOOS == "windows" {
		agentPath += ".exe"
	}

	cmd := exec.Command(agentPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	env := os.Environ()
	env = append(env, "OPENCLAW_AGENT_SOCK="+agentAddr)
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	log.Printf("Started Agent: %s", agentPath)
	return cmd, nil
}

func waitForAgent(addr string, timeout time.Duration) (*rpc.Client, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		client, err := rpc.Dial("unix", addr)
		if err == nil {
			return client, nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for agent at %s", addr)
}

// writeEnvConfig writes env.config (KEY=VALUE)
func writeEnvConfig(path string, updates map[string]string) {
	config := readEnvConfig(path)
	for k, v := range updates {
		config[k] = v
	}
	f, err := os.Create(path)
	if err != nil {
		log.Printf("⚠️ failed to write config: %v", err)
		return
	}
	defer f.Close()
	keys := make([]string, 0, len(config))
	for k := range config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(f, "%s=%s\n", k, config[k])
	}
	log.Printf("📁 config updated: %s", path)
}

// readEnvConfig reads env.config (KEY=VALUE)
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
