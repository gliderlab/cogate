// Memory Tool - vector memory tool (FAISS/SQLite)
package tools

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/gliderlab/cogate/memory"
)

// ===================== memory_search =====================

type MemoryTool struct {
	Store *memory.VectorMemoryStore
}

func NewMemoryTool(store *memory.VectorMemoryStore) *MemoryTool {
	return &MemoryTool{Store: store}
}

func (t *MemoryTool) Name() string { return "memory_search" }

func (t *MemoryTool) Description() string {
	return "Search long-term memory (vector search) and return similarity scores."
}

func (t *MemoryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query keywords",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Optional category filter (preference/decision/fact/entity/other)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max results (default 5)",
				"default":     5,
			},
			"minScore": map[string]interface{}{
				"type":        "number",
				"description": "Min similarity 0-1 (default 0.7)",
				"default":     0.7,
			},
		},
		"required": []string{"query"},
	}
}

func (t *MemoryTool) Execute(args map[string]interface{}) (interface{}, error) {
	query := GetString(args, "query")
	category := GetString(args, "category")
	limit := GetInt(args, "limit")
	minScore := GetFloat64(args, "minScore")

	if limit <= 0 {
		limit = 5
	}
	if minScore <= 0 {
		minScore = 0.7
	}

	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if t.Store == nil {
		return nil, fmt.Errorf("memory store is not initialized")
	}

	results, err := t.Store.Search(query, limit, float32(minScore))
	if err != nil {
		return nil, fmt.Errorf("search failed: %v", err)
	}

	// Optional category filter
	if category != "" {
		filtered := make([]memory.MemoryResult, 0)
		for _, r := range results {
			if r.Entry.Category == category {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	if len(results) == 0 {
		return MemorySearchResult{Query: query, Count: 0, Result: "No relevant memories found."}, nil
	}

	resultText := fmt.Sprintf("Found %d related memories (similarity):\n\n", len(results))
	items := make([]map[string]interface{}, 0, len(results))
	for i, r := range results {
		scorePct := int(r.Score * 100)
		resultText += fmt.Sprintf("%d. [%s] %s (similarity %d%%)\n", i+1, r.Entry.Category, r.Entry.Text, scorePct)
		items = append(items, map[string]interface{}{
			"id":         r.Entry.ID,
			"text":       r.Entry.Text,
			"category":   r.Entry.Category,
			"importance": r.Entry.Importance,
			"score":      fmt.Sprintf("%.4f", r.Score),
			"matched":    r.Matched,
			"source":     r.Entry.Source,
			"createdAt":  time.Unix(r.Entry.CreatedAt, 0).Format("2006-01-02 15:04"),
			"updatedAt":  time.Unix(r.Entry.UpdatedAt, 0).Format("2006-01-02 15:04"),
		})
	}

	return MemorySearchResult{Query: query, Count: len(results), Items: items, Result: resultText}, nil
}

// ===================== memory_get =====================

type MemoryGetTool struct {
	Store *memory.VectorMemoryStore
}

func NewMemoryGetTool(store *memory.VectorMemoryStore) *MemoryGetTool {
	return &MemoryGetTool{Store: store}
}

func (t *MemoryGetTool) Name() string { return "memory_get" }

func (t *MemoryGetTool) Description() string {
	return "Get details of a single memory."
}

func (t *MemoryGetTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Memory ID",
			},
		},
		"required": []string{"path"},
	}
}

func (t *MemoryGetTool) Execute(args map[string]interface{}) (interface{}, error) {
	id := GetString(args, "path")
	if id == "" {
		return nil, fmt.Errorf("path is required")
	}
	if t.Store == nil {
		return nil, fmt.Errorf("memory store is not initialized")
	}

	entry, err := t.Store.Get(id)
	if err != nil {
		return nil, fmt.Errorf("memory not found or failed to fetch: %v", err)
	}

	return map[string]interface{}{
		"id":         entry.ID,
		"text":       entry.Text,
		"category":   entry.Category,
		"importance": entry.Importance,
		"source":     entry.Source,
		"createdAt":  time.Unix(entry.CreatedAt, 0).Format("2006-01-02 15:04:05"),
		"updatedAt":  time.Unix(entry.UpdatedAt, 0).Format("2006-01-02 15:04:05"),
	}, nil
}

// ===================== memory_store =====================

type MemoryStoreTool struct {
	Store *memory.VectorMemoryStore
}

func NewMemoryStoreTool(store *memory.VectorMemoryStore) *MemoryStoreTool {
	return &MemoryStoreTool{Store: store}
}

func (t *MemoryStoreTool) Name() string { return "memory_store" }

func (t *MemoryStoreTool) Description() string {
	return "Store important info into long-term memory (vector store)."
}

func (t *MemoryStoreTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"text": map[string]interface{}{
				"type":        "string",
				"description": "Content to memorize",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Category: preference/decision/fact/entity/other",
				"default":     "other",
			},
			"importance": map[string]interface{}{
				"type":        "number",
				"description": "Importance 0-1",
				"default":     0.7,
			},
		},
		"required": []string{"text"},
	}
}

func (t *MemoryStoreTool) Execute(args map[string]interface{}) (interface{}, error) {
	text := GetString(args, "text")
	category := GetString(args, "category")
	importance := GetFloat64(args, "importance")
	if category == "" {
		category = "other"
	}
	if importance <= 0 {
		importance = 0.7
	}
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if t.Store == nil {
		return nil, fmt.Errorf("memory store is not initialized")
	}

	// Approximate duplicate detection (similarity > 0.95)
	results, _ := t.Store.Search(text, 3, 0.95)
	for _, r := range results {
		if strings.TrimSpace(r.Entry.Text) == strings.TrimSpace(text) {
			return map[string]interface{}{
				"action": "duplicate",
				"id":     r.Entry.ID,
				"result": "Similar memory already exists",
			}, nil
		}
	}

	id, err := t.Store.Store(text, category, importance)
	if err != nil {
		return nil, fmt.Errorf("store failed: %v", err)
	}

	log.Printf("✅ memory stored: %s", id)
	return map[string]interface{}{
		"action": "created",
		"id":     id,
		"result": fmt.Sprintf("Stored: %s", Truncate(text, 50)),
	}, nil
}

// ===================== Helpers =====================

type MemorySearchResult struct {
	Query  string                   `json:"query"`
	Count  int                      `json:"count"`
	Items  []map[string]interface{} `json:"items,omitempty"`
	Result string                   `json:"result"`
}

// Memory capture rules (aligned with OpenClaw)
var captureTriggers = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(zapamatuj|pamatuj|remember)`),
	regexp.MustCompile(`(?i)(preferuji|radši|prefer)`),
	regexp.MustCompile(`(?i)(rozhodli jsme|budeme používat|decided|will use)`),
	regexp.MustCompile(`(?i)(můj\s+\w+\s+je|my\s+\w+\s+is|is\s+my)`),
	regexp.MustCompile(`(?i)(i\s+(like|prefer|hate|love|want|need))`),
	regexp.MustCompile(`(?i)(always|never|important)`),
}

func ShouldCapture(text string) bool {
	if len(text) < 10 || len(text) > 500 {
		return false
	}
	if strings.Contains(text, "<relevant-memories>") {
		return false
	}
	if strings.HasPrefix(text, "<") && strings.Contains(text, "</") {
		return false
	}
	emojiCount := len(regexp.MustCompile(`[\x{1F300}-\x{1F9FF}]`).FindAllString(text, -1))
	if emojiCount > 3 {
		return false
	}
	for _, r := range captureTriggers {
		if r.MatchString(text) {
			return true
		}
	}
	return false
}

func DetectCategory(text string) string {
	return memory.DetectCategory(text)
}

// ===================== Auto Recall Helpers =====================

// FindRelevantMemories to find relevant memories (auto recall)
func FindRelevantMemories(store *memory.VectorMemoryStore, prompt string, limit int) ([]memory.MemoryResult, error) {
	if store == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}

	// Simple keyword extraction
	keywords := extractKeywords(prompt)
	seen := make(map[string]bool)
	var results []memory.MemoryResult

	for _, kw := range keywords {
		if len(kw) < 3 {
			continue
		}
		res, err := store.Search(kw, limit, 0.7)
		if err != nil {
			continue
		}
		for _, r := range res {
			if !seen[r.Entry.ID] {
				seen[r.Entry.ID] = true
				results = append(results, r)
			}
		}
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// Format memories for context injection
func FormatMemoriesForContext(results []memory.MemoryResult) string {
	if len(results) == 0 {
		return ""
	}
	lines := make([]string, 0, len(results))
	for _, r := range results {
		lines = append(lines, fmt.Sprintf("- [%s] %s", r.Entry.Category, r.Entry.Text))
	}
	return fmt.Sprintf("<relevant-memories>\nThe following memories may be relevant to the current conversation:\n%s\n</relevant-memories>", strings.Join(lines, "\n"))
}

// Keyword extraction (very simple)
func extractKeywords(prompt string) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true,
		"have": true, "has": true, "had": true, "do": true,
		"does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true,
		"must": true, "shall": true, "can": true, "need": true,
		"i": true, "you": true, "he": true, "she": true, "it": true,
		"we": true, "they": true, "me": true, "him": true, "her": true,
		"us": true, "them": true, "my": true, "your": true, "his": true,
		"our": true, "their": true, "what": true, "which": true,
		"who": true, "whom": true, "this": true, "that": true,
		"these": true, "those": true, "and": true, "but": true,
		"or": true, "nor": true, "so": true, "yet": true, "not": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "at": true, "by": true, "from": true, "up": true,
		"about": true, "into": true, "through": true, "during": true,
		"before": true, "after": true, "above": true, "below": true,
		"between": true, "under": true, "again": true, "further": true,
		"then": true, "once": true, "here": true, "there": true,
		"when": true, "where": true, "why": true, "how": true, "all": true,
		"any": true, "both": true, "each": true, "few": true, "more": true,
		"most": true, "other": true, "some": true, "such": true, "no": true,
		"only": true, "own": true, "same": true, "than": true,
		"too": true, "very": true, "just": true, "also": true, "now": true,
	}

	words := strings.Fields(prompt)
	var keywords []string
	for _, w := range words {
		clean := strings.Trim(strings.ToLower(w), ".,!?;:\"'()[]{}")
		if len(clean) >= 3 && !stopWords[clean] {
			keywords = append(keywords, clean)
		}
	}
	return keywords
}
