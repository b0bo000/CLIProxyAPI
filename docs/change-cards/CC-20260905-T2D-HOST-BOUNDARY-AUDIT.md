# Change Card: T2-D Host Boundary Audit

## Identity

- Signal: `pool/ownership` (default-server host boundary)
- Scope: determine whether the default HTTP server can truthfully bind one opaque transport owner per downstream process
- Branch: `feat/s3-resolved-profile`
- Baseline: `571fba46`
- Date: 2026-09-05
- Status: `DESIGN_COMPLETE_NO_SOURCE_CHANGE`

## Question

T2-A through T2-C provide an opaque, process-local `ClaudeCodeTransportOwner`, owner-aware transport-cache keys, and an explicit trusted provider boundary. This audit answers whether the default CPA HTTP server has enough trusted lifecycle information to install that provider automatically without deriving identity from caller-controlled or connection-derived data.

## Verified source facts

1. `internal/api/server.go:118-173` constructs one `BaseAPIHandler` from one `auth.Manager` for the server. The server constructor does not receive a downstream process registry or owner factory.
2. `sdk/cliproxy/builder.go:192-195,269-273` exposes an explicit `WithClaudeCodeTransportOwnerProvider` opt-in. The default builder installs no provider.
3. `sdk/cliproxy/auth/conductor.go:33-40` defines the provider as a trusted embedding-host contract. It must preserve one opaque owner for a downstream instance and must leave the context unchanged when the boundary is unknown.
4. `sdk/cliproxy/auth/conductor_execution.go:30-167` binds the provider at Manager execution boundaries and carries the same context through retries. The binding is provider-wide, not inferred per request.
5. `sdk/cliproxy/auth/conductor_execution.go:1679,1737` also binds direct Claude request paths, while non-Claude providers are skipped.
6. No verified default-server path creates a distinct owner for each inbound client process. An HTTP request supplies a request/connection boundary, not a trusted process identity.

## Candidate integration boundaries

| Candidate | Identity source | Official-process equivalence | Decision |
|---|---|---:|---|
| Trusted in-process embedding | Host creates and retains one opaque owner per downstream instance, then injects it into that instance's context | Highest available | Valid opt-in; requires a real embedding host lifecycle |
| Process-per-client deployment | Each downstream client gets its own CPA process and process-local cache | Strong at process boundary | Valid operational deployment; no default server code needed |
| Connection-scoped owner | Generate/reuse owner from inbound TCP connection | Not equivalent to process identity; reconnects and multiplexing break the mapping | Keep as an experiment-only approximation |
| Session/credential/UA/IP/header/body-derived owner | Caller-controlled or request-derived value | Not trustworthy as process identity and can merge/split unrelated processes | Prohibited |
| Request-scoped owner | New owner for every HTTP request | Breaks keep-alive/TLS/session-cache behavior | Prohibited |

## Required invariants for a future integration

- The owner is created by a trusted host before Manager dispatch and is opaque/non-serialisable.
- The same downstream process instance retains the same owner across sessions, retries, and requests.
- A process restart creates a new owner and cannot reuse the old owner or its transport cache.
- No owner is inferred from session ID, credential ID, OAuth material, User-Agent, IP, headers, body, PID, process name, TCP connection, or timing.
- Non-Claude providers remain unaffected.
- The default HTTP server remains ownerless until a documented trusted boundary is supplied.
- Tests cover owner lifetime, restart, concurrent sessions, retry continuity, and absence of accidental fallback.

## Decision

Do not modify `internal/api/server.go` or add automatic owner generation in the default HTTP server during T2-D. The default server has no verified trusted process boundary. The next implementation step may target either (a) a separate process-per-client deployment contract, or (b) an explicit in-process embedding API that supplies a provider and owner lifecycle. Connection-scoped binding may be used only in a labelled experiment and must not be presented as official process isolation.

## Effect on release conclusion

T2-D does not change transport behavior and does not establish strict official-client equivalence. The release classification remains `NOT STRICTLY EQUIVALENT`; T2-C remains `IMPLEMENTED_AND_ROLLBACK_VALIDATED` for its explicit propagation boundary.

## Evidence

- `artifacts/CPA-CLAUDECODE-T2D-HOST-BOUNDARY-AUDIT-20260905/MODIFIED_FILE.md`
- `artifacts/CPA-CLAUDECODE-T2D-HOST-BOUNDARY-AUDIT-20260905/VERIFICATION.txt`
- `artifacts/CPA-CLAUDECODE-DOCS-20260829/00-FIRST-READ.md`, section 95
