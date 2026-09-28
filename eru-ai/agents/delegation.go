package agents

import (
	"context"
	"fmt"
	"strings"
)

// How deep delegation may go, and what stops it going round in circles.
//
// An orchestrator calls sub-agents, and nothing said a sub-agent may not call
// the orchestrator. Nothing said an agent may not call itself. Nothing bounded
// how many layers there could be. None of that had ever happened, which is a
// statement about the two agents that exist rather than about the framework -
// and the failure mode is not a bad answer, it is a service that goes on calling
// models until something else runs out.
//
// A depth limit and a cycle check cost a context value each and turn an
// unbounded recursion into a clear refusal naming the chain that caused it.

// DefaultMaxDelegationDepth is how many layers of agent are allowed by default.
// Three is enough for an orchestrator that delegates to a specialist which
// consults a helper, and short enough that a mistake is caught in one call
// rather than a hundred.
const DefaultMaxDelegationDepth = 3

type delegationKey struct{}

// delegationChain is the agents currently on the stack, outermost first.
type delegationChain []string

// EnterAgent records that an agent is starting, and refuses if doing so would
// go too deep or revisit an agent already running.
//
// The refusal is deliberately not "too deep" alone. A cycle at depth two is a
// different mistake from honest nesting at depth four, and the message names the
// chain because that is the only thing that makes either fixable.
func EnterAgent(ctx context.Context, agentName string, maxDepth int) (context.Context, error) {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDelegationDepth
	}
	chain, _ := ctx.Value(delegationKey{}).(delegationChain)

	for _, running := range chain {
		if strings.EqualFold(running, agentName) {
			return ctx, fmt.Errorf(
				"agent %q is already running further up this chain (%s), so calling it again would not terminate",
				agentName, strings.Join(append(chain, agentName), " -> "))
		}
	}
	if len(chain) >= maxDepth {
		return ctx, fmt.Errorf(
			"delegation is %d deep, which is the limit (%s -> %s); an agent this far from the request is usually a loop rather than a plan",
			len(chain), strings.Join(chain, " -> "), agentName)
	}
	return context.WithValue(ctx, delegationKey{}, append(append(delegationChain{}, chain...), agentName)), nil
}

// DelegationChain is who is currently running, outermost first.
func DelegationChain(ctx context.Context) []string {
	chain, _ := ctx.Value(delegationKey{}).(delegationChain)
	return append([]string{}, chain...)
}

// DelegationDepth is how many agents deep the current call is.
func DelegationDepth(ctx context.Context) int {
	chain, _ := ctx.Value(delegationKey{}).(delegationChain)
	return len(chain)
}
