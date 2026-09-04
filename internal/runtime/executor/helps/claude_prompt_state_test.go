package helps

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParseClaudePromptIDBillingText(t *testing.T) {
	t.Parallel()

	const promptID = "986813f9-290b-409e-8541-06cd11254627"
	tests := []struct {
		name      string
		text      string
		wantID    string
		wantFound bool
		wantErr   bool
	}{
		{
			name:      "valid UUID",
			text:      "x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cc_prompt_id=" + promptID + ";",
			wantID:    promptID,
			wantFound: true,
		},
		{
			name:      "valid ID with surrounding billing whitespace",
			text:      "  x-anthropic-billing-header: cc_entrypoint=sdk-cli; cc_prompt_id=986813f9-290b-409e-8541-06cd11254627;  ",
			wantID:    "986813f9-290b-409e-8541-06cd11254627",
			wantFound: true,
		},
		{
			name: "absent field",
			text: "x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli;",
		},
		{
			name: "not a billing block",
			text: "ordinary system text",
		},
		{
			name:    "empty value",
			text:    "x-anthropic-billing-header: cc_entrypoint=sdk-cli; cc_prompt_id=;",
			wantErr: true,
		},
		{
			name:    "uppercase UUID",
			text:    "x-anthropic-billing-header: cc_prompt_id=986813F9-290B-409E-8541-06CD11254627;",
			wantErr: true,
		},
		{
			name:    "non UUID value",
			text:    "x-anthropic-billing-header: cc_prompt_id=prompt-1;",
			wantErr: true,
		},
		{
			name:    "duplicate value",
			text:    "x-anthropic-billing-header: cc_prompt_id=one; cc_prompt_id=two;",
			wantErr: true,
		},
		{
			name:    "delimiter in value",
			text:    "x-anthropic-billing-header: cc_prompt_id=prompt;bad;",
			wantErr: true,
		},
		{
			name:    "malformed segment",
			text:    "x-anthropic-billing-header: cc_prompt_id=prompt-1; broken;",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotID, gotFound, errParse := ParseClaudePromptIDBillingText(test.text)
			if test.wantErr {
				if errParse == nil {
					t.Fatalf("error = nil, want validation error")
				}
				return
			}
			if errParse != nil {
				t.Fatalf("ParseClaudePromptIDBillingText() error = %v", errParse)
			}
			if gotID != test.wantID || gotFound != test.wantFound {
				t.Fatalf("got (%q, %v), want (%q, %v)", gotID, gotFound, test.wantID, test.wantFound)
			}
		})
	}
}

func TestClaudeCodeSubagentMarkerFromBody(t *testing.T) {
	t.Parallel()

	base := `{"model":"m","system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; %s"}],"messages":[{"role":"user","content":"prompt"}]}`
	tests := []struct {
		name      string
		billing   string
		wantValue bool
		wantFound bool
		wantErr   bool
	}{
		{name: "true", billing: "cc_is_subagent=true;", wantValue: true, wantFound: true},
		{name: "false", billing: "cc_is_subagent=false;", wantFound: true},
		{name: "absent", billing: "", wantFound: false},
		{name: "duplicate", billing: "cc_is_subagent=true; cc_is_subagent=false;", wantErr: true},
		{name: "malformed", billing: "broken;", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(base, test.billing))
			gotValue, gotFound, errMarker := ClaudeCodeSubagentMarkerFromBody(body)
			if test.wantErr {
				if errMarker == nil {
					t.Fatal("ClaudeCodeSubagentMarkerFromBody() error = nil, want error")
				}
				return
			}
			if errMarker != nil {
				t.Fatalf("ClaudeCodeSubagentMarkerFromBody() error = %v", errMarker)
			}
			if gotValue != test.wantValue || gotFound != test.wantFound {
				t.Fatalf("marker = (%v, %v), want (%v, %v)", gotValue, gotFound, test.wantValue, test.wantFound)
			}
		})
	}
}

func TestBeginClaudePromptIDLifecycle(t *testing.T) {
	resetClaudePromptStateForTest()
	defer resetClaudePromptStateForTest()

	base := []byte(`{"messages":[{"role":"user","content":"first"}]}`)
	id, generated, errBegin := BeginClaudePromptID("credential-a", "claude:session-a:agent:main", base)
	if errBegin != nil || !generated {
		t.Fatalf("first BeginClaudePromptID() = (%q, %v, %v), want generated ID", id, generated, errBegin)
	}
	if errUUID := uuid.Validate(id); errUUID != nil {
		t.Fatalf("generated ID %q is not UUID-shaped: %v", id, errUUID)
	}
	if retryID, retryGenerated, errRetry := BeginClaudePromptID("credential-a", "claude:session-a:agent:main", base); errRetry != nil || retryGenerated || retryID != id {
		t.Fatalf("retry BeginClaudePromptID() = (%q, %v, %v), want stable %q", retryID, retryGenerated, errRetry, id)
	}

	continuation := []byte(`{"messages":[{"role":"user","content":"first"},{"role":"assistant","content":[{"type":"tool_use","id":"tool-1","name":"probe","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":"done"}]}]}`)
	if continuationID, continuationGenerated, errContinuation := BeginClaudePromptID("credential-a", "claude:session-a:agent:main", continuation); errContinuation != nil || continuationGenerated || continuationID != id {
		t.Fatalf("tool continuation = (%q, %v, %v), want stable %q", continuationID, continuationGenerated, errContinuation, id)
	}

	nextPrompt := []byte(`{"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"done"},{"role":"user","content":"second"}]}`)
	nextID, nextGenerated, errNext := BeginClaudePromptID("credential-a", "claude:session-a:agent:main", nextPrompt)
	if errNext != nil || !nextGenerated || nextID == id {
		t.Fatalf("new prompt = (%q, %v, %v), want a distinct generated ID", nextID, nextGenerated, errNext)
	}
	if otherSessionID, _, errOtherSession := BeginClaudePromptID("credential-a", "claude:session-b:agent:main", base); errOtherSession != nil || otherSessionID == id {
		t.Fatalf("other session ID = %q, want isolation from %q", otherSessionID, id)
	}
	if otherCredentialID, _, errOtherCredential := BeginClaudePromptID("credential-b", "claude:session-a:agent:main", base); errOtherCredential != nil || otherCredentialID == id {
		t.Fatalf("other credential ID = %q, want isolation from %q", otherCredentialID, id)
	}
}

func TestBeginClaudePromptIDToolContinuationWithoutStateStaysAbsent(t *testing.T) {
	resetClaudePromptStateForTest()
	defer resetClaudePromptStateForTest()

	continuation := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":"done"}]}]}`)
	if id, generated, errBegin := BeginClaudePromptID("credential-a", "claude:session-a:agent:main", continuation); errBegin != nil || generated || id != "" {
		t.Fatalf("orphan continuation = (%q, %v, %v), want absent", id, generated, errBegin)
	}
}

func TestBeginClaudePromptIDInheritedReusesParentAcrossBoundary(t *testing.T) {
	resetClaudePromptStateForTest()
	defer resetClaudePromptStateForTest()

	parent := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli;"}],"messages":[{"role":"user","content":"parent"}]}`)
	child := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cc_is_subagent=true;"}],"messages":[{"role":"user","content":"different child boundary"}]}`)
	parentID, generated, errParent := BeginClaudePromptID("credential-inherit", "claude:session-inherit:agent:main", parent)
	if errParent != nil || !generated || parentID == "" {
		t.Fatalf("parent = (%q, %v, %v), want generated ID", parentID, generated, errParent)
	}
	childID, childGenerated, errChild := BeginClaudePromptIDInherited("credential-inherit", "claude:session-inherit:agent:main", child)
	if errChild != nil || childGenerated || childID != parentID {
		t.Fatalf("child = (%q, %v, %v), want inherited parent %q", childID, childGenerated, errChild, parentID)
	}

	orphanID, orphanGenerated, errOrphan := BeginClaudePromptIDInherited("credential-orphan", "claude:session-orphan:agent:main", child)
	if errOrphan != nil || orphanGenerated || orphanID != "" {
		t.Fatalf("orphan child = (%q, %v, %v), want absent without parent state", orphanID, orphanGenerated, errOrphan)
	}
}

func TestBeginClaudePromptIDRejectsExpiredState(t *testing.T) {
	resetClaudePromptStateForTest()
	defer resetClaudePromptStateForTest()

	base := []byte(`{"messages":[{"role":"user","content":"first"}]}`)
	firstID, _, errFirst := BeginClaudePromptID("credential-a", "claude:session-a:agent:main", base)
	if errFirst != nil {
		t.Fatalf("first BeginClaudePromptID() error = %v", errFirst)
	}
	digest := sha256.Sum256([]byte("credential-a\x00claude:session-a:agent:main"))
	key := hex.EncodeToString(digest[:])
	claudePromptState.Lock()
	entry := claudePromptState.entries[key]
	entry.expiresAt = time.Now().Add(-time.Second)
	claudePromptState.entries[key] = entry
	claudePromptState.Unlock()

	secondID, generated, errSecond := BeginClaudePromptID("credential-a", "claude:session-a:agent:main", base)
	if errSecond != nil || !generated || secondID == firstID {
		t.Fatalf("expired BeginClaudePromptID() = (%q, %v, %v), want a new generated UUID", secondID, generated, errSecond)
	}
}

func TestBeginClaudePromptIDConcurrentCallsShareOnePrompt(t *testing.T) {
	resetClaudePromptStateForTest()
	defer resetClaudePromptStateForTest()

	const callCount = 64
	base := []byte(`{"messages":[{"role":"user","content":"first"}]}`)
	ids := make(chan string, callCount)
	generated := make(chan bool, callCount)
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	waitGroup.Add(callCount)
	for range callCount {
		go func() {
			defer waitGroup.Done()
			<-start
			id, isGenerated, errBegin := BeginClaudePromptID("credential-a", "claude:session-a:agent:main", base)
			if errBegin != nil {
				t.Errorf("concurrent BeginClaudePromptID() error = %v", errBegin)
				return
			}
			ids <- id
			generated <- isGenerated
		}()
	}
	close(start)
	waitGroup.Wait()
	close(ids)
	close(generated)

	var firstID string
	generatedCount := 0
	for id := range ids {
		if id == "" {
			t.Fatal("concurrent BeginClaudePromptID() returned an empty ID")
		}
		if firstID == "" {
			firstID = id
		} else if id != firstID {
			t.Fatalf("concurrent IDs differ: first=%q current=%q", firstID, id)
		}
	}
	for isGenerated := range generated {
		if isGenerated {
			generatedCount++
		}
	}
	if generatedCount != 1 {
		t.Fatalf("generated count = %d, want exactly one", generatedCount)
	}
}

func TestClaudePromptIDFromBodyRejectsAmbiguousBillingBlocks(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_entrypoint=sdk-cli;"},{"type":"text","text":"x-anthropic-billing-header: cc_entrypoint=sdk-cli;"}],"messages":[{"role":"user","content":"prompt"}]}`)
	if _, _, errBody := ClaudePromptIDFromBody(body); errBody == nil {
		t.Fatal("ClaudePromptIDFromBody() error = nil, want multiple billing block error")
	}
}

func TestClaudePromptIDCallerValueAndBillingInsertion(t *testing.T) {
	const callerID = "986813f9-290b-409e-8541-06cd11254627"
	body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000; cc_prev_req=req_previous;"}],"messages":[{"role":"user","content":"prompt"}]}`)
	callerBody := []byte(strings.Replace(string(body), "cc_prev_req=req_previous;", "cc_prev_req=req_previous; cc_prompt_id="+callerID+";", 1))
	if id, generated, errBegin := BeginClaudePromptID("credential-a", "claude:session-a:agent:main", callerBody); errBegin != nil || generated || id != callerID {
		t.Fatalf("caller-owned resolution = (%q, %v, %v), want %q", id, generated, errBegin, callerID)
	}
	if updated, errInsert := InsertClaudePromptIDBilling(callerBody, callerID); errInsert != nil || string(updated) != string(callerBody) {
		t.Fatalf("caller-owned insertion changed body: err=%v body=%s", errInsert, updated)
	}
	updated, errInsert := InsertClaudePromptIDBilling(body, callerID)
	if errInsert != nil {
		t.Fatalf("generated insertion error = %v", errInsert)
	}
	billing := string(updated)
	if !strings.Contains(billing, "cc_prev_req=req_previous; cc_prompt_id="+callerID+";") {
		t.Fatalf("generated insertion order = %s", billing)
	}
	fallback, errFallback := AppendClaudePromptIDBillingText("x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000;", callerID)
	if errFallback != nil || !strings.HasSuffix(fallback, "cc_prompt_id="+callerID+";") {
		t.Fatalf("fallback append = (%q, %v)", fallback, errFallback)
	}
}
