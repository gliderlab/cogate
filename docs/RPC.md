# RPC Protocol

RPC communication protocol between Gateway and Agent.

## Overview

```
Gateway (Unix Socket RPC)  ←→  Agent
   /tmp/ocg-agent.sock
```

## Connection

```go
client, err := rpc.Dial("unix", "/tmp/ocg-agent.sock")
```

## Data Types

### Message

```go
type Message struct {
    Role                 string       // "user" | "assistant" | "system" | "tool"
    Content              string       // message content
    ToolCalls            []ToolCall   // tool calls from LLM
    ToolExecutionResults []ToolResult // tool execution results
}
```

### ToolCall

```go
type ToolCall struct {
    ID   string // tool call ID
    Type string // "function"
    Function struct {
        Name      string // tool name
        Arguments string // JSON arguments
    }
}
```

### ToolResult

```go
type ToolResult struct {
    ID     string      // corresponds to ToolCall.ID
    Type   string      // "function"
    Result interface{} // execution result
}
```

### ChatArgs

```go
type ChatArgs struct {
    Messages []Message // conversation history
    Tools    []Tool    // available tool descriptions (optional)
}
```

### ChatReply

```go
type ChatReply struct {
    Content string     // LLM response
    Tools   []ToolCall // subsequent tool calls (optional)
}
```

## RPC Methods

### Chat

Process chat request, return LLM response or tool calls.

```go
func (s *RPCService) Chat(args ChatArgs, reply *ChatReply) error
```

**Example:**

```go
args := rpcproto.ChatArgs{
    Messages: []rpcproto.Message{
        {Role: "user", Content: "Hello"},
    },
}
var reply rpcproto.ChatReply
client.Call("RPCService.Chat", args, &reply)
fmt.Println(reply.Content)
```

### Stats

Get storage statistics.

```go
func (s *RPCService) Stats(_ struct{}, reply *StatsReply) error
```

**Returns:**

```json
{
    "sessions": 10,
    "messages": 150,
    "tools_calls": 45
}
```

### MemorySearch

Vector memory search.

```go
func (s *RPCService) MemorySearch(args MemorySearchArgs, reply *ToolResultReply) error
```

**Parameters:**

```go
type MemorySearchArgs struct {
    Query    string  // search text
    Category string  // category filter (optional)
    Limit    int     // result count (default 5)
    MinScore float64 // minimum similarity (default 0.7)
}
```

### MemoryGet

Get a single memory entry.

```go
func (s *RPCService) MemoryGet(args MemoryGetArgs, reply *ToolResultReply) error
```

### MemoryStore

Store a new memory.

```go
func (s *RPCService) MemoryStore(args MemoryStoreArgs, reply *ToolResultReply) error
```

**Parameters:**

```go
type MemoryStoreArgs struct {
    Text       string  // memory content
    Category   string  // category (preference/decision/fact/entity/other)
    Importance float64 // importance (0-1)
}
```

## Tool Call Flow

```
1. Gateway → Agent.RPC.Chat(ChatArgs)
2. Agent runs LLM, returns ToolCall or Content
3. Gateway executes tools
4. Gateway → Agent.RPC.Chat(with ToolExecutionResults)
5. Agent continues LLM to generate final response
```

## Error Handling

| Error | Description |
|-------|-------------|
| `agent not initialized` | Agent not started |
| `storage not initialized` | Storage not initialized |
| `memory store not initialized` | Memory store not initialized |
| `timeout waiting for agent` | Agent socket not ready |

## Client Example (Go)

```go
package main

import (
    "fmt"
    "net/rpc"
    "github.com/gliderlab/cogate/rpcproto"
)

func main() {
    client, err := rpc.Dial("unix", "/tmp/ocg-agent.sock")
    if err != nil {
        panic(err)
    }
    defer client.Close()

    args := rpcproto.ChatArgs{
        Messages: []rpcproto.Message{
            {Role: "user", Content: "Hello"},
        },
    }
    var reply rpcproto.ChatReply
    if err := client.Call("RPCService.Chat", args, &reply); err != nil {
        panic(err)
    }
    fmt.Println("Reply:", reply.Content)
}
```

## Ports and Paths

| Service | Address |
|---------|---------|
| Agent RPC Socket | `/tmp/ocg-agent.sock` |
| Gateway HTTP | `http://localhost:55003` |
| Embedding | `http://localhost:50001` |
