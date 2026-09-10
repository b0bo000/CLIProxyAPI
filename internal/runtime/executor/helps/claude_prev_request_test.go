package helps

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClaudePrevRequestTracksCompletedRequestPerCredentialSession(t *testing.T) {
	resetClaudePrevRequestForTest()
	defer resetClaudePrevRequestForTest()

	key, sequence, previous := BeginClaudePrevRequest("credential-a", "session-a")
	if key == "" || sequence != 1 || previous != "" {
		t.Fatalf("first begin = %q/%d/%q, want key/1/empty", key, sequence, previous)
	}
	if strings.Contains(key, "credential-a") || strings.Contains(key, "session-a") {
		t.Fatalf("state key exposes identity: %q", key)
	}
	CommitClaudePrevRequest(key, sequence, "req_first")
	secondKey, secondSequence, previous := BeginClaudePrevRequest("credential-a", "session-a")
	if secondKey != key || secondSequence != 2 || previous != "req_first" {
		t.Fatalf("second begin = %q/%d/%q, want same key/2/req_first", secondKey, secondSequence, previous)
	}

	_, _, otherSession := BeginClaudePrevRequest("credential-a", "session-b")
	_, _, otherCredential := BeginClaudePrevRequest("credential-b", "session-a")
	if otherSession != "" || otherCredential != "" {
		t.Fatalf("prev request leaked across identity: session=%q credential=%q", otherSession, otherCredential)
	}
}

func TestClaudePrevRequestRequiresCredentialAndSessionScope(t *testing.T) {
	resetClaudePrevRequestForTest()
	defer resetClaudePrevRequestForTest()

	for _, test := range []struct {
		credential string
		session    string
	}{
		{credential: "", session: "session"},
		{credential: "credential", session: ""},
		{credential: "  ", session: "session"},
		{credential: "credential", session: "  "},
	} {
		key, sequence, previous := BeginClaudePrevRequest(test.credential, test.session)
		if key != "" || sequence != 0 || previous != "" {
			t.Fatalf("empty scope begin = %q/%d/%q, want empty/0/empty", key, sequence, previous)
		}
	}
	claudePrevRequestState.Lock()
	entryCount := len(claudePrevRequestState.entries)
	claudePrevRequestState.Unlock()
	if entryCount != 0 {
		t.Fatalf("empty scope created %d state entries", entryCount)
	}
}

func TestClaudePrevRequestRejectsExpiredGenerationCommit(t *testing.T) {
	resetClaudePrevRequestForTest()
	defer resetClaudePrevRequestForTest()

	key, expiredSequence, _ := BeginClaudePrevRequest("credential", "session")
	claudePrevRequestState.Lock()
	entry := claudePrevRequestState.entries[key]
	entry.expiresAt = time.Now().Add(-time.Second)
	claudePrevRequestState.entries[key] = entry
	claudePrevRequestState.Unlock()

	newKey, currentSequence, previous := BeginClaudePrevRequest("credential", "session")
	if newKey != key || currentSequence <= expiredSequence || previous != "" {
		t.Fatalf("new generation = %q/%d/%q, want same key/new sequence/empty", newKey, currentSequence, previous)
	}
	CommitClaudePrevRequest(newKey, currentSequence, "req_current")
	CommitClaudePrevRequest(key, expiredSequence, "req_expired")
	_, _, previous = BeginClaudePrevRequest("credential", "session")
	if previous != "req_current" {
		t.Fatalf("previous request = %q, want current generation", previous)
	}
}

func TestClaudePrevRequestRejectsDirectLateCommitAfterExpiry(t *testing.T) {
	resetClaudePrevRequestForTest()
	defer resetClaudePrevRequestForTest()

	key, sequence, _ := BeginClaudePrevRequest("credential", "session")
	claudePrevRequestState.Lock()
	entry := claudePrevRequestState.entries[key]
	entry.expiresAt = time.Now().Add(-time.Second)
	claudePrevRequestState.entries[key] = entry
	claudePrevRequestState.Unlock()

	CommitClaudePrevRequest(key, sequence, "req_late")
	_, _, previous := BeginClaudePrevRequest("credential", "session")
	if previous != "" {
		t.Fatalf("previous request after direct late commit = %q, want empty", previous)
	}
}

func TestClaudePrevRequestCacheEvictsOldestEntriesWithinCapacity(t *testing.T) {
	resetClaudePrevRequestForTest()
	defer resetClaudePrevRequestForTest()

	firstKey, firstSequence, _ := BeginClaudePrevRequest("credential", "session-0")
	var newestKey string
	for index := 1; index <= claudePrevRequestMaxEntries; index++ {
		newestKey, _, _ = BeginClaudePrevRequest("credential", fmt.Sprintf("session-%d", index))
	}

	claudePrevRequestState.Lock()
	entryCount := len(claudePrevRequestState.entries)
	_, firstFound := claudePrevRequestState.entries[firstKey]
	_, newestFound := claudePrevRequestState.entries[newestKey]
	claudePrevRequestState.Unlock()
	if entryCount > claudePrevRequestMaxEntries {
		t.Fatalf("cache entries = %d, want at most %d", entryCount, claudePrevRequestMaxEntries)
	}
	if firstFound {
		t.Fatal("oldest prev-request entry was not evicted")
	}
	if !newestFound {
		t.Fatal("newest prev-request entry was evicted")
	}

	newKey, newSequence, _ := BeginClaudePrevRequest("credential", "session-0")
	if newKey != firstKey || newSequence <= firstSequence {
		t.Fatalf("recreated generation = %q/%d, want same key after sequence %d", newKey, newSequence, firstSequence)
	}
	CommitClaudePrevRequest(newKey, newSequence, "req_recreated")
	CommitClaudePrevRequest(firstKey, firstSequence, "req_evicted")
	_, _, previous := BeginClaudePrevRequest("credential", "session-0")
	if previous != "req_recreated" {
		t.Fatalf("previous request = %q, want recreated generation", previous)
	}
}

func TestClaudePrevRequestRejectsLateOlderCommit(t *testing.T) {
	resetClaudePrevRequestForTest()
	defer resetClaudePrevRequestForTest()

	key, first, _ := BeginClaudePrevRequest("credential", "session")
	_, second, _ := BeginClaudePrevRequest("credential", "session")
	CommitClaudePrevRequest(key, second, "req_newer")
	CommitClaudePrevRequest(key, first, "req_older")
	_, _, previous := BeginClaudePrevRequest("credential", "session")
	if previous != "req_newer" {
		t.Fatalf("previous request = %q, want newer completed generation", previous)
	}
}

func TestClaudePrevRequestCredentialRotationPreservesStableIdentityOnly(t *testing.T) {
	resetClaudePrevRequestForTest()
	defer resetClaudePrevRequestForTest()

	key, sequence, _ := BeginClaudePrevRequest("credential-stable-id", "session")
	CommitClaudePrevRequest(key, sequence, "req_before_rotation")

	rotatedKey, rotatedSequence, previous := BeginClaudePrevRequest("credential-stable-id", "session")
	if rotatedKey != key || rotatedSequence <= sequence || previous != "req_before_rotation" {
		t.Fatalf("rotated credential identity = %q/%d/%q, want stable key/new sequence/previous request", rotatedKey, rotatedSequence, previous)
	}

	_, _, otherCredentialPrevious := BeginClaudePrevRequest("credential-new-id", "session")
	if otherCredentialPrevious != "" {
		t.Fatalf("new credential identity inherited previous request %q", otherCredentialPrevious)
	}
}

func TestClaudePrevRequestConcurrentBeginsAndCommitsKeepNewestSequence(t *testing.T) {
	resetClaudePrevRequestForTest()
	defer resetClaudePrevRequestForTest()

	const requestCount = 64
	type startedRequest struct {
		key      string
		sequence uint64
	}
	started := make(chan startedRequest, requestCount)
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	waitGroup.Add(requestCount)
	for index := 0; index < requestCount; index++ {
		go func() {
			defer waitGroup.Done()
			<-start
			key, sequence, _ := BeginClaudePrevRequest("credential", "session")
			started <- startedRequest{key: key, sequence: sequence}
		}()
	}
	close(start)
	waitGroup.Wait()
	close(started)

	seen := make(map[uint64]bool, requestCount)
	requests := make([]startedRequest, 0, requestCount)
	var newest startedRequest
	for request := range started {
		if request.key == "" || request.sequence == 0 || seen[request.sequence] {
			t.Fatalf("invalid or duplicate concurrent generation: %+v", request)
		}
		seen[request.sequence] = true
		requests = append(requests, request)
		if request.sequence > newest.sequence {
			newest = request
		}
	}
	if len(seen) != requestCount {
		t.Fatalf("concurrent generation count = %d, want %d", len(seen), requestCount)
	}

	var commitWaitGroup sync.WaitGroup
	for _, request := range requests {
		commitWaitGroup.Add(1)
		go func(request startedRequest) {
			defer commitWaitGroup.Done()
			requestID := fmt.Sprintf("req_%d", request.sequence)
			if request.sequence == newest.sequence {
				requestID = "req_newest"
			}
			CommitClaudePrevRequest(request.key, request.sequence, requestID)
		}(request)
	}
	commitWaitGroup.Wait()

	_, _, previous := BeginClaudePrevRequest("credential", "session")
	if previous != "req_newest" {
		t.Fatalf("previous request after concurrent commits = %q, want newest completed request", previous)
	}
}

func TestClaudePrevRequestIgnoresEmptyCommitValues(t *testing.T) {
	resetClaudePrevRequestForTest()
	defer resetClaudePrevRequestForTest()

	key, sequence, _ := BeginClaudePrevRequest("credential", "session")
	for _, requestID := range []string{"", " ", "\t"} {
		CommitClaudePrevRequest(key, sequence, requestID)
	}
	_, _, previous := BeginClaudePrevRequest("credential", "session")
	if previous != "" {
		t.Fatalf("previous request = %q, want empty after empty commits", previous)
	}
}

func TestClaudePrevRequestRejectsDuplicateGenerationCommit(t *testing.T) {
	resetClaudePrevRequestForTest()
	defer resetClaudePrevRequestForTest()

	key, sequence, _ := BeginClaudePrevRequest("credential", "session")
	CommitClaudePrevRequest(key, sequence, "req_first")
	CommitClaudePrevRequest(key, sequence, "req_duplicate")
	_, _, previous := BeginClaudePrevRequest("credential", "session")
	if previous != "req_first" {
		t.Fatalf("previous request after duplicate commit = %q, want first commit", previous)
	}
}
