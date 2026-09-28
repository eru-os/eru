package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/eru-os/eru/eru-cache/cache"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// An agent's chat memory is already two tiers, and the working set is the first
// of them.
//
// The store an agent is configured with - in process, or Redis - is the
// short-term tier. What persist_enabled and cache_db_alias send on to the
// database is the long-term tier. A row reaches the long-term tier by being
// handed to SyncToDatabase and no other way, so choosing a tier is a matter of
// what a writer does with a row, not of which store it reaches for.
//
// That makes the working set a question of use rather than of plumbing. The
// transcript - role, prose, message id, timestamp - is small, worth keeping, and
// written to both tiers. What a turn CARRIED - the page the client sent as its
// baseline, the params it was invoked with - is far larger than every line of
// prose in a long conversation put together, and is written to the short-term
// tier alone, under the conversation's own key, with a ttl.
//
// Losing it is what makes the second turn of a conversation stupid: "revert the
// label back to what it was" is unanswerable without the page as it stood before
// the last edit, and the only recourse is to ask the user for something they
// have every right to expect us to remember. Keeping it forever buys a great
// deal of storage and very little recall.
//
// So it lives exactly as long as the conversation is warm, in whatever store the
// agent was already configured with. Pointing every instance at one Redis is the
// same configuration change it has always been.

// SessionTurn is one turn's working set.
type SessionTurn struct {
	MessageId string                 `json:"message_id,omitempty"`
	Role      string                 `json:"role,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`

	// Code is the artifact this turn carried, kept only for the most recent
	// turns. See sessionFullArtifactTurns.
	Code string `json:"code,omitempty"`

	// Diff is what changed between the previous client baseline and this one,
	// computed once when the turn is recorded.
	//
	// It is the only thing anything actually reads the artifacts for. Keeping
	// the diff rather than re-deriving it means an old turn no longer has to
	// keep a whole page alive to stay useful, and a page is not walked again on
	// every request to produce an answer that cannot have changed.
	Diff []JournalEntry `json:"diff,omitempty"`

	RecordedAt time.Time `json:"recorded_at"`
}

// SessionState is everything a warm conversation remembers beyond its transcript.
type SessionState struct {
	ConversationId string        `json:"conversation_id"`
	Turns          []SessionTurn `json:"turns"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// LatestCode is the artifact the most recent turn carried.
func (s *SessionState) LatestCode() (string, bool) {
	for i := len(s.Turns) - 1; i >= 0; i-- {
		if s.Turns[i].Code != "" {
			return s.Turns[i].Code, true
		}
	}
	return "", false
}

// PreviousCode is the artifact as it stood one turn before the latest - what
// "put it back the way it was" refers to.
func (s *SessionState) PreviousCode() (string, bool) {
	seen := 0
	for i := len(s.Turns) - 1; i >= 0; i-- {
		if s.Turns[i].Code == "" {
			continue
		}
		seen++
		if seen == 2 {
			return s.Turns[i].Code, true
		}
	}
	return "", false
}

// LatestParams are the params the most recent turn was invoked with.
func (s *SessionState) LatestParams() map[string]interface{} {
	for i := len(s.Turns) - 1; i >= 0; i-- {
		if len(s.Turns[i].Params) > 0 {
			return s.Turns[i].Params
		}
	}
	return nil
}

// sessionKeyPrefix separates working-set keys from the transcript keys that
// share the store. A transcript is stored under the conversation id itself.
const sessionKeyPrefix = "session:"

// sessionMaxTurns bounds how far back the working set reaches. Six turns is
// three exchanges - far enough back for "no, the one before that", short of
// holding a whole afternoon of pages.
const sessionMaxTurns = 6

// sessionFullArtifactTurns is how many turns keep their artifact in full.
//
// An artifact is worth keeping for two reasons: to diff the next one against it,
// which needs only the turn before this one, and to hand back whole, which needs
// only the latest. Every turn older than that was keeping a page alive to
// support a diff that had already been taken. Two is what the work needs; the
// rest of the history stays as diffs, which are a few lines each.
const sessionFullArtifactTurns = 2

func sessionKey(conversationId string) string {
	return sessionKeyPrefix + conversationId
}

// sessionTtl is the short-term lifetime the store was configured with.
//
// The expiry is on the conversation, not on any one message: the whole working
// set is a single key rewritten on every touch, so a conversation someone is
// still using never expires and one abandoned an hour ago goes as a unit.
func sessionTtl(ctx context.Context, store cache.CacheStoreI) time.Duration {
	if store == nil {
		return cache.DefaultSessionTtl
	}
	ttlI, err := store.GetAttribute(ctx, "session_ttl")
	if err != nil {
		return cache.DefaultSessionTtl
	}
	if ttl, ok := ttlI.(time.Duration); ok && ttl > 0 {
		return ttl
	}
	return cache.DefaultSessionTtl
}

// sessionWrites serialises the read-modify-write of one conversation's working
// set within this process.
var sessionWrites sync.Mutex

// sessionStateOf returns the working set of a conversation, refreshing its
// expiry. Reading counts as using it: a conversation someone is still working
// through is warm.
func sessionStateOf(ctx context.Context, store cache.CacheStoreI, conversationId string) *SessionState {
	state := &SessionState{ConversationId: conversationId}
	if store == nil || conversationId == "" {
		return state
	}
	raw, err := store.Get(ctx, sessionKey(conversationId))
	if err != nil || raw == "" {
		return state
	}
	if err := json.Unmarshal([]byte(raw), state); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to unmarshal session state for %s: %v", conversationId, err))
		return &SessionState{ConversationId: conversationId}
	}
	_ = store.SetWithTTL(ctx, sessionKey(conversationId), raw, sessionTtl(ctx, store))
	return state
}

// recordSessionTurn adds a turn to the working set, dropping the oldest once the
// conversation is at its ceiling.
//
// Nothing here is handed to SyncToDatabase, which is the whole of what keeps it
// out of the long-term tier.
func recordSessionTurn(ctx context.Context, store cache.CacheStoreI, conversationId string, turn SessionTurn) {
	if store == nil || conversationId == "" {
		return
	}
	if turn.Code == "" && len(turn.Params) == 0 {
		return
	}
	if turn.RecordedAt.IsZero() {
		turn.RecordedAt = time.Now()
	}

	sessionWrites.Lock()
	defer sessionWrites.Unlock()

	state := sessionStateOf(ctx, store, conversationId)
	state.ConversationId = conversationId

	// The same turn is offered more than once - once as the request comes in and
	// again as it is saved - and recording it twice puts a pair of identical
	// artifacts in the middle of the history, which reads as a turn that changed
	// nothing and pushes a real one off the end of the ceiling.
	if last := len(state.Turns) - 1; last >= 0 {
		prev := state.Turns[last]
		if prev.MessageId == turn.MessageId && prev.Role == turn.Role && prev.Code == turn.Code {
			state.Turns[last].Params = turn.Params
			turn = state.Turns[last]
			state.Turns = state.Turns[:last]
		}
	}

	// Diff against the baseline before this one while that artifact is still in
	// hand. After the prune below it may be gone, and this is the last chance.
	if isClientBaseline(turn) && turn.Code != "" {
		if previous, ok := latestBaselineCode(state.Turns); ok {
			if entries, changed := DiffArtifacts(previous, turn.Code); changed {
				turn.Diff = entries
			}
		}
	}

	state.Turns = append(state.Turns, turn)
	if len(state.Turns) > sessionMaxTurns {
		state.Turns = state.Turns[len(state.Turns)-sessionMaxTurns:]
	}
	pruneArtifacts(state.Turns)
	state.UpdatedAt = time.Now()

	body, err := json.Marshal(state)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to marshal session state for %s: %v", conversationId, err))
		return
	}
	ttl := sessionTtl(ctx, store)
	if err := store.SetWithTTL(ctx, sessionKey(conversationId), string(body), ttl); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to write session state for %s: %v", conversationId, err))
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("working set: conversation %s now holds %d turn(s), ttl %s", conversationId, len(state.Turns), ttl))
}

// forgetSession drops a conversation's working set.
func forgetSession(ctx context.Context, store cache.CacheStoreI, conversationId string) {
	if store == nil || conversationId == "" {
		return
	}
	_ = store.Delete(ctx, sessionKey(conversationId))
}

// isClientBaseline reports a turn carrying the artifact as the client sent it.
// The agent's own answer is a different shape - an answer envelope rather than
// the bare artifact - and the two must not be diffed against each other.
func isClientBaseline(turn SessionTurn) bool {
	return turn.Role == "" || turn.Role == "user"
}

// latestBaselineCode is the most recent client baseline still held in full.
func latestBaselineCode(turns []SessionTurn) (string, bool) {
	for i := len(turns) - 1; i >= 0; i-- {
		if isClientBaseline(turns[i]) && turns[i].Code != "" {
			return turns[i].Code, true
		}
	}
	return "", false
}

// pruneArtifacts drops the artifacts that have already given up what they were
// kept for, leaving their diffs behind.
func pruneArtifacts(turns []SessionTurn) {
	kept := 0
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Code == "" {
			continue
		}
		if kept < sessionFullArtifactTurns {
			kept++
			continue
		}
		turns[i].Code = ""
	}
}
