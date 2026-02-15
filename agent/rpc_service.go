package agent

import (
	"encoding/json"
	"fmt"

	"github.com/gliderlab/cogate/rpcproto"
	"github.com/gliderlab/cogate/tools"
)

type RPCService struct {
	agent *Agent
}

func NewRPCService(a *Agent) *RPCService {
	return &RPCService{agent: a}
}

func (s *RPCService) Chat(args rpcproto.ChatArgs, reply *rpcproto.ChatReply) error {
	if s.agent == nil {
		return fmt.Errorf("agent not initialized")
	}

	msgs := make([]Message, len(args.Messages))
	for i, m := range args.Messages {
		msgs[i] = Message{
			Role:    m.Role,
			Content: m.Content,
		}
		if len(m.ToolCalls) > 0 {
			msgs[i].ToolCalls = make([]ToolCall, len(m.ToolCalls))
			for j, c := range m.ToolCalls {
				msgs[i].ToolCalls[j] = ToolCall{
					ID:   c.ID,
					Type: c.Type,
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      c.Function.Name,
						Arguments: c.Function.Arguments,
					},
				}
			}
		}
		if len(m.ToolExecutionResults) > 0 {
			msgs[i].ToolExecutionResults = make([]ToolResult, len(m.ToolExecutionResults))
			for j, r := range m.ToolExecutionResults {
				msgs[i].ToolExecutionResults[j] = ToolResult{ID: r.ID, Type: r.Type, Result: r.Result}
			}
		}
	}

	reply.Content = s.agent.Chat(msgs)
	return nil
}

func (s *RPCService) Stats(_ struct{}, reply *rpcproto.StatsReply) error {
	if s.agent == nil || s.agent.Store() == nil {
		return fmt.Errorf("storage not initialized")
	}
	stats, err := s.agent.Store().Stats()
	if err != nil {
		return err
	}
	reply.Stats = stats
	return nil
}

func (s *RPCService) MemorySearch(args rpcproto.MemorySearchArgs, reply *rpcproto.ToolResultReply) error {
	if s.agent == nil || s.agent.MemoryStore() == nil {
		return fmt.Errorf("memory store not initialized")
	}

	tool := tools.NewMemoryTool(s.agent.MemoryStore())
	result, err := tool.Execute(map[string]interface{}{
		"query":    args.Query,
		"category": args.Category,
		"limit":    args.Limit,
		"minScore": args.MinScore,
	})
	if err != nil {
		return err
	}
	// Convert to JSON string to support gob serialization
	jsonBytes, _ := json.Marshal(result)
	reply.Result = string(jsonBytes)
	return nil
}

func (s *RPCService) MemoryGet(args rpcproto.MemoryGetArgs, reply *rpcproto.ToolResultReply) error {
	if s.agent == nil || s.agent.MemoryStore() == nil {
		return fmt.Errorf("memory store not initialized")
	}

	tool := tools.NewMemoryGetTool(s.agent.MemoryStore())
	result, err := tool.Execute(map[string]interface{}{"path": args.Path})
	if err != nil {
		return err
	}
	// Convert to JSON string to support gob serialization
	jsonBytes, _ := json.Marshal(result)
	reply.Result = string(jsonBytes)
	return nil
}

func (s *RPCService) MemoryStore(args rpcproto.MemoryStoreArgs, reply *rpcproto.ToolResultReply) error {
	if s.agent == nil || s.agent.MemoryStore() == nil {
		return fmt.Errorf("memory store not initialized")
	}

	tool := tools.NewMemoryStoreTool(s.agent.MemoryStore())
	result, err := tool.Execute(map[string]interface{}{
		"text":       args.Text,
		"category":   args.Category,
		"importance": args.Importance,
	})
	if err != nil {
		return err
	}
	// Convert to JSON string to support gob serialization
	jsonBytes, _ := json.Marshal(result)
	reply.Result = string(jsonBytes)
	return nil
}

// PulseArgs represents arguments for pulse operations
type PulseArgs struct {
	Action   string // "add", "status", "list"
	Title    string
	Content  string
	Priority int    // 0-3
	Channel  string
	Limit    int
}

// PulseReply represents the result of pulse operations
type PulseReply struct {
	Result  string
	EventID int64
	Status  string
}

// PulseAdd adds a new pulse event
func (s *RPCService) PulseAdd(args PulseArgs, reply *PulseReply) error {
	if s.agent == nil {
		return fmt.Errorf("agent not initialized")
	}

	eventID, err := s.agent.AddPulseEvent(args.Title, args.Content, args.Priority, args.Channel)
	if err != nil {
		return err
	}

	reply.EventID = eventID
	reply.Result = "Event added successfully"
	reply.Status = "pending"
	return nil
}

// PulseStatus returns the current pulse system status
func (s *RPCService) PulseStatus(args struct{}, reply *PulseReply) error {
	if s.agent == nil {
		return fmt.Errorf("agent not initialized")
	}

	status, err := s.agent.GetPulseStatus()
	if err != nil {
		return err
	}

	data, _ := json.Marshal(status)
	reply.Result = string(data)
	return nil
}
