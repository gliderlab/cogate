# RPC Protocol

Gateway 与 Agent 之间的 RPC 通信协议。

## 概览

```
Gateway (Unix Socket RPC)  ←→  Agent
   /tmp/ocg-agent.sock
```

## 连接

```go
client, err := rpc.Dial("unix", "/tmp/ocg-agent.sock")
```

## 数据类型

### Message

```go
type Message struct {
    Role                 string       // "user" | "assistant" | "system" | "tool"
    Content              string       // 消息内容
    ToolCalls            []ToolCall   // LLM 产生的工具调用
    ToolExecutionResults []ToolResult // 工具执行结果
}
```

### ToolCall

```go
type ToolCall struct {
    ID   string // 工具调用 ID
    Type string // "function"
    Function struct {
        Name      string // 工具名称
        Arguments string // JSON 参数
    }
}
```

### ToolResult

```go
type ToolResult struct {
    ID     string      // 对应 ToolCall.ID
    Type   string      // "function"
    Result interface{} // 执行结果
}
```

### ChatArgs

```go
type ChatArgs struct {
    Messages []Message // 对话历史
    Tools    []Tool    // 可用工具描述 (可选)
}
```

### ChatReply

```go
type ChatReply struct {
    Content string     // LLM 回复
    Tools   []ToolCall // 后续工具调用 (可选)
}
```

## RPC 方法

### Chat

处理聊天请求，返回 LLM 回复或工具调用。

```go
func (s *RPCService) Chat(args ChatArgs, reply *ChatReply) error
```

**调用示例：**

```go
args := rpcproto.ChatArgs{
    Messages: []rpcproto.Message{
        {Role: "user", Content: "你好"},
    },
}
var reply rpcproto.ChatReply
client.Call("RPCService.Chat", args, &reply)
fmt.Println(reply.Content)
```

### Stats

获取存储统计信息。

```go
func (s *RPCService) Stats(_ struct{}, reply *StatsReply) error
```

**返回：**

```json
{
    "sessions": 10,
    "messages": 150,
    "tools_calls": 45
}
```

### MemorySearch

向量记忆搜索。

```go
func (s *RPCService) MemorySearch(args MemorySearchArgs, reply *ToolResultReply) error
```

**参数：**

```go
type MemorySearchArgs struct {
    Query    string  // 搜索文本
    Category string  // 类别过滤 (可选)
    Limit    int     // 返回数量 (默认 5)
    MinScore float64 // 最小相似度 (默认 0.7)
}
```

### MemoryGet

获取单条记忆。

```go
func (s *RPCService) MemoryGet(args MemoryGetArgs, reply *ToolResultReply) error
```

### MemoryStore

存储新记忆。

```go
func (s *RPCService) MemoryStore(args MemoryStoreArgs, reply *ToolResultReply) error
```

**参数：**

```go
type MemoryStoreArgs struct {
    Text       string  // 记忆内容
    Category   string  // 类别 (preference/decision/fact/entity/other)
    Importance float64 // 重要程度 (0-1)
}
```

## 工具调用流程

```
1. Gateway → Agent.RPC.Chat(ChatArgs)
2. Agent 执行 LLM，返回 ToolCall 或 Content
3. Gateway 执行工具
4. Gateway → Agent.RPC.Chat(带 ToolExecutionResults)
5. Agent 继续 LLM 生成最终回复
```

## 错误处理

| 错误 | 说明 |
|------|------|
| `agent not initialized` | Agent 未启动 |
| `storage not initialized` | 存储未初始化 |
| `memory store not initialized` | 记忆存储未初始化 |
| `timeout waiting for agent` | Agent socket 未就绪 |

## 客户端示例 (Go)

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
            {Role: "user", Content: "你好"},
        },
    }
    var reply rpcproto.ChatReply
    if err := client.Call("RPCService.Chat", args, &reply); err != nil {
        panic(err)
    }
    fmt.Println("Reply:", reply.Content)
}
```

## 端口与路径

| 服务 | 地址 |
|------|------|
| Agent RPC Socket | `/tmp/ocg-agent.sock` |
| Gateway HTTP | `http://localhost:55003` |
| Embedding | `http://localhost:50001` |
