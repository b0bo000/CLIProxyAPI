package helps

import (
	"context"
	"crypto/sha256"
	"strings"
	"unicode"

	internalcache "github.com/router-for-me/CLIProxyAPI/v7/internal/cache"
)

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

const claudeCodeSessionTransportOwnerCapacity = 4096

var claudeCodeSessionTransportOwners = internalcache.NewBoundedLRU[[sha256.Size]byte, ClaudeCodeTransportOwner](
	claudeCodeSessionTransportOwnerCapacity,
	nil,
)

// claudeCodeSessionTransportOwner returns one process-local owner for a valid
// Claude Code session. The cache key is irreversible and does not retain the
// caller-provided session value. Missing or invalid sessions receive an
// ephemeral owner so they cannot fall back into one shared transport pool.
func claudeCodeSessionTransportOwner(ctx context.Context) (*claudeCodeTransportOwnerToken, bool) {
	sessionID := strings.TrimSpace(ExtractClaudeCodeSessionID(ctx, nil, nil))
	if !validClaudeCodeTransportSessionID(sessionID) {
		owner := NewClaudeCodeTransportOwner()
		return owner.token, false
	}

	scope := sha256.Sum256([]byte("cpa:claude-code-transport-session:v1\x00" + sessionID))
	owner := claudeCodeSessionTransportOwners.GetOrAdd(scope, NewClaudeCodeTransportOwner)
	return owner.token, true
}

func validClaudeCodeTransportSessionID(sessionID string) bool {
	if sessionID == "" || len(sessionID) > 256 {
		return false
	}
	for _, r := range sessionID {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
