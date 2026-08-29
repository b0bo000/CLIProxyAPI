# AGENTS.md

Go 1.26+ proxy server providing OpenAI/Gemini/Claude/Codex compatible APIs with OAuth and round-robin load balancing.

## Repository
- GitHub: https://github.com/router-for-me/CLIProxyAPI

## Commands
```bash
gofmt -w . # Format (required after Go changes)
go build -o cli-proxy-api ./cmd/server # Build
go run ./cmd/server # Run dev server
go test ./... # Run all tests
go test -v -run TestName ./path/to/pkg # Run single test
go build -o test-output ./cmd/server && rm test-output # Verify compile (REQUIRED after changes)
```
- Common flags: `--config <path>`, `--tui`, `--standalone`, `--local-model`, `--no-browser`, `--oauth-callback-port <port>`

## Config
- Default config: `config.yaml` (template: `config.example.yaml`)
- `.env` is auto-loaded from the working directory
- Auth material defaults under `auths/`
- Storage backends: file-based default; optional Postgres/git/object store (`PGSTORE_*`, `GITSTORE_*`, `OBJECTSTORE_*`)

## Architecture
- `cmd/server/` — Server entrypoint
- `internal/api/` — Gin HTTP API (routes, middleware, modules)
- `internal/api/modules/amp/` — Amp integration (Amp-style routes + reverse proxy)
- `internal/thinking/` — Main thinking/reasoning pipeline. `ApplyThinking()` (apply.go) parses suffixes (`suffix.go`, suffix overrides body), normalizes config to canonical `ThinkingConfig` (`types.go`), normalizes and validates centrally (`validate.go`/`convert.go`), then applies provider-specific output via `ProviderApplier`. Do not break this "canonical representation → per-provider translation" architecture.
- `internal/runtime/executor/` — Per-provider runtime executors (incl. Codex WebSocket)
- `internal/translator/` — Provider protocol translators (and shared `common`)
- `internal/registry/` — Model registry + remote updater (`StartModelsUpdater`); `--local-model` disables remote updates
- `internal/store/` — Storage implementations and secret resolution
- `internal/managementasset/` — Config snapshots and management assets
- `internal/cache/` — Request signature caching
- `internal/watcher/` — Config hot-reload and watchers
- `internal/wsrelay/` — WebSocket relay sessions
- `internal/usage/` — Usage and token accounting
- `internal/tui/` — Bubbletea terminal UI (`--tui`, `--standalone`)
- `sdk/cliproxy/` — Embeddable SDK entry (service/builder/watchers/pipeline)
- `test/` — Cross-module integration tests

## Code Conventions
- Keep changes small and simple (KISS)
- Comments in English only
- If editing code that already contains non-English comments, translate them to English (don’t add new non-English comments)
- For user-visible strings, keep the existing language used in that file/area
- New Markdown docs should be in English unless the file is explicitly language-specific (e.g. `README_CN.md`)
- As a rule, do not make standalone changes to `internal/translator/`. You may modify it only as part of broader changes elsewhere.
- If a task requires changing only `internal/translator/`, run `gh repo view --json viewerPermission -q .viewerPermission` to confirm you have `WRITE`, `MAINTAIN`, or `ADMIN`. If you do, you may proceed; otherwise, file a GitHub issue including the goal, rationale, and the intended implementation code, then stop further work.
- `internal/runtime/executor/` should contain executors and their unit tests only. Place any helper/supporting files under `internal/runtime/executor/helps/`.
- Follow `gofmt`; keep imports goimports-style; wrap errors with context where helpful
- Do not use `log.Fatal`/`log.Fatalf` (terminates the process); prefer returning errors and logging via logrus
- Shadowed variables: use method suffix (`errStart := server.Start()`)
- Wrap defer errors: `defer func() { if err := f.Close(); err != nil { log.Errorf(...) } }()`
- Use logrus structured logging; avoid leaking secrets/tokens in logs
- Avoid panics in HTTP handlers; prefer logged errors and meaningful HTTP status codes
- Timeouts are allowed only during credential acquisition; after an upstream connection is established, do not set timeouts for any subsequent network behavior. Intentional exceptions that must remain allowed are the Codex websocket liveness deadlines in `internal/runtime/executor/codex_websockets_executor.go`, the wsrelay session deadlines in `internal/wsrelay/session.go`, the management APICall timeout in `internal/api/handlers/management/api_tools.go`, and the `cmd/fetch_antigravity_models` utility timeouts

## Local Fork Maintenance

This checkout is the user-maintained fork of CLIProxyAPI.

- `upstream` is the official repository `router-for-me/CLIProxyAPI`; treat it as read-only.
- `origin` is the user fork `b0bo000/CLIProxyAPI`; push user branches there and open pull requests from those branches.
- Keep `main` as a clean mirror of `upstream/main`. Do not develop directly on `main`.
- Keep dated `baseline/*` branches immutable for reproducibility.
- Use one short-lived `feat/*` branch per behavior change. Use `integration/*` only to combine tested features, and `release/*` only for a deployed private build.
- Synchronize official changes with `git fetch upstream --prune --tags`, fast-forward `main` from `upstream/main`, then rebase unshared feature branches onto `main`.
- Never force-push `main`. Use `--force-with-lease` only for a private feature branch after a rebase.
- Do not overwrite a fork's existing history to synchronize it. Create a new branch or merge deliberately when histories diverge.
- Keep experiments, packet captures, OAuth material, proxy credentials, emails, and generated reports outside this source repository.

## Validation Policy

- Do not run `go test`, `go build`, Docker builds, or other compilation-heavy commands on the local workstation unless the user explicitly enables local builds.
- Use the repository GitHub Actions workflows for tests, builds, path checks, and release artifacts. Record the workflow URL or run ID with each release candidate.
- Local validation is limited to read-only Git checks, formatting inspection, static file checks, and targeted non-build analysis.
- Before any CPA/Claude Code experiment, analysis, modification, or report, read `C:\Users\Administrator\gpt-5.6-instruct\artifacts\CPA-ClaudeCode-FIRST-READ-20260825\MODIFIED_FILE.md`.

## Claude Compatibility Change Boundaries

- Treat transport ownership, TLS session policy, and application continuity state as separate changes with separate tests and commits.
- Do not hard-code model, Tools, system prompt, workspace paths, beta sets, CCH values, device IDs, session IDs, request IDs, fixed socket counts, or fixed TLS resumption counts.
- Preserve caller-provided dynamic metadata unless a lifecycle-aware implementation and regression tests justify a change.
- Do not claim strict client equivalence from matching UA, JA3, ALPN, or header values alone; record the observation boundary and confounders.
- A process-owned transport scope must use a trustworthy owner identity. Do not treat an arbitrary client-supplied header as a verified process identity.
- Keep strict or experimental TLS and continuity behavior opt-in with backward-compatible defaults until same-scenario evidence and CI coverage exist.
- Prefer generic, provider-neutral abstractions for upstream pull requests. Keep product-specific experiments in private feature branches.

## Source Change Sequence

1. Add or update focused tests and documentation on a `feat/*` branch.
2. Implement the smallest behavior change in the owning package; keep helper code under `internal/runtime/executor/helps/`.
3. Push the branch to `origin` and wait for GitHub Actions to finish.
4. Review the diff and workflow results, then merge into the user fork's `main` only after validation.
5. Rebase remaining unshared feature branches onto the updated `main`.
6. Create a dated `release/*` branch or tag only after the integration behavior and rollback path are documented.

## Compatibility Change Control

Before any Claude compatibility experiment, analysis, source edit, or report, read:

`C:\Users\Administrator\gpt-5.6-instruct\artifacts\CPA-ClaudeCode-FIRST-READ-20260825\MODIFIED_FILE.md`

Use `docs/CPA-COMPATIBILITY-CHANGE-CONTROL.md` as the single change-control index. The report hierarchy is:

1. `FIRST-READ`: current verified factual baseline and evidence boundaries. If new verified evidence conflicts with it, update `FIRST-READ` first.
2. `STRICT-MATRIX`: machine-readable signal inventory, classification, and required next evidence.
3. `FULL-FINGERPRINT-REANALYSIS`: cross-axis interpretation and attribution limits.
4. `DETAILED-EXPLANATION`: human-readable explanation of the current state.
5. B-gap, startup-graph, raw-evidence, rerun, and other reports: scoped evidence appendices; they cannot silently override the first two levels.

Every source change requires a change card in `docs/change-cards/` before implementation. A card must name exactly one signal or one explicitly scoped group of signals, its current classification, evidence paths, hypothesis, allowed files, prohibited changes, expected behavior, CI validation, and rollback command. A change without a card is out of process.

The pre-edit gate is mandatory:

- Confirm the branch is a `feat/*` branch and the worktree is understood.
- Record the exact Signal ID from `STRICT-MATRIX.json` and its current classification.
- Record host, egress, account, version, workload, and other confounders before interpreting a delta.
- State what is being changed and what must remain untouched.
- Define a same-scenario test and a falsifying result; do not treat a matching UA, JA3, header value, or one successful request as strict equivalence.
- Define the GitHub Actions workflow and artifact paths that will validate the change.
- Define and test rollback on a separate copy before calling the change releasable.

Do not start implementation when the card is missing, the signal is `UNTESTED` without a measurement plan, or the proposed edit hard-codes caller-generated fields such as model, Tools, system prompt, paths, beta values, CCH, device IDs, session IDs, request IDs, socket counts, or TLS resumption counts.
