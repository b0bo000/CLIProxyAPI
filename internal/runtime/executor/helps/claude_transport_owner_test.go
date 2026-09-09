package helps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

func claudeTransportSessionContext(sessionID string) context.Context {
	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if sessionID != "" {
		ginContext.Request.Header.Set(ClaudeCodeSessionHeader, sessionID)
	}
	return context.WithValue(context.Background(), "gin", ginContext)
}

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

func TestClaudeCodeSessionTransportOwnerStableAndIsolated(t *testing.T) {
	first, stable := claudeCodeSessionTransportOwner(claudeTransportSessionContext("session-owner-a"))
	if !stable || first == nil {
		t.Fatal("valid session did not produce a stable owner")
	}
	reused, stable := claudeCodeSessionTransportOwner(claudeTransportSessionContext("session-owner-a"))
	if !stable || reused != first {
		t.Fatal("same session did not reuse its owner")
	}
	isolated, stable := claudeCodeSessionTransportOwner(claudeTransportSessionContext("session-owner-b"))
	if !stable || isolated == first {
		t.Fatal("different session reused the first owner")
	}
}

func TestClaudeCodeSessionTransportOwnerUsesEphemeralFallback(t *testing.T) {
	first, stable := claudeCodeSessionTransportOwner(context.Background())
	if stable || first == nil {
		t.Fatal("missing session did not produce an ephemeral owner")
	}
	second, stable := claudeCodeSessionTransportOwner(claudeTransportSessionContext("bad\nsession"))
	if stable || second == nil || second == first {
		t.Fatal("invalid session did not produce a distinct ephemeral owner")
	}
	tooLong := claudeTransportSessionContext(strings.Repeat("x", 257))
	third, stable := claudeCodeSessionTransportOwner(tooLong)
	if stable || third == nil || third == second {
		t.Fatal("oversized session did not produce a distinct ephemeral owner")
	}
}

func TestClaudeCodeSessionTransportOwnerConcurrentReuse(t *testing.T) {
	ctx := claudeTransportSessionContext("session-owner-concurrent")
	const workers = 64
	owners := make(chan *claudeCodeTransportOwnerToken, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			owner, stable := claudeCodeSessionTransportOwner(ctx)
			if !stable {
				owners <- nil
				return
			}
			owners <- owner
		}()
	}
	group.Wait()
	close(owners)

	var first *claudeCodeTransportOwnerToken
	for owner := range owners {
		if owner == nil {
			t.Fatal("concurrent lookup returned a non-stable owner")
		}
		if first == nil {
			first = owner
			continue
		}
		if owner != first {
			t.Fatal("concurrent lookups produced different owners")
		}
	}
}
