package agents

import "context"

// Quality judgement is the half of the inner loop that rules cannot reach.
//
// ValidateOutput asks whether an answer is WELL FORMED: does the component type
// exist, is the property real, was the query probed. It is computational, it is
// cheap, and it is right to run first. But a dashboard passed every one of those
// checks on the first attempt while carrying raw `an` and `avg_bal` column
// headings, a filler tile with nothing behind it, and a currency symbol on a
// count. Nothing was malformed. It was simply not good, and no rule this
// codebase could write would have said so.
//
// So after conformance passes and before the answer is handed back, an agent
// type may be asked one more question: is this fit for the person who will read
// it? The constraints below are not incidental - each is a hazard the harness
// literature measured.

// QualityRating is a closed set, deliberately.
//
// A judge asked for a number invents one. Strands' free-rubric path lets the
// model emit any float and has to beg it in the prompt - "THE FINAL SCORE MUST
// BE A DECIMAL BETWEEN 0.0 AND 1.0" - while their built-in rubrics project a
// StrEnum through a fixed mapping instead. The enum is the better design: three
// distinguishable states a judge can actually hold an opinion about, and no
// false precision to read meaning into later.
type QualityRating string

const (
	// QualityUnrated is the zero value: no judgement was made.
	QualityUnrated QualityRating = ""
	// QualityGood - fit to hand over as it is.
	QualityGood QualityRating = "good"
	// QualityAcceptable - imperfect but usable. Does NOT trigger a retry: an
	// agent that retries on "could be better" never stops.
	QualityAcceptable QualityRating = "acceptable"
	// QualityPoor - would embarrass whoever receives it. Worth one more attempt.
	QualityPoor QualityRating = "poor"
)

// RetryWorthy reports whether this rating justifies spending an attempt.
func (r QualityRating) RetryWorthy() bool { return r == QualityPoor }

// QualityVerdict is one judgement.
type QualityVerdict struct {
	Rating QualityRating `json:"rating"`
	// Reason must name the specific fault and the concrete fix. It is fed back
	// to the agent verbatim, so "could be better" wastes an attempt; "the grid
	// headings are the raw column names an, cl, os_amt - label them" does not.
	Reason string `json:"reason,omitempty"`
	// Attempt is which attempt was judged, so a verdict can be read against the
	// run that produced it.
	Attempt int `json:"attempt"`
	// Enforced says whether this verdict actually caused a retry. A poor verdict
	// on the final attempt is recorded and NOT enforced - see below.
	Enforced bool `json:"enforced"`
}

// QualityJudge is implemented by an agent type whose output is worth judging
// beyond conformance.
//
// Returning an error is not a failure of the answer: a judge that cannot run
// leaves the answer alone. A check that cannot be made must never read as a
// check that failed - the same rule the eval's BLOCKED state exists for.
type QualityJudge interface {
	// JudgeQuality is called once per attempt, only after conformance passes.
	JudgeQuality(ctx context.Context, output map[string]interface{}) (QualityVerdict, error)
}

// QualityRepairer is implemented by an agent type that can answer a quality
// verdict with a PATCH rather than a rewrite.
//
// This is not an optimisation. Asked to add a title to one chart on a valid
// dashboard, the eru_studio agent twice regenerated the entire page - and twice
// the rewrite was worse than what it replaced: dropped required keys, invented
// properties, and finally every query unbound. The answer being improved was
// already correct, so a full regeneration is a large and unnecessary bet on a
// small and specific change.
//
// Declining is always safe: the loop falls back to asking for the whole answer
// again.
type QualityRepairer interface {
	// QualityRepairTurn is given the answer that PASSED validation and the
	// verdict against it. Unlike RepairTurn, nothing here was rejected - the
	// prompt it returns has to say so, or the model sets about fixing an error
	// that does not exist.
	QualityRepairTurn(ctx context.Context, accepted map[string]interface{}, verdict QualityVerdict) (RepairTurn, bool)
}

// QualityGateBudget is how many attempts a poor verdict may cost, across a whole
// run. One, and the reasons are specific:
//
//   - Strands' Guide intervention discards the response and retries with
//     feedback, and its own docstring concedes "the framework imposes no retry
//     cap on guide-triggered retries". An unbounded taste loop is a bill, not a
//     feature.
//   - A judge is a model and is wrong sometimes. HarnessX's Critic saw a
//     concentration risk, said so, and shipped the edit anyway; DGM's grader was
//     satisfied by deleting the instrumentation it read. One attempt bounds what
//     a wrong judgement can cost.
//   - Conformance retries are separate and keep their own budget. Quality must
//     not eat the attempts a real defect needs.
const QualityGateBudget = 1

// qualityRetryPrompt is what the agent is told. It states plainly that the
// answer was accepted as correct, so the agent improves rather than re-checks
// what already passed.
const QualityRetryPrompt = `Your answer is valid - every structural check passed and nothing is malformed. This is not a rejection.

It was then read as the person receiving it will read it, and judged not good enough yet:

%s

Send the whole answer again with that addressed, and change nothing else. You have ONE attempt; after it the answer is delivered as it stands, so fix the named problem rather than reworking anything that already reads well.`
