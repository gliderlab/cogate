package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/gliderlab/cogate/agent"
	"github.com/gliderlab/cogate/memory"
	"github.com/gliderlab/cogate/storage"
	"github.com/gliderlab/cogate/tools"
)

type Config struct {
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
	Port    int    `json:"port"`
	DBPath  string `json:"dbPath"`
}

func main() {
	log.Println("Starting OpenClaw Agent...")

	// 1. Read env.config (initial boot)
	envConfig := readEnvConfig("env.config")
	syncEnvToConfig("env.config", envConfig, []string{
		"OPENCLAW_API_KEY",
		"OPENCLAW_BASE_URL",
		"OPENCLAW_MODEL",
		"OPENCLAW_DB_PATH",
		"OPENCLAW_PORT",
		"OPENAI_API_KEY",
		"EMBEDDING_SERVER_URL",
		"EMBEDDING_MODEL",
	})

	// 2. Init SQLite storage
	dbPath := "ocg.db"
	if v, ok := envConfig["OPENCLAW_DB_PATH"]; ok && v != "" {
		dbPath = v
	}
	if v := os.Getenv("OPENCLAW_DB_PATH"); v != "" {
		dbPath = v
	}

	store, err := storage.New(dbPath)
	if err != nil {
		log.Fatalf("Storage init failed: %v", err)
	}
	defer store.Close()

	// Init vector memory store (FAISS + local embedding)
	embeddingServer := envConfig["EMBEDDING_SERVER_URL"]
	if v := os.Getenv("EMBEDDING_SERVER_URL"); v != "" {
		embeddingServer = v
	}
	embeddingModel := envConfig["EMBEDDING_MODEL"]
	if v := os.Getenv("EMBEDDING_MODEL"); v != "" {
		embeddingModel = v
	}
	openaiKey := envConfig["OPENAI_API_KEY"]
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		openaiKey = v
	}

	hnswPath := envConfig["HNSW_PATH"]
	if v := os.Getenv("HNSW_PATH"); v != "" {
		hnswPath = v
	}
	if hnswPath == "" {
		hnswPath = "vector.index"
	}

	memoryStore, err := memory.NewVectorMemoryStore(dbPath, memory.Config{
		EmbeddingServer: embeddingServer,
		EmbeddingModel:  embeddingModel,
		ApiKey:          openaiKey,
		HNSWPath:        hnswPath,
	})
	if err != nil {
		log.Printf("Vector memory init failed: %v", err)
	}
	if memoryStore != nil {
		defer memoryStore.Close()
	}

	// Graceful shutdown: single signal handler
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigCh
		log.Printf("Agent received signal %v, shutting down...", s)
		os.Exit(0)
	}()

	// 3. Load config (skip file if DB already has config)
	var cfg agent.Config
	configExists, _ := store.ConfigExists("llm")

	forceEnvConfig := strings.ToLower(os.Getenv("OPENCLAW_FORCE_ENV_CONFIG")) == "true"
	if forceEnvConfig {
		configExists = false
		log.Printf("[Config] Force loading from env.config (OPENCLAW_FORCE_ENV_CONFIG=true)")
	}

	if !configExists {
		// 3.1 env.config
		if v, ok := envConfig["OPENCLAW_API_KEY"]; ok && v != "" {
			cfg.APIKey = v
		}
		if v, ok := envConfig["OPENCLAW_BASE_URL"]; ok && v != "" {
			cfg.BaseURL = v
		}
		if v, ok := envConfig["OPENCLAW_MODEL"]; ok && v != "" {
			cfg.Model = v
		}

		// 3.2 environment overrides
		if v := os.Getenv("OPENCLAW_API_KEY"); v != "" {
			cfg.APIKey = v
		}
		if v := os.Getenv("OPENCLAW_BASE_URL"); v != "" {
			cfg.BaseURL = v
		}
		if v := os.Getenv("OPENCLAW_MODEL"); v != "" {
			cfg.Model = v
		}

		// 3.3 optional config.json
		cfgFile := "config.json"
		if _, err := os.Stat(cfgFile); err == nil {
			data, _ := os.ReadFile(cfgFile)
			var c Config
			if err := json.Unmarshal(data, &c); err == nil {
				if c.APIKey != "" {
					cfg.APIKey = c.APIKey
				}
				if c.BaseURL != "" {
					cfg.BaseURL = c.BaseURL
				}
				if c.Model != "" {
					cfg.Model = c.Model
				}
				if c.Port > 0 {
					os.Setenv("OPENCLAW_PORT", fmt.Sprintf("%d", c.Port))
				}
				log.Printf("Loaded config from config.json")
			}
		}
	} else {
		log.Printf("Config found in database, skipping file load")
	}

	autoRecall := strings.ToLower(os.Getenv("OPENCLAW_AUTO_RECALL"))
	if autoRecall == "" {
		autoRecall = strings.ToLower(envConfig["OPENCLAW_AUTO_RECALL"])
	}
	log.Printf("Config: API Key=%s, BaseURL=%s, Model=%s, DB=%s, AutoRecall=%v",
		maskKey(cfg.APIKey), cfg.BaseURL, cfg.Model, dbPath, autoRecall == "true")

	// 4. Init Agent with storage
	var registry *tools.Registry
	if memoryStore != nil {
		registry = tools.NewMemoryRegistry(memoryStore)
	} else {
		registry = tools.NewDefaultRegistry()
	}

	recallLimit := 3
	if v := os.Getenv("OPENCLAW_RECALL_LIMIT"); v != "" {
		fmt.Sscanf(v, "%d", &recallLimit)
	}
	if recallLimit <= 0 {
		recallLimit = 3
	}
	recallMinScore := 0.3
	if v := os.Getenv("OPENCLAW_RECALL_MINSCORE"); v != "" {
		fmt.Sscanf(v, "%f", &recallMinScore)
	}
	if recallMinScore <= 0 {
		recallMinScore = 0.3
	}

	ai := agent.New(agent.Config{
		APIKey:         cfg.APIKey,
		BaseURL:        cfg.BaseURL,
		Model:          cfg.Model,
		Storage:        store,
		MemoryStore:    memoryStore,
		Registry:       registry,
		AutoRecall:     strings.ToLower(autoRecall) == "true",
		RecallLimit:    recallLimit,
		RecallMinScore: recallMinScore,
		PulseEnabled:   true,
	})

	// 5. Start RPC service (Unix socket, no port)
	sockPath := os.Getenv("OPENCLAW_AGENT_SOCK")
	if sockPath == "" {
		sockPath = envConfig["OPENCLAW_AGENT_SOCK"]
	}
	if sockPath == "" {
		sockPath = "/tmp/ocg-agent.sock"
	}

	// Ensure old socket is removed
	_ = os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Fatalf("RPC listen failed: %v", err)
	}
	defer listener.Close()
	_ = os.Chmod(sockPath, 0666)

	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("Agent", agent.NewRPCService(ai)); err != nil {
		log.Fatalf("RPC register failed: %v", err)
	}

	log.Printf("Agent RPC listening on unix://%s", sockPath)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go rpcServer.ServeConn(conn)
		}
	}()

	// 6. Print storage stats
	if stats, err := store.Stats(); err == nil {
		log.Printf("Storage stats: %+v", stats)
	}

	// 7. Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c

	log.Println("Agent shutting down...")
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
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

func writeEnvConfig(path string, config map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	keys := make([]string, 0, len(config))
	for k := range config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(f, "%s=%s\n", k, config[k])
	}
	return nil
}

func syncEnvToConfig(path string, config map[string]string, keys []string) {
	changed := false
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			if config[k] != v {
				config[k] = v
				changed = true
			}
		}
	}
	if changed {
		_ = writeEnvConfig(path, config)
	}
}
