package agents

import (
	"context"
	"strings"
)

const (
	HeaderAgentChain     = "Eru-Agent-Chain"
	HeaderStreamId       = "Eru-Stream-Id"
	HeaderStreamCallback = "Eru-Stream-Callback"
)

// MaxAgentChainDepth bounds how deep agent-to-agent delegation may nest. A cycle
// (A delegates to B, B delegates back to A for a different sub-task) is legal, so
// this is a runaway brake rather than a cycle check: without it a mis-planned loop
// holds a request goroutine and an HTTP connection per level until timeouts cascade.
const MaxAgentChainDepth = 8

const agentChainSep = "/"

type callChainKey struct{}
type streamTargetKey struct{}

// StreamTarget addresses the SSE connection that started this call. It travels
// down every delegation hop by header so a sub-agent, running in its own request
// on any pod, can post its events back to the pod holding the client's stream.
type StreamTarget struct {
	StreamId    string
	CallbackUrl string
}

func WithAgentChain(ctx context.Context, chain []string) context.Context {
	return context.WithValue(ctx, callChainKey{}, chain)
}

func AgentChain(ctx context.Context) []string {
	if chain, ok := ctx.Value(callChainKey{}).([]string); ok {
		return chain
	}
	return nil
}

func ParseAgentChain(header string) []string {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	var chain []string
	for _, part := range strings.Split(header, agentChainSep) {
		if part = strings.TrimSpace(part); part != "" {
			chain = append(chain, part)
		}
	}
	return chain
}

func FormatAgentChain(chain []string) string {
	return strings.Join(chain, agentChainSep)
}

// ChainWith returns the chain a sub-agent invoked by agentName should receive.
func ChainWith(chain []string, agentName string) []string {
	extended := make([]string, 0, len(chain)+1)
	extended = append(extended, chain...)
	return append(extended, agentName)
}

func WithStreamTarget(ctx context.Context, target StreamTarget) context.Context {
	return context.WithValue(ctx, streamTargetKey{}, target)
}

func GetStreamTarget(ctx context.Context) (StreamTarget, bool) {
	target, ok := ctx.Value(streamTargetKey{}).(StreamTarget)
	if !ok || target.StreamId == "" || target.CallbackUrl == "" {
		return StreamTarget{}, false
	}
	return target, true
}
