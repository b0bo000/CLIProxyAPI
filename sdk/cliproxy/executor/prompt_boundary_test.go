package executor

import (
	"context"
	"testing"
)

func TestPromptBoundaryContextRoundTrip(t *testing.T) {
	t.Parallel()

	hint := PromptBoundaryHint{Kind: PromptBoundaryNew, TransactionID: "prompt-transaction-1"}
	ctx := WithPromptBoundary(context.Background(), hint)
	got, ok := PromptBoundaryFromContext(ctx)
	if !ok || got != hint {
		t.Fatalf("PromptBoundaryFromContext() = (%+v, %t), want (%+v, true)", got, ok, hint)
	}

	next := PromptBoundaryHint{Kind: PromptBoundaryContinue, TransactionID: hint.TransactionID}
	got, ok = PromptBoundaryFromContext(WithPromptBoundary(ctx, next))
	if !ok || got != next {
		t.Fatalf("overridden PromptBoundaryFromContext() = (%+v, %t), want (%+v, true)", got, ok, next)
	}
}

func TestPromptBoundaryContextAbsent(t *testing.T) {
	t.Parallel()

	for name, ctx := range map[string]context.Context{
		"nil":        nil,
		"background": context.Background(),
	} {
		t.Run(name, func(t *testing.T) {
			if got, ok := PromptBoundaryFromContext(ctx); ok || got != (PromptBoundaryHint{}) {
				t.Fatalf("PromptBoundaryFromContext() = (%+v, %t), want zero, false", got, ok)
			}
		})
	}
}

func TestWithPromptBoundaryAcceptsNilParent(t *testing.T) {
	t.Parallel()

	hint := PromptBoundaryHint{Kind: PromptBoundaryAmbiguous}
	got, ok := PromptBoundaryFromContext(WithPromptBoundary(nil, hint))
	if !ok || got != hint {
		t.Fatalf("PromptBoundaryFromContext() = (%+v, %t), want (%+v, true)", got, ok, hint)
	}
}
