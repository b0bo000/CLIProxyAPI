package helps

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	claudePrevRequestTTL            = time.Hour
	claudePrevRequestCleanupPeriod  = 15 * time.Minute
	claudePrevRequestMaxEntries     = 4096
	claudePrevRequestEvictBatchSize = 256
)

type claudePrevRequestEntry struct {
	previousRequestID string
	minimumSequence   uint64
	committedSequence uint64
	lastAccess        uint64
	expiresAt         time.Time
}

var claudePrevRequestState = struct {
	sync.Mutex
	entries      map[string]claudePrevRequestEntry
	lastCleanup  time.Time
	nextSequence uint64
	nextAccess   uint64
}{entries: make(map[string]claudePrevRequestEntry)}

// BeginClaudePrevRequest starts one request generation for a stable credential
// identity and Claude session scope. It returns the previous successfully
// committed upstream request ID, if any. Only a SHA-256 digest of the scope is
// retained, so access-token rotation does not expose or reset the state.
func BeginClaudePrevRequest(credentialIdentity, sessionScope string) (key string, sequence uint64, previousRequestID string) {
	credentialIdentity = strings.TrimSpace(credentialIdentity)
	sessionScope = strings.TrimSpace(sessionScope)
	if credentialIdentity == "" || sessionScope == "" {
		return "", 0, ""
	}
	digest := sha256.Sum256([]byte(credentialIdentity + "\x00" + sessionScope))
	key = hex.EncodeToString(digest[:])
	now := time.Now()

	claudePrevRequestState.Lock()
	defer claudePrevRequestState.Unlock()
	cleanupClaudePrevRequestLocked(now)

	entry, found := claudePrevRequestState.entries[key]
	newGeneration := !found || (!entry.expiresAt.IsZero() && now.After(entry.expiresAt))
	if newGeneration && !found {
		evictClaudePrevRequestLocked()
	}

	claudePrevRequestState.nextSequence++
	sequence = claudePrevRequestState.nextSequence
	if newGeneration {
		entry = claudePrevRequestEntry{minimumSequence: sequence}
	}
	claudePrevRequestState.nextAccess++
	entry.lastAccess = claudePrevRequestState.nextAccess
	entry.expiresAt = now.Add(claudePrevRequestTTL)
	claudePrevRequestState.entries[key] = entry
	return key, sequence, entry.previousRequestID
}

// CommitClaudePrevRequest advances continuity only after a response completes.
// An older concurrently started request cannot overwrite a newer generation,
// including after TTL expiry or capacity eviction.
func CommitClaudePrevRequest(key string, sequence uint64, requestID string) {
	key = strings.TrimSpace(key)
	requestID = strings.TrimSpace(requestID)
	if key == "" || sequence == 0 || requestID == "" {
		return
	}
	now := time.Now()

	claudePrevRequestState.Lock()
	defer claudePrevRequestState.Unlock()
	entry, ok := claudePrevRequestState.entries[key]
	if !ok || (!entry.expiresAt.IsZero() && !now.Before(entry.expiresAt)) || sequence < entry.minimumSequence || (entry.committedSequence != 0 && sequence <= entry.committedSequence) {
		return
	}
	claudePrevRequestState.nextAccess++
	entry.previousRequestID = requestID
	entry.committedSequence = sequence
	entry.lastAccess = claudePrevRequestState.nextAccess
	entry.expiresAt = now.Add(claudePrevRequestTTL)
	claudePrevRequestState.entries[key] = entry
}

func cleanupClaudePrevRequestLocked(now time.Time) {
	if !claudePrevRequestState.lastCleanup.IsZero() && now.Sub(claudePrevRequestState.lastCleanup) < claudePrevRequestCleanupPeriod {
		return
	}
	for key, entry := range claudePrevRequestState.entries {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			delete(claudePrevRequestState.entries, key)
		}
	}
	claudePrevRequestState.lastCleanup = now
}

func evictClaudePrevRequestLocked() {
	if len(claudePrevRequestState.entries) < claudePrevRequestMaxEntries {
		return
	}
	type candidate struct {
		key        string
		lastAccess uint64
	}
	candidates := make([]candidate, 0, len(claudePrevRequestState.entries))
	for key, entry := range claudePrevRequestState.entries {
		candidates = append(candidates, candidate{key: key, lastAccess: entry.lastAccess})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].lastAccess < candidates[j].lastAccess
	})
	count := min(claudePrevRequestEvictBatchSize, len(candidates))
	for _, candidate := range candidates[:count] {
		delete(claudePrevRequestState.entries, candidate.key)
	}
}

func resetClaudePrevRequestForTest() {
	claudePrevRequestState.Lock()
	defer claudePrevRequestState.Unlock()
	claudePrevRequestState.entries = make(map[string]claudePrevRequestEntry)
	claudePrevRequestState.lastCleanup = time.Time{}
	claudePrevRequestState.nextSequence = 0
	claudePrevRequestState.nextAccess = 0
}
