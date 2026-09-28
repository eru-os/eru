package reasoning_agents

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Noticing that the agent is going round in circles.
//
// # What this is for
//
// Two live dashboard runs failed the same way, and neither was random:
//
//	attempt 1  the page bound no queries
//	attempt 2  7 x "tile has no property query_result_path"
//	attempt 3  the page bound no queries
//	attempt 4  7 x "tile has no property query_result_path"
//
// The agent binds the tiles with a property tiles do not have, is told, removes
// the property by removing the binding, is told THAT, and binds again with the
// same bad property. A perfect A-B-A-B, and both runs spent their entire
// conformance budget inside it.
//
// Nothing noticed, because each attempt was judged alone. A fault the agent had
// already been shown, already "fixed", and already been shown again arrived
// looking exactly like a fresh one - so the feedback said "this is wrong" for
// the fourth time instead of "you are trading one fault for the other, and both
// have to hold at once", which is the only sentence that could have helped.
//
// # Why fingerprints rather than codes
//
// The loop does not know what an issue code is; it has an error. Hashing the
// normalised fault text keeps this generic - it works for eru_studio's formatted
// issue list, for a configured agent's rule findings, and for a plain schema
// error, without any of them knowing this exists.

// faultTrail remembers the shape of every rejection in a run.
type faultTrail struct {
	seenAt map[string][]int
	order  []string
}

func newFaultTrail() *faultTrail {
	return &faultTrail{seenAt: map[string][]int{}}
}

// note records a rejection and reports how many times this exact shape has been
// seen before, and on which attempts.
func (t *faultTrail) note(attempt int, message string) (repeats int, earlier []int) {
	print := fingerprintFault(message)
	if _, known := t.seenAt[print]; !known {
		t.order = append(t.order, print)
	}
	earlier = append([]int{}, t.seenAt[print]...)
	t.seenAt[print] = append(t.seenAt[print], attempt)
	return len(earlier), earlier
}

// distinct is how many different faults the run has produced, which is what
// tells "stuck on one thing" from "trading two things".
func (t *faultTrail) distinct() int { return len(t.order) }

// fingerprintFault reduces a rejection to what makes it the same complaint.
//
// The lines are sorted and the per-instance detail stripped, so "seven tiles
// have no such property" and "six tiles have no such property" are recognised as
// the same fault appearing again rather than as progress.
func fingerprintFault(message string) string {
	var lines []string
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, normaliseFaultLine(line))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "|")))
	return hex.EncodeToString(sum[:])[:16]
}

// normaliseFaultLine drops the parts that vary between two reports of the same
// underlying mistake: which index it was on, and how many there were.
func normaliseFaultLine(line string) string {
	var b strings.Builder
	inDigits := false
	for _, r := range line {
		if r >= '0' && r <= '9' {
			if !inDigits {
				b.WriteByte('#')
				inDigits = true
			}
			continue
		}
		inDigits = false
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// cycleNote is what the agent is told when a fault comes back.
//
// It names the other fault explicitly. "You have seen this before" invites
// another pass at the same fix; "you removed X to satisfy Y and have now been
// told about Y again" is the only framing that describes the actual problem.
const cycleNote = `
READ THIS FIRST. You have already been given this exact rejection, on attempt %s.

Something you changed since then has brought it back. That means you are trading one fault for another: the change that satisfied the last rejection is what caused this one. Fixing either alone will fail again.

Both have to hold at the same time. Work out what the two constraints are, state to yourself how a single answer can satisfy both, and only then write it. Do NOT simply undo your last change.
`

func describeAttempts(attempts []int) string {
	parts := make([]string, 0, len(attempts))
	for _, attempt := range attempts {
		parts = append(parts, itoa(attempt))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
