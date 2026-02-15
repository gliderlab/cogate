// Local Embedding Server - GGUF embedding service (bundled llama.cpp server)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Config
type Config struct {
	Host       string `json:"host"`
	ModelPath  string `json:"modelPath"`
	ServerPort int    `json:"serverPort"`
	LLMHost    string `json:"llmHost"`
	LLMPort    int    `json:"llmPort"`
	LLMServer  string `json:"llmServer"`
	LlamaBin   string `json:"llamaBin"`
	Dim        int    `json:"dim"`
	MaxTokens  int    `json:"maxTokens"`
	Verbose    bool   `json:"verbose"`
}

var (
	config     Config
	llamaCmd   *exec.Cmd
	llamaDone  chan struct{}
	configPath = "env.config"
)

func main() {
	// Parse command-line arguments
	port := flag.Int("port", 0, "Server port (50000-60000, 0 for auto)")
	model := flag.String("model", "", "Path to GGUF embedding model")
	llmPort := flag.Int("llm-port", 0, "llama.cpp server port (18000-19000, 0 for auto)")
	flag.Parse()

	// Read existing env.config
	existingConfig := readEnvConfig(configPath)

	// Host/Port (supports host:port shorthand)
	embeddingAddr := existingConfig["EMBEDDING_SERVER_ADDR_PORT"]
	if v := os.Getenv("EMBEDDING_SERVER_ADDR_PORT"); v != "" {
		embeddingAddr = v
	}
	if embeddingAddr != "" {
		parts := strings.Split(embeddingAddr, ":")
		if len(parts) == 2 {
			config.Host = parts[0]
			fmt.Sscanf(parts[1], "%d", &config.ServerPort)
		}
	}

	config.Host = existingConfig["EMBEDDING_SERVER_HOST"]
	if v := os.Getenv("EMBEDDING_SERVER_HOST"); v != "" {
		config.Host = v
	}
	if config.Host == "" {
		config.Host = "0.0.0.0"
	}

	llamaAddr := existingConfig["LLAMA_SERVER_ADDR_PORT"]
	if v := os.Getenv("LLAMA_SERVER_ADDR_PORT"); v != "" {
		llamaAddr = v
	}
	if llamaAddr != "" {
		parts := strings.Split(llamaAddr, ":")
		if len(parts) == 2 {
			config.LLMHost = parts[0]
			fmt.Sscanf(parts[1], "%d", &config.LLMPort)
		}
	}

	config.LLMHost = existingConfig["LLAMA_SERVER_HOST"]
	if v := os.Getenv("LLAMA_SERVER_HOST"); v != "" {
		config.LLMHost = v
	}
	if config.LLMHost == "" {
		config.LLMHost = "0.0.0.0"
	}

	// Ports
	config.ServerPort = *port
	if config.ServerPort == 0 {
		if v, ok := existingConfig["EMBEDDING_SERVER_PORT"]; ok {
			fmt.Sscanf(v, "%d", &config.ServerPort)
		}
	}
	if config.ServerPort == 0 {
		config.ServerPort = findFreePort(50000, 60000)
	}

	config.LLMPort = *llmPort
	if config.LLMPort == 0 {
		if v, ok := existingConfig["LLAMA_SERVER_PORT"]; ok {
			fmt.Sscanf(v, "%d", &config.LLMPort)
		}
	}
	if config.LLMPort == 0 {
		config.LLMPort = findFreePort(18000, 19000)
	}

	if embeddingAddr == "" {
		embeddingAddr = fmt.Sprintf("%s:%d", config.Host, config.ServerPort)
	}
	if llamaAddr == "" {
		llamaAddr = fmt.Sprintf("%s:%d", config.LLMHost, config.LLMPort)
	}

	// Optional custom llama-server path
	config.LlamaBin = existingConfig["LLAMA_SERVER_BIN"]
	if v := os.Getenv("LLAMA_SERVER_BIN"); v != "" {
		config.LlamaBin = v
	}

	// Model path
	config.ModelPath = *model
	if config.ModelPath == "" {
		config.ModelPath = os.Getenv("EMBEDDING_MODEL_PATH")
	}
	if config.ModelPath == "" {
		config.ModelPath = existingConfig["EMBEDDING_MODEL_PATH"]
	}
	if config.ModelPath == "" {
		config.ModelPath = "models/embeddinggemma-300M-Q8_0.gguf"
	}

	config.LLMServer = fmt.Sprintf("http://%s:%d", config.LLMHost, config.LLMPort)

	// Verbose flag (default quiet)
	verb := os.Getenv("EMBEDDING_VERBOSE")
	if verb == "" {
		verb = existingConfig["EMBEDDING_VERBOSE"]
	}
	config.Verbose = strings.ToLower(strings.TrimSpace(verb)) == "true"

	// Ensure model file exists
	if _, err := os.Stat(config.ModelPath); os.IsNotExist(err) {
		log.Fatalf("❌ model file not found: %s", config.ModelPath)
	}

	// Default llama-server binary path: prefer project root bin/llama-server; fallback to submodule build
	exePath, _ := os.Executable()
	baseDir := filepath.Dir(exePath)
	if config.LlamaBin == "" {
		primary := filepath.Join(baseDir, "llama-server")
		fallback := filepath.Join(filepath.Dir(baseDir), "llama.cpp", "build", "bin", "llama-server")
		if _, err := os.Stat(primary); err == nil {
			config.LlamaBin = primary
		} else {
			config.LlamaBin = fallback
		}
	}

	// Write env.config
	writeEnvConfig(configPath, map[string]string{
		"EMBEDDING_MODEL_PATH":       config.ModelPath,
		"EMBEDDING_SERVER_ADDR_PORT": embeddingAddr,
		"EMBEDDING_SERVER_HOST":      config.Host,
		"EMBEDDING_SERVER_PORT":      fmt.Sprintf("%d", config.ServerPort),
		"EMBEDDING_SERVER_URL":       fmt.Sprintf("http://%s:%d", config.Host, config.ServerPort),
		"LLAMA_SERVER_ADDR_PORT":     llamaAddr,
		"LLAMA_SERVER_HOST":          config.LLMHost,
		"LLAMA_SERVER_PORT":          fmt.Sprintf("%d", config.LLMPort),
		"LLM_SERVER_URL":             fmt.Sprintf("http://%s:%d", config.LLMHost, config.LLMPort),
		"LLAMA_SERVER_BIN":           config.LlamaBin,
		"EMBEDDING_VERBOSE":          fmt.Sprintf("%v", config.Verbose),
	})

	log.Printf("Starting local embedding service...")
	log.Printf("Model: %s", config.ModelPath)
	log.Printf("Embedding service: http://%s:%d", config.Host, config.ServerPort)
	log.Printf("Llama server: http://%s:%d", config.LLMHost, config.LLMPort)

	// Start llama.cpp server
	if err := startLlamaServer(); err != nil {
		log.Printf("Failed to start llama server: %v", err)
	} else {
		// Wait for llama server ready
		waitForLlamaReady()
	}

	// Start HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/embed", embedHandler)
	mux.HandleFunc("/embed-batch", embedBatchHandler)
	mux.HandleFunc("/info", infoHandler)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Host, config.ServerPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  90 * time.Second,
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		s := <-sigCh
		log.Printf("Received signal %v, shutting down...", s)
		stopLlamaServer()
		server.Close()
	}()

	log.Printf("Embedding service started: http://%s:%d", config.Host, config.ServerPort)
	log.Fatal(server.ListenAndServe())
}

// Start llama.cpp server
func startLlamaServer() error {
	if runtime.GOOS != "windows" {
		_ = exec.Command("pkill", "-f", "llama.cpp/build/bin/llama-server").Run()
	}

	// Prefer configured binary path
	llamaPath := config.LlamaBin
	if _, err := os.Stat(llamaPath); os.IsNotExist(err) {
		// Try building in submodule as fallback
		exePath, _ := os.Executable()
		baseDir := filepath.Dir(exePath)
		llamaDir := filepath.Join(filepath.Dir(baseDir), "llama.cpp")
		llamaPath = filepath.Join(llamaDir, "build", "bin", "llama-server")
		log.Printf("llama-server not found at %s, building from source in %s...", config.LlamaBin, llamaDir)
		if err := buildLlamaServer(llamaDir); err != nil {
			return fmt.Errorf("failed to build llama-server: %v", err)
		}
	}

	args := []string{
		"-m", config.ModelPath,
		"--port", fmt.Sprintf("%d", config.LLMPort),
		"--host", config.LLMHost,
		"--embedding",
		"--threads", "4",
		"--ctx-size", "2048",
	}

	llamaCmd = exec.Command(llamaPath, args...)
	llamaCmd.Dir = filepath.Dir(llamaPath)
	if config.Verbose {
		llamaCmd.Stdout = os.Stdout
		llamaCmd.Stderr = os.Stderr
	} else {
		llamaCmd.Stdout = io.Discard
		llamaCmd.Stderr = io.Discard
	}
	llamaDone = make(chan struct{})

	go func() {
		if err := llamaCmd.Run(); err != nil {
			log.Printf("llama server exited: %v", err)
		}
		close(llamaDone)
	}()

	log.Printf("Started llama server: %s", strings.Join(args, " "))
	return nil
}

// Build llama-server
func buildLlamaServer(dir string) error {
	buildDir := filepath.Join(dir, "build")
	os.MkdirAll(buildDir, 0755)

	if runtime.GOOS == "windows" {
		return fmt.Errorf("llama-server build not supported on windows; prebuild required")
	}
	// Use llama.cpp Makefile
	makeCmd := exec.Command("make", "LLAMA_BUILD_EMBEDDING=ON", "-j", "4")
	makeCmd.Dir = dir
	makeCmd.Stdout = os.Stdout
	makeCmd.Stderr = os.Stderr
	return makeCmd.Run()
}

// Stop llama server
func stopLlamaServer() {
	if llamaCmd != nil && llamaCmd.Process != nil {
		llamaCmd.Process.Signal(os.Interrupt)
		select {
		case <-llamaDone:
		case <-time.After(5 * time.Second):
			llamaCmd.Process.Kill()
		}
	}
}

// Wait for llama server readiness
func waitForLlamaReady() {
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, config.LLMServer+"/health", nil)
		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				log.Printf("Llama server is ready")
				return
			}
		}
		time.Sleep(time.Second)
	}
	log.Printf("Llama server start timeout, continuing...")
}

// Health check handler
func healthHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "ok",
		"serverPort": config.ServerPort,
		"llmServer":  config.LLMServer,
		"model":      config.ModelPath,
		"timestamp":  time.Now().Unix(),
	})
}

// Embed a single text
func embedHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}

	embedding, err := getEmbedding(req.Text)
	if err != nil {
		http.Error(w, fmt.Sprintf("Embedding failed: %v", err), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"embedding": embedding,
		"dim":       len(embedding),
		"model":     config.ModelPath,
	})
}

// Embed a batch of texts
func embedBatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Texts []string `json:"texts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if len(req.Texts) == 0 {
		http.Error(w, "texts is required", http.StatusBadRequest)
		return
	}

	embeddings := make([][]float32, 0, len(req.Texts))
	for _, text := range req.Texts {
		if strings.TrimSpace(text) == "" {
			http.Error(w, "empty text in batch", http.StatusBadRequest)
			return
		}
		emb, err := getEmbedding(text)
		if err != nil {
			short := text
			if len(short) > 50 {
				short = short[:50]
			}
			http.Error(w, fmt.Sprintf("Embedding failed for: %s", short), http.StatusInternalServerError)
			return
		}
		embeddings = append(embeddings, emb)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"embeddings": embeddings,
		"count":      len(embeddings),
		"dim":        len(embeddings[0]),
	})
}

// Get model info
func infoHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"modelPath":  config.ModelPath,
		"serverPort": config.ServerPort,
		"llmServer":  config.LLMServer,
		"dim":        config.Dim,
		"maxTokens":  config.MaxTokens,
		"endpoints": map[string]string{
			"/health":      "Health check",
			"/embed":       "Embed single text (POST)",
			"/embed-batch": "Embed batch (POST)",
			"/info":        "Model info",
		},
	})
}

// Call llama.cpp server to get embeddings
func getEmbedding(text string) ([]float32, error) {
	url := fmt.Sprintf("%s/embedding", strings.TrimSuffix(config.LLMServer, "/"))

	reqBody, _ := json.Marshal(map[string]interface{}{
		"content": text,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("llama.cpp server returned %d: %s", resp.StatusCode, string(body))
	}

	var raw []map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %v", err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	emb, ok := raw[0]["embedding"].([]interface{})
	if !ok || len(emb) == 0 {
		return nil, fmt.Errorf("no embedding field in response")
	}

	firstRow, ok := emb[0].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid embedding format")
	}

	result := make([]float32, len(firstRow))
	for i, v := range firstRow {
		if f, ok := v.(float64); ok {
			result[i] = float32(f)
		}
	}
	return result, nil
}

// Find a free port
func findFreePort(min, max int) int {
	for port := min; port <= max; port++ {
		if err := checkPort(port); err == nil {
			return port
		}
	}
	panic("no free port found")
}

func checkPort(port int) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), time.Second)
	if err == nil {
		conn.Close()
		return fmt.Errorf("port %d in use", port)
	}
	return nil
}

// Read env.config
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
		if len(parts) == 2 {
			config[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return config
}

// Write env.config
func writeEnvConfig(path string, updates map[string]string) {
	config := readEnvConfig(path)
	for k, v := range updates {
		config[k] = v
	}
	f, err := os.Create(path)
	if err != nil {
		log.Printf("Failed to write config: %v", err)
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
	log.Printf("Config written: %s", path)
}
