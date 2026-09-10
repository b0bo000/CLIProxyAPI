# CC-20260909 Session-Scoped Transport

## Evidence

The same-host TLS control showed that a credential-scoped CPA transport shares
HTTP connections and TLS tickets across independent downstream Claude Code
processes. Official direct resumption was conditional and naturally confined
to each CLI process. The default CPA HTTP server cannot observe the real client
PID, so a Claude session is the closest available lifecycle approximation.

## Contract

`claude-code.session-scoped-transport` is disabled by default. When enabled:

- one credential, proxy, TLS policy, and valid Claude session reuse one
  upstream HTTP transport and its TLS ticket cache;
- different sessions receive different transports even when one downstream
  API key routes both sessions to the same upstream credential;
- different credentials remain isolated by the T1 credential scope;
- a missing, invalid, or oversized session gets an ephemeral transport and
  cannot enter a shared ownerless pool;
- an opaque owner supplied by a trusted embedding host takes precedence over
  the session approximation.

The in-memory lookup hashes the session value before using it as a bounded LRU
key. It does not log or persist the raw value. Restarting CPA clears both the
owner map and all transport/TLS state.

## Boundary

This setting approximates one Claude Code process with one session. It cannot
prove physical device or process identity, and it can over-isolate a real CLI
process that deliberately owns multiple sessions. The option therefore remains
explicit rather than changing the upstream-compatible default.
