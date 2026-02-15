package channels

import (
	"net/rpc"

	"github.com/gliderlab/cogate/rpcproto"
)

// RPCClient implements AgentRPCInterface using RPC calls to the agent
type RPCClient struct {
	client *rpc.Client
}

// NewRPCClient creates a new RPC client for agent communication
func NewRPCClient(client *rpc.Client) *RPCClient {
	return &RPCClient{
		client: client,
	}
}

// Chat sends a chat request to the agent via RPC
func (r *RPCClient) Chat(messages []rpcproto.Message) (string, error) {
	var reply rpcproto.ChatReply
	args := rpcproto.ChatArgs{Messages: messages}
	
	err := r.client.Call("Agent.Chat", args, &reply)
	if err != nil {
		return "", err
	}
	
	return reply.Content, nil
}

// GetStats gets statistics from the agent via RPC
func (r *RPCClient) GetStats() (map[string]int, error) {
	var reply rpcproto.StatsReply
	
	err := r.client.Call("Agent.Stats", struct{}{}, &reply)
	if err != nil {
		return nil, err
	}
	
	return reply.Stats, nil
}