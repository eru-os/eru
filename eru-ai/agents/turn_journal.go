package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// A conversation about an artifact needs more than the prose of what was asked.
//
// "Change the label to xxx" followed by "revert it back to what it was" is two
// perfectly clear instructions, and the second one is unanswerable from the
// transcript alone: the transcript records that a label was changed, not what it
// was changed from. The agent is then reduced to either asking the user for a
// value it was itself handed a moment ago, or guessing.
//
// The journal closes that gap without carrying whole revisions into the prompt.
// Two consecutive versions of the artifact are compared leaf by leaf and the
// result is a handful of lines - the path, the old value, the new value. A page
// of JSON is tens of thousands of tokens; the edit that produced it is usually
// three lines. Those three lines are what "revert", "undo", "the one before" and
// "put the old colour back" all resolve against.

// maxJournalEntries caps one turn's journal. An edit that touched more than this
// was a rewrite, not an edit, and listing all of it would be as long as sending
// the artifact.
const maxJournalEntries = 40

// maxJournalScan bounds the walk itself, which is a different job from bounding
// the report. Stopping the walk at the size of the report means the leaves are
// taken in the order the document happens to be laid out, and the edit the user
// is about to refer back to is simply whichever one it was - a title near the
// end of a page never gets looked at. The walk therefore collects everything up
// to a ceiling that only a pathological document reaches, and the selection of
// what is worth reporting happens afterwards, on the whole set.
const maxJournalScan = 5000

// JournalEntry is one leaf that differed between two versions of an artifact.
type JournalEntry struct {
	Path string
	From string
	To   string
}

func (e JournalEntry) String() string {
	switch {
	case e.From == "":
		return fmt.Sprintf("%s: added %s", e.Path, e.To)
	case e.To == "":
		return fmt.Sprintf("%s: removed (was %s)", e.Path, e.From)
	default:
		return fmt.Sprintf("%s: %s -> %s", e.Path, e.From, e.To)
	}
}

// DiffArtifacts reports the leaves that differ between two JSON documents.
//
// The second return value is false when the documents cannot be compared - not
// JSON, or nothing changed - so a caller can tell "no journal" from "no changes".
func DiffArtifacts(before, after string) ([]JournalEntry, bool) {
	if strings.TrimSpace(before) == "" || strings.TrimSpace(after) == "" {
		return nil, false
	}
	var b, a interface{}
	if err := json.Unmarshal([]byte(before), &b); err != nil {
		return nil, false
	}
	if err := json.Unmarshal([]byte(after), &a); err != nil {
		return nil, false
	}
	var entries []JournalEntry
	diffValue("", b, a, &entries)

	entries = rank(entries)
	if len(entries) == 0 {
		return nil, false
	}
	if len(entries) > maxJournalEntries {
		entries = entries[:maxJournalEntries]
	}
	return entries, true
}

// rank puts the changes worth remembering first and drops the ones that are not
// changes at all.
//
// Two versions of a page that went through the client between turns differ in
// more than the edit: the canvas fills in defaults it did not have before, an
// empty events list here, a blank name there, a fresh updated_at. Listed
// alongside the one real edit and then cut off at a fixed ceiling, that
// bookkeeping is perfectly capable of pushing the edit off the end of the list -
// which leaves a journal that says a great deal and answers nothing.
//
// A leaf that went from one real value to another is the only kind "revert it"
// can be about, so those are kept and listed first. A value that merely appeared
// or vanished is noise until proven otherwise, and an empty one always is.
func rank(entries []JournalEntry) []JournalEntry {
	var changed, appeared []JournalEntry
	for _, e := range entries {
		if isBookkeeping(e.Path) {
			continue
		}
		switch {
		case e.From != "" && e.To != "":
			changed = append(changed, e)
		case isEmptyValue(e.From) && isEmptyValue(e.To):
			// nothing either way
		default:
			appeared = append(appeared, e)
		}
	}
	sort.SliceStable(changed, func(i, j int) bool { return changed[i].Path < changed[j].Path })
	sort.SliceStable(appeared, func(i, j int) bool { return appeared[i].Path < appeared[j].Path })
	return append(changed, appeared...)
}

// isBookkeeping reports leaves the client maintains for itself, which change on
// their own and mean nothing to a user asking for something to be put back.
func isBookkeeping(path string) bool {
	for _, suffix := range []string{"created_at", "updated_at", "revision", "base_revision"} {
		if strings.HasSuffix(path, "."+suffix) || path == suffix {
			return true
		}
	}
	return false
}

func isEmptyValue(rendered string) bool {
	switch rendered {
	case "", `""`, "[]", "{}", "[...]", "null":
		return true
	}
	return false
}

func diffValue(path string, before, after interface{}, out *[]JournalEntry) {
	if len(*out) > maxJournalScan {
		return
	}
	switch b := before.(type) {
	case map[string]interface{}:
		a, ok := after.(map[string]interface{})
		if !ok {
			record(path, before, after, out)
			return
		}
		keys := map[string]struct{}{}
		for k := range b {
			keys[k] = struct{}{}
		}
		for k := range a {
			keys[k] = struct{}{}
		}
		ordered := make([]string, 0, len(keys))
		for k := range keys {
			ordered = append(ordered, k)
		}
		sort.Strings(ordered)
		for _, k := range ordered {
			diffValue(join(path, k), b[k], a[k], out)
		}
	case []interface{}:
		a, ok := after.([]interface{})
		if !ok {
			record(path, before, after, out)
			return
		}
		// Arrays are compared by identity where the elements carry one, so
		// inserting a component at the top of a page does not report every
		// component below it as changed.
		bIds, bOk := keyedByIdentity(b)
		aIds, aOk := keyedByIdentity(a)
		if bOk && aOk {
			ids := map[string]struct{}{}
			for id := range bIds {
				ids[id] = struct{}{}
			}
			for id := range aIds {
				ids[id] = struct{}{}
			}
			ordered := make([]string, 0, len(ids))
			for id := range ids {
				ordered = append(ordered, id)
			}
			sort.Strings(ordered)
			for _, id := range ordered {
				diffValue(fmt.Sprintf("%s[%s]", path, id), bIds[id], aIds[id], out)
			}
			return
		}
		n := len(b)
		if len(a) > n {
			n = len(a)
		}
		for i := 0; i < n; i++ {
			var bv, av interface{}
			if i < len(b) {
				bv = b[i]
			}
			if i < len(a) {
				av = a[i]
			}
			diffValue(fmt.Sprintf("%s[%d]", path, i), bv, av, out)
		}
	default:
		if !sameScalar(before, after) {
			record(path, before, after, out)
		}
	}
}

// keyedByIdentity indexes array elements by their own id, when every element has
// a distinct one.
func keyedByIdentity(items []interface{}) (map[string]interface{}, bool) {
	if len(items) == 0 {
		return nil, false
	}
	byId := make(map[string]interface{}, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			return nil, false
		}
		id, ok := obj["id"].(string)
		if !ok || id == "" {
			return nil, false
		}
		if _, clash := byId[id]; clash {
			return nil, false
		}
		byId[id] = item
	}
	return byId, true
}

func sameScalar(before, after interface{}) bool {
	if before == nil && after == nil {
		return true
	}
	return fmt.Sprintf("%v", before) == fmt.Sprintf("%v", after)
}

func record(path string, before, after interface{}, out *[]JournalEntry) {
	*out = append(*out, JournalEntry{Path: path, From: render(before), To: render(after)})
}

// render keeps a leaf short. A nested object that appeared wholesale is reported
// as having appeared, not reprinted.
func render(v interface{}) string {
	if v == nil {
		return ""
	}
	switch v.(type) {
	case map[string]interface{}:
		return "{...}"
	case []interface{}:
		return "[...]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	s := string(b)
	if len(s) > 80 {
		s = s[:77] + `..."`
	}
	return s
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// JournalNote is the block of prose handed to the model describing what the
// previous turns of this conversation actually changed.
//
// It is what makes a follow-up referential: without it "revert that" and "make
// it the old colour again" have no antecedent, and the agent either asks the
// user to repeat themselves or goes looking through the whole artifact for
// something that might match.
func JournalNote(state *SessionState) string {
	if state == nil {
		return ""
	}

	// Each turn's diff was taken when the turn was recorded, against the
	// baseline before it, so this is a render rather than a comparison: walking
	// forward lists the oldest change first, and "the one before that" counts
	// backwards from the end. Older turns no longer hold their artifact at all -
	// the diff is the part that was ever read.
	var sections []string
	turn := 0
	for _, t := range state.Turns {
		if len(t.Diff) == 0 {
			continue
		}
		turn++
		lines := make([]string, 0, len(t.Diff))
		for _, e := range t.Diff {
			lines = append(lines, "  "+e.String())
		}
		sections = append(sections, fmt.Sprintf("change %d:\n%s", turn, strings.Join(lines, "\n")))
	}

	if len(sections) == 0 {
		return ""
	}

	return "Changes already made to this artifact earlier in this conversation, oldest first. " +
		"Values on the left are what a leaf held before that change, values on the right what it holds now. " +
		"When the user refers to an earlier state - \"revert it\", \"undo that\", \"put the old one back\", " +
		"\"change it back to what it was\" - resolve it from this list. Do not ask the user for a value that " +
		"appears here, and do not search the artifact for something that merely resembles it.\n\n" +
		strings.Join(sections, "\n\n")
}

// summariseJournal renders a journal on one log line, so what an agent was
// actually told about earlier turns can be read back without the prompt.
func summariseJournal(note string) string {
	i := strings.Index(note, "change ")
	if i < 0 {
		i = 0
	}
	body := strings.ReplaceAll(note[i:], "\n", " | ")
	if len(body) > 900 {
		body = body[:900] + " ..."
	}
	return body
}

// ConversationJournal is what earlier turns of this conversation changed, for a
// caller outside this package.
//
// It reads the same working set the chat request is built from, so a component
// that has to reason about "what it was before" - the orchestrator deciding
// whether a question is answerable, say - sees exactly what the agent was told,
// and not a second, differently-derived account of the same thing.
func ConversationJournal(ctx context.Context, cm *ConversationManager, conversation *Conversation) string {
	if cm == nil || conversation == nil {
		return ""
	}
	return JournalNote(sessionStateOf(ctx, cm.ChatMemory, conversation.MemoryKey))
}
