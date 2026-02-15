// Cron job system for OpenClaw-Go
// Provides scheduled task execution with multiple schedule types and delivery modes

package cron

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Schedule kinds
const (
	ScheduleKindAt    = "at"
	ScheduleKindEvery = "every"
	ScheduleKindCron  = "cron"
)

// Session targets
const (
	SessionTargetMain     = "main"
	SessionTargetIsolated = "isolated"
)

// Wake modes
const (
	WakeModeNow         = "now"
	WakeModeNextHeartbeat = "next-heartbeat"
)

// Delivery modes
const (
	DeliveryModeAnnounce = "announce"
	DeliveryModeNone     = "none"
)

// Payload kinds
const (
	PayloadKindSystemEvent = "systemEvent"
	PayloadKindAgentTurn  = "agentTurn"
)

// Schedule defines when a job should run
type Schedule struct {
	Kind     string `json:"kind"`     // "at", "every", "cron"
	At       string `json:"at,omitempty"`       // ISO 8601 timestamp
	EveryMs  int64  `json:"everyMs,omitempty"`  // milliseconds
	Expr     string `json:"expr,omitempty"`     // cron expression
	Tz       string `json:"tz,omitempty"`       // timezone
}

// Payload defines what the job should do
type Payload struct {
	Kind         string `json:"kind"` // "systemEvent", "agentTurn"
	Text         string `json:"text,omitempty"`    // for systemEvent
	Message      string `json:"message,omitempty"` // for agentTurn
	Model        string `json:"model,omitempty"`
	Thinking     string `json:"thinking,omitempty"`
	TimeoutSeconds int   `json:"timeoutSeconds,omitempty"`
}

// Delivery defines how to deliver job output
type Delivery struct {
	Mode        string `json:"mode"` // "announce", "none"
	Channel     string `json:"channel,omitempty"` // "telegram", "discord", etc.
	To          string `json:"to,omitempty"`     // channel-specific target
	BestEffort  bool   `json:"bestEffort"`
}

// Job represents a scheduled job
type Job struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	AgentID     string    `json:"agentId,omitempty"` // specific agent or empty for default
	Enabled     bool      `json:"enabled"`
	Schedule    Schedule  `json:"schedule"`
	SessionTarget string  `json:"sessionTarget"` // "main" or "isolated"
	WakeMode    string    `json:"wakeMode"`     // "now" or "next-heartbeat"
	Payload     Payload   `json:"payload"`
	Delivery    *Delivery `json:"delivery,omitempty"`
	DeleteAfterRun bool   `json:"deleteAfterRun"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	// State
	State struct {
		NextRunAtMs     int64  `json:"nextRunAtMs"`
		LastRunAtMs     int64  `json:"lastRunAtMs"`
		LastStatus      string `json:"lastStatus"` // "ok", "error", "skipped"
		LastDurationMs  int64  `json:"lastDurationMs"`
		ConsecutiveErrors int `json:"consecutiveErrors"`
	} `json:"state"`
}

// JobStore manages cron jobs
type JobStore struct {
	mu      sync.RWMutex
	jobs    map[string]*Job
	filePath string
}

// NewJobStore creates a new job store
func NewJobStore(filePath string) *JobStore {
	js := &JobStore{
		jobs:     make(map[string]*Job),
		filePath: filePath,
	}
	js.load()
	return js
}

// load loads jobs from file
func (js *JobStore) load() {
	data, err := os.ReadFile(js.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[Cron] Failed to load jobs: %v", err)
		}
		return
	}

	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		log.Printf("[Cron] Failed to parse jobs: %v", err)
		return
	}

	js.mu.Lock()
	defer js.mu.Unlock()
	for _, job := range jobs {
		js.jobs[job.ID] = job
	}
	log.Printf("[Cron] Loaded %d jobs", len(jobs))
}

// save saves jobs to file
func (js *JobStore) save() error {
	js.mu.RLock()
	defer js.mu.RUnlock()

	jobs := make([]*Job, 0, len(js.jobs))
	for _, job := range js.jobs {
		jobs = append(jobs, job)
	}

	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(js.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(js.filePath, data, 0644)
}

// Add adds a new job
func (js *JobStore) Add(job *Job) error {
	js.mu.Lock()
	defer js.mu.Unlock()

	js.jobs[job.ID] = job
	return js.save()
}

// Get returns a job by ID
func (js *JobStore) Get(id string) (*Job, bool) {
	js.mu.RLock()
	defer js.mu.RUnlock()
	job, ok := js.jobs[id]
	return job, ok
}

// List returns all jobs
func (js *JobStore) List() []*Job {
	js.mu.RLock()
	defer js.mu.RUnlock()

	jobs := make([]*Job, 0, len(js.jobs))
	for _, job := range js.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// Update updates a job
func (js *JobStore) Update(id string, updates map[string]interface{}) (*Job, error) {
	js.mu.Lock()
	defer js.mu.Unlock()

	job, ok := js.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job not found: %s", id)
	}

	// Apply updates
	if v, ok := updates["name"].(string); ok {
		job.Name = v
	}
	if v, ok := updates["description"].(string); ok {
		job.Description = v
	}
	if v, ok := updates["enabled"].(bool); ok {
		job.Enabled = v
	}
	if v, ok := updates["schedule"].(map[string]interface{}); ok {
		if kind, ok := v["kind"].(string); ok {
			job.Schedule.Kind = kind
		}
		if at, ok := v["at"].(string); ok {
			job.Schedule.At = at
		}
		if everyMs, ok := v["everyMs"].(float64); ok {
			job.Schedule.EveryMs = int64(everyMs)
		}
		if expr, ok := v["expr"].(string); ok {
			job.Schedule.Expr = expr
		}
		if tz, ok := v["tz"].(string); ok {
			job.Schedule.Tz = tz
		}
	}
	if v, ok := updates["payload"].(map[string]interface{}); ok {
		if kind, ok := v["kind"].(string); ok {
			job.Payload.Kind = kind
		}
		if text, ok := v["text"].(string); ok {
			job.Payload.Text = text
		}
		if message, ok := v["message"].(string); ok {
			job.Payload.Message = message
		}
		if model, ok := v["model"].(string); ok {
			job.Payload.Model = model
		}
		if thinking, ok := v["thinking"].(string); ok {
			job.Payload.Thinking = thinking
		}
	}

	job.UpdatedAt = time.Now()
	js.jobs[id] = job

	if err := js.save(); err != nil {
		return nil, err
	}

	return job, nil
}

// Remove removes a job
func (js *JobStore) Remove(id string) error {
	js.mu.Lock()
	defer js.mu.Unlock()

	if _, ok := js.jobs[id]; !ok {
		return fmt.Errorf("job not found: %s", id)
	}

	delete(js.jobs, id)
	return js.save()
}

// GetDueJobs returns jobs that are due to run
func (js *JobStore) GetDueJobs() []*Job {
	js.mu.RLock()
	defer js.mu.RUnlock()

	now := time.Now().UnixMilli()
	var due []*Job

	for _, job := range js.jobs {
		if !job.Enabled {
			continue
		}
		if job.State.NextRunAtMs > 0 && job.State.NextRunAtMs <= now {
			due = append(due, job)
		}
	}

	return due
}

// CalculateNextRun calculates the next run time for a job
func (js *JobStore) CalculateNextRun(job *Job) int64 {
	now := time.Now()

	switch job.Schedule.Kind {
	case ScheduleKindAt:
		if job.Schedule.At == "" {
			return 0
		}
		t, err := time.Parse(time.RFC3339, job.Schedule.At)
		if err != nil {
			return 0
		}
		return t.UnixMilli()

	case ScheduleKindEvery:
		if job.Schedule.EveryMs <= 0 {
			return 0
		}
		return now.Add(time.Duration(job.Schedule.EveryMs) * time.Millisecond).UnixMilli()

	case ScheduleKindCron:
		// Simple cron calculation (for production, use a proper cron library)
		// For now, return a simple estimate
		return now.Add(1 * time.Hour).UnixMilli()

	default:
		return 0
	}
}

// CronHandler manages the cron system
type CronHandler struct {
	store     *JobStore
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
	interval  time.Duration
	// Callbacks
	onSystemEvent func(string) // (message)
	onAgentTurn   func(string, string, string) (string, error) // (message, model, thinking)
	onBroadcast  func(string, string, string) error // (message, channel, target)
}

// NewCronHandler creates a new cron handler
func NewCronHandler(storePath string) *CronHandler {
	return &CronHandler{
		store:    NewJobStore(storePath),
		stopCh:   make(chan struct{}),
		interval: 1 * time.Second,
	}
}

// SetSystemEventCallback sets the callback for system events
func (c *CronHandler) SetSystemEventCallback(cb func(string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onSystemEvent = cb
}

// SetAgentTurnCallback sets the callback for agent turns
func (c *CronHandler) SetAgentTurnCallback(cb func(string, string, string) (string, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onAgentTurn = cb
}

// SetBroadcastCallback sets the callback for broadcasting
func (c *CronHandler) SetBroadcastCallback(cb func(string, string, string) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onBroadcast = cb
}

// Start starts the cron scheduler
func (c *CronHandler) Start() {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.stopCh = make(chan struct{})
	c.mu.Unlock()

	log.Printf("[Cron] Starting cron scheduler")

	// Calculate initial next run times
	for _, job := range c.store.List() {
		nextRun := c.store.CalculateNextRun(job)
		job.State.NextRunAtMs = nextRun
	}
	c.store.save()

	go c.runLoop()
}

// Stop stops the cron scheduler
func (c *CronHandler) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	close(c.stopCh)
	c.mu.Unlock()

	log.Printf("[Cron] Stopped cron scheduler")
}

// IsRunning returns whether the cron is running
func (c *CronHandler) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// runLoop runs the main cron loop
func (c *CronHandler) runLoop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.tick()
		}
	}
}

// tick performs one cron check
func (c *CronHandler) tick() {
	dueJobs := c.store.GetDueJobs()

	for _, job := range dueJobs {
		c.executeJob(job)
	}
}

// executeJob runs a single job
func (c *CronHandler) executeJob(job *Job) {
	log.Printf("[Cron] Executing job: %s (%s)", job.Name, job.ID)

	startTime := time.Now()
	job.State.LastRunAtMs = startTime.UnixMilli()

	var err error
	var result string

	// Execute based on payload kind
	switch job.Payload.Kind {
	case PayloadKindSystemEvent:
		// Execute in main session
		c.mu.RLock()
		cb := c.onSystemEvent
		c.mu.RUnlock()

		if cb != nil {
			cb(job.Payload.Text)
			result = "System event sent"
		} else {
			result = "No callback configured"
		}

	case PayloadKindAgentTurn:
		// Execute as isolated agent turn
		c.mu.RLock()
		cb := c.onAgentTurn
		c.mu.RUnlock()

		if cb != nil {
			result, err = cb(job.Payload.Message, job.Payload.Model, job.Payload.Thinking)
			if err != nil {
				job.State.ConsecutiveErrors++
			} else {
				job.State.ConsecutiveErrors = 0
			}
		} else {
			err = fmt.Errorf("no callback configured")
		}

		// Handle delivery
		if job.Delivery != nil && job.Delivery.Mode == DeliveryModeAnnounce {
			c.mu.RLock()
			broadcastCb := c.onBroadcast
			c.mu.RUnlock()

			if broadcastCb != nil && result != "" {
				broadcastCb(result, job.Delivery.Channel, job.Delivery.To)
			}
		}

	default:
		err = fmt.Errorf("unknown payload kind: %s", job.Payload.Kind)
	}

	// Update job state
	job.State.LastDurationMs = time.Since(startTime).Milliseconds()

	if err != nil {
		job.State.LastStatus = "error"
		log.Printf("[Cron] Job error: %s - %v", job.Name, err)
	} else {
		job.State.LastStatus = "ok"
		log.Printf("[Cron] Job completed: %s", job.Name)
	}

	// Calculate next run
	job.State.NextRunAtMs = c.store.CalculateNextRun(job)

	// Handle one-shot jobs
	if job.Schedule.Kind == ScheduleKindAt && job.DeleteAfterRun {
		if job.State.LastStatus == "ok" || job.State.LastStatus == "error" {
			job.Enabled = false
		}
	}

	c.store.save()
}

// AddJob adds a new job
func (c *CronHandler) AddJob(job *Job) error {
	job.ID = generateJobID()
	job.CreatedAt = time.Now()
	job.UpdatedAt = time.Now()
	job.State.NextRunAtMs = c.store.CalculateNextRun(job)

	return c.store.Add(job)
}

// ListJobs returns all jobs
func (c *CronHandler) ListJobs() []*Job {
	return c.store.List()
}

// GetJob returns a job by ID
func (c *CronHandler) GetJob(id string) (*Job, bool) {
	return c.store.Get(id)
}

// UpdateJob updates a job
func (c *CronHandler) UpdateJob(id string, updates map[string]interface{}) (*Job, error) {
	job, err := c.store.Update(id, updates)
	if err != nil {
		return nil, err
	}
	job.State.NextRunAtMs = c.store.CalculateNextRun(job)
	c.store.save()
	return job, nil
}

// RemoveJob removes a job
func (c *CronHandler) RemoveJob(id string) error {
	return c.store.Remove(id)
}

// RunJob immediately runs a job
func (c *CronHandler) RunJob(id string) error {
	job, ok := c.store.Get(id)
	if !ok {
		return fmt.Errorf("job not found: %s", id)
	}

	go c.executeJob(job)
	return nil
}

// GetStatus returns the cron status
func (c *CronHandler) GetStatus() map[string]interface{} {
	jobs := c.store.List()

	enabled := 0
	disabled := 0
	dueNow := 0

	for _, job := range jobs {
		if job.Enabled {
			enabled++
		} else {
			disabled++
		}
		if job.Enabled && job.State.NextRunAtMs > 0 && job.State.NextRunAtMs <= time.Now().UnixMilli() {
			dueNow++
		}
	}

	return map[string]interface{}{
		"running":      c.IsRunning(),
		"total_jobs":   len(jobs),
		"enabled":      enabled,
		"disabled":     disabled,
		"due_now":     dueNow,
		"next_check":  time.Now().Add(c.interval).UnixMilli(),
	}
}

// generateJobID generates a unique job ID
func generateJobID() string {
	return fmt.Sprintf("job-%d", time.Now().UnixMilli())
}

// CreateJobFromMap creates a Job from a map (for API calls)
func CreateJobFromMap(data map[string]interface{}) (*Job, error) {
	job := &Job{
		Enabled: true,
	}

	// Basic fields
	if v, ok := data["name"].(string); ok {
		job.Name = v
	}
	if v, ok := data["description"].(string); ok {
		job.Description = v
	}
	if v, ok := data["agentId"].(string); ok {
		job.AgentID = v
	}

	// Schedule
	if sched, ok := data["schedule"].(map[string]interface{}); ok {
		if v, ok := sched["kind"].(string); ok {
			job.Schedule.Kind = v
		}
		if v, ok := sched["at"].(string); ok {
			job.Schedule.At = v
		}
		if v, ok := sched["everyMs"].(float64); ok {
			job.Schedule.EveryMs = int64(v)
		}
		if v, ok := sched["expr"].(string); ok {
			job.Schedule.Expr = v
		}
		if v, ok := sched["tz"].(string); ok {
			job.Schedule.Tz = v
		}
	}

	// Session target
	if v, ok := data["sessionTarget"].(string); ok {
		job.SessionTarget = v
	} else {
		job.SessionTarget = SessionTargetMain
	}

	// Wake mode
	if v, ok := data["wakeMode"].(string); ok {
		job.WakeMode = v
	} else {
		job.WakeMode = WakeModeNow
	}

	// Payload
	if payload, ok := data["payload"].(map[string]interface{}); ok {
		if v, ok := payload["kind"].(string); ok {
			job.Payload.Kind = v
		}
		if v, ok := payload["text"].(string); ok {
			job.Payload.Text = v
		}
		if v, ok := payload["message"].(string); ok {
			job.Payload.Message = v
		}
		if v, ok := payload["model"].(string); ok {
			job.Payload.Model = v
		}
		if v, ok := payload["thinking"].(string); ok {
			job.Payload.Thinking = v
		}
		if v, ok := payload["timeoutSeconds"].(float64); ok {
			job.Payload.TimeoutSeconds = int(v)
		}
	}

	// Delivery
	if delivery, ok := data["delivery"].(map[string]interface{}); ok {
		job.Delivery = &Delivery{}
		if v, ok := delivery["mode"].(string); ok {
			job.Delivery.Mode = v
		}
		if v, ok := delivery["channel"].(string); ok {
			job.Delivery.Channel = v
		}
		if v, ok := delivery["to"].(string); ok {
			job.Delivery.To = v
		}
		if v, ok := delivery["bestEffort"].(bool); ok {
			job.Delivery.BestEffort = v
		}
	}

	// Delete after run
	if v, ok := data["deleteAfterRun"].(bool); ok {
		job.DeleteAfterRun = v
	} else if job.Schedule.Kind == ScheduleKindAt {
		job.DeleteAfterRun = true
	}

	// Validate
	if job.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if job.Schedule.Kind == "" {
		return nil, fmt.Errorf("schedule.kind is required")
	}
	if job.SessionTarget == SessionTargetMain && job.Payload.Kind != PayloadKindSystemEvent {
		job.Payload.Kind = PayloadKindSystemEvent
	}
	if job.SessionTarget == SessionTargetIsolated && job.Payload.Kind != PayloadKindAgentTurn {
		job.Payload.Kind = PayloadKindAgentTurn
	}

	return job, nil
}
