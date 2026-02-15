package rpcproto

// Shared RPC types between gateway and agent.

type Message struct {
	Role                 string       `json:"role"`
	Content              string       `json:"content"`
	ToolCalls            []ToolCall   `json:"tool_calls,omitempty"`
	ToolExecutionResults []ToolResult `json:"tool_results,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ToolResult struct {
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	Result interface{} `json:"result"`
}

type ChatArgs struct {
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

type ChatReply struct {
	Content string     `json:"content"`
	Tools   []ToolCall `json:"tools,omitempty"`
}

type Tool struct {
	Type       string                 `json:"type"`
	Function   ToolFunction           `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type StatsReply struct {
	Stats map[string]int `json:"stats"`
}

type MemorySearchArgs struct {
	Query    string  `json:"query"`
	Category string  `json:"category,omitempty"`
	Limit    int     `json:"limit,omitempty"`
	MinScore float64 `json:"minScore,omitempty"`
}

type MemoryGetArgs struct {
	Path string `json:"path"`
}

type MemoryStoreArgs struct {
	Text       string  `json:"text"`
	Category   string  `json:"category,omitempty"`
	Importance float64 `json:"importance,omitempty"`
}

type ToolResultReply struct {
	Result string `json:"result"`
}
