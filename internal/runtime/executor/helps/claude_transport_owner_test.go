package helps

import (
	"context"
	"testing"
)

func TestClaudeCodeTransportOwnerCreationIsOpaqueAndDistinct(t *testing.T) {
	t.Parallel()

	first := NewClaudeCodeTransportOwner()
	second := NewClaudeCodeTransportOwner()
	if first.token == nil || second.token == nil {
		t.Fatal("owner constructor returned a zero capability")
	}
	if first.token == second.token {
		t.Fatal("two owner constructors returned the same capability")
	}
	if _, ok := claudeCodeTransportOwnerFromContext(context.Background()); ok {
		t.Fatal("background context unexpectedly contained an owner")
	}
}

func TestClaudeCodeTransportOwnerContextPropagationAndReplacement(t *testing.T) {
	t.Parallel()

	first := NewClaudeCodeTransportOwner()
	second := NewClaudeCodeTransportOwner()
	ctx := WithClaudeCodeTransportOwner(context.Background(), first)
	got, ok := claudeCodeTransportOwnerFromContext(ctx)
	if !ok || got != first.token {
		t.Fatal("attached owner was not recovered")
	}
	child := context.WithValue(ctx, struct{}{}, "unrelated")
	if got, ok := claudeCodeTransportOwnerFromContext(child); !ok || got != first.token {
		t.Fatal("child context did not inherit owner")
	}
	replaced := WithClaudeCodeTransportOwner(ctx, second)
	if got, ok := claudeCodeTransportOwnerFromContext(replaced); !ok || got != second.token {
		t.Fatal("explicit owner replacement did not take effect")
	}
}

func TestClaudeCodeTransportOwnerRejectsZeroAndMissingValues(t *testing.T) {
	t.Parallel()

	zero := ClaudeCodeTransportOwner{}
	ctx := WithClaudeCodeTransportOwner(context.Background(), zero)
	if _, ok := claudeCodeTransportOwnerFromContext(ctx); ok {
		t.Fatal("zero owner was attached")
	}
	if _, ok := claudeCodeTransportOwnerFromContext(nil); ok {
		t.Fatal("nil context unexpectedly contained an owner")
	}
}

func TestClaudeCodeTransportOwnerNilContextUsesBackground(t *testing.T) {
	t.Parallel()

	owner := NewClaudeCodeTransportOwner()
	ctx := WithClaudeCodeTransportOwner(nil, owner)
	if ctx == nil {
		t.Fatal("nil context attachment returned nil")
	}
	if got, ok := claudeCodeTransportOwnerFromContext(ctx); !ok || got != owner.token {
		t.Fatal("owner was not attached to nil-derived context")
	}
}

func TestClaudeCodeTransportOwnerZeroDoesNotEraseInheritedValue(t *testing.T) {
	t.Parallel()

	owner := NewClaudeCodeTransportOwner()
	ctx := WithClaudeCodeTransportOwner(context.Background(), owner)
	ctx = WithClaudeCodeTransportOwner(ctx, ClaudeCodeTransportOwner{})
	if got, ok := claudeCodeTransportOwnerFromContext(ctx); !ok || got != owner.token {
		t.Fatal("zero owner erased inherited owner")
	}
}
