package executor

import "context"

// PromptBoundaryKind describes an adapter-owned user-prompt lifecycle event.
// A transport handler must not derive this value from an arbitrary HTTP header
// or request-body field.
type PromptBoundaryKind string

const (
	PromptBoundaryAbsent    PromptBoundaryKind = "absent"
	PromptBoundaryNew       PromptBoundaryKind = "new"
	PromptBoundaryContinue  PromptBoundaryKind = "continue"
	PromptBoundaryAmbiguous PromptBoundaryKind = "ambiguous"
)

// PromptBoundaryHint carries a prompt boundary from an adapter that owns the
// user-prompt lifecycle. TransactionID is an opaque adapter-local key. It is
// not an upstream prompt ID and must never be sent on the wire.
type PromptBoundaryHint struct {
	Kind          PromptBoundaryKind
	TransactionID string
}

type promptBoundaryContextKey struct{}

// WithPromptBoundary returns a context containing an adapter-owned prompt
// boundary hint. Validation is provider-specific and occurs in the executor.
func WithPromptBoundary(ctx context.Context, hint PromptBoundaryHint) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, promptBoundaryContextKey{}, hint)
}

// PromptBoundaryFromContext returns the prompt boundary hint, when one was
// supplied through WithPromptBoundary.
func PromptBoundaryFromContext(ctx context.Context) (PromptBoundaryHint, bool) {
	if ctx == nil {
		return PromptBoundaryHint{}, false
	}
	hint, ok := ctx.Value(promptBoundaryContextKey{}).(PromptBoundaryHint)
	return hint, ok
}
