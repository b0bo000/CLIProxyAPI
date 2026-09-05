package helps

import "context"

type claudeCodeTransportOwnerToken struct {
	_ [32]byte
}

// ClaudeCodeTransportOwner is an opaque capability for one downstream owner
// boundary. Its token cannot be constructed or replaced outside this package.
type ClaudeCodeTransportOwner struct {
	token *claudeCodeTransportOwnerToken
}

// NewClaudeCodeTransportOwner creates a process-local owner capability.
func NewClaudeCodeTransportOwner() ClaudeCodeTransportOwner {
	return ClaudeCodeTransportOwner{token: &claudeCodeTransportOwnerToken{}}
}

type claudeCodeTransportOwnerContextKey struct{}

// WithClaudeCodeTransportOwner attaches a valid owner capability to context.
// A zero owner is ignored so it cannot erase an inherited capability.
func WithClaudeCodeTransportOwner(ctx context.Context, owner ClaudeCodeTransportOwner) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if owner.token == nil {
		return ctx
	}
	return context.WithValue(ctx, claudeCodeTransportOwnerContextKey{}, owner)
}

func claudeCodeTransportOwnerFromContext(ctx context.Context) (*claudeCodeTransportOwnerToken, bool) {
	if ctx == nil {
		return nil, false
	}
	owner, ok := ctx.Value(claudeCodeTransportOwnerContextKey{}).(ClaudeCodeTransportOwner)
	if !ok || owner.token == nil {
		return nil, false
	}
	return owner.token, true
}
