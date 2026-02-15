package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/rpc"
	"os"
	"os/signal"
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

	// 1) Connect to Agent (ocg-managed)
	client, err := waitForAgent(agentSock, 20*time.Second)
	if err != nil {
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
	os.Exit(0)
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
