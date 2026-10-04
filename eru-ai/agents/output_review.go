package agents

import (
	"context"

	models "github.com/eru-os/eru/eru-ai/models"
)

// OutputReview is what an agent type learned by looking at an answer that had
// already passed every check - typically by having the client render it.
//
// Findings are hard: concrete defects the answer must fix, phrased like a
// validation error. Files (screenshots) are soft: shown to the model so it can
// judge the result as a person would, but no rule depends on them.
type OutputReview struct {
	Findings error
	Files    []models.FileMessage
	Prompt   string
}

// OutputReviewer is implemented by an agent type that can review a validated
// answer - render it, measure it, look at it - before it is delivered.
//
// ok is false when no review happened (no client, nothing to review, review
// timed out), and the answer is delivered as it stands. A review never fails a
// run: the loop keeps the reviewed answer and falls back to it if the revision
// does worse.
type OutputReviewer interface {
	ReviewOutput(ctx context.Context, output map[string]interface{}, round int) (review OutputReview, ok bool)
}

// MaxReviewRounds caps review-driven revisions per run.
const MaxReviewRounds = 2
