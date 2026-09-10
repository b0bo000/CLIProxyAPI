package auth

import (
	"context"
	"sync/atomic"
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type testClaudeCodeTransportOwnerProvider struct {
	calls atomic.Int32
}

type testClaudeCodeTransportOwnerContextKey struct{}

func (p *testClaudeCodeTransportOwnerProvider) BindClaudeCodeTransportOwner(ctx context.Context) context.Context {
	p.calls.Add(1)
	return context.WithValue(ctx, testClaudeCodeTransportOwnerContextKey{}, "owner")
}

func TestManagerClaudeCodeTransportOwnerProviderBinding(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	base := context.Background()
	if got := manager.bindClaudeCodeTransportOwner(base); got != base {
		t.Fatal("ownerless manager changed context")
	}

	provider := &testClaudeCodeTransportOwnerProvider{}
	manager.SetClaudeCodeTransportOwnerProvider(provider)
	bound := manager.bindClaudeCodeTransportOwner(base)
	if got := bound.Value(testClaudeCodeTransportOwnerContextKey{}); got != "owner" {
		t.Fatalf("bound owner value = %v, want owner", got)
	}
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}

	manager.SetClaudeCodeTransportOwnerProvider(nil)
	if got := manager.bindClaudeCodeTransportOwner(base); got != base {
		t.Fatal("nil provider did not restore ownerless behavior")
	}
}

func TestManagerClaudeCodeTransportOwnerProviderSkipsNonClaudeProviders(t *testing.T) {
	provider := &testClaudeCodeTransportOwnerProvider{}
	manager := NewManager(nil, nil, nil)
	manager.SetClaudeCodeTransportOwnerProvider(provider)
	_, _ = manager.Execute(context.Background(), []string{"codex"}, cliproxyexecutor.Request{}, cliproxyexecutor.Options{})
	if got := provider.calls.Load(); got != 0 {
		t.Fatalf("non-Claude provider calls = %d, want 0", got)
	}
}

func TestManagerClaudeCodeTransportOwnerProviderNilResultKeepsContext(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.SetClaudeCodeTransportOwnerProvider(nilResultClaudeCodeTransportOwnerProvider{})
	base := context.Background()
	if got := manager.bindClaudeCodeTransportOwner(base); got != base {
		t.Fatal("nil provider result changed context")
	}
}

func TestManagerExecutionEntrypointsBindOwnerBeforeDispatch(t *testing.T) {
	provider := &testClaudeCodeTransportOwnerProvider{}
	manager := NewManager(nil, nil, nil)
	manager.SetClaudeCodeTransportOwnerProvider(provider)

	_, _ = manager.Execute(context.Background(), []string{"claude"}, cliproxyexecutor.Request{}, cliproxyexecutor.Options{})
	_, _ = manager.ExecuteCount(context.Background(), []string{"claude"}, cliproxyexecutor.Request{}, cliproxyexecutor.Options{})
	_, _ = manager.ExecuteStream(context.Background(), []string{"claude"}, cliproxyexecutor.Request{}, cliproxyexecutor.Options{})

	if got := provider.calls.Load(); got != 3 {
		t.Fatalf("execution entrypoint provider calls = %d, want 3", got)
	}
}

type nilResultClaudeCodeTransportOwnerProvider struct{}

func (nilResultClaudeCodeTransportOwnerProvider) BindClaudeCodeTransportOwner(context.Context) context.Context {
	return nil
}
