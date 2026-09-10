# Process-Per-Client Deployment Contract

## Purpose

This deployment keeps one CPA process and one upstream transport/TLS cache per
downstream logical client. It is the recommended production boundary when the
caller reaches CPA through an ordinary HTTP API and no trusted in-process host
can provide an opaque `ClaudeCodeTransportOwner`.

## Instance contract

Every client instance must have all of the following isolated resources:

- one CPA process/container;
- one host API port mapped to the container's internal `8317` port;
- one `config.yaml` file;
- one auth directory;
- one logs directory;
- one plugins directory when plugins are enabled;
- one stable container/process name.

The same upstream credential may intentionally be installed in two isolated
instances. The credential identity remains the same, while the process-local
transport and TLS state remain separate.

## Provided template

`docker-compose.process-per-client.yml` defines two instances:

| Instance | Default host port | Container name | Host state root |
|---|---:|---|---|
| `cpa-client-a` | `8318` | `cli-proxy-api-client-a` | `./instances/client-a/` |
| `cpa-client-b` | `8319` | `cli-proxy-api-client-b` | `./instances/client-b/` |

Set `CPA_A_*` and `CPA_B_*` environment variables to relocate paths or ports.
The template intentionally does not include credentials or a management secret.

## Downstream routing rules

1. Configure each downstream client to use its own CPA host port.
2. Keep a client on the same instance for the full session, including retries,
   resume, compact, and subagent activity.
3. Do not round-robin one session across instances.
4. Do not mount one auth directory or one writable config file into multiple
   instances.
5. A reverse proxy may front the instances only when it uses deterministic
   client-to-instance routing and preserves that mapping for a session.

## Lifecycle rules

- A process start creates a new process-local transport owner boundary.
- A graceful stop closes that instance's listener and upstream connections.
- A process restart creates a new transport/TLS cache; it does not reuse the
  old process's in-memory owner or connections.
- Updating one instance's config/auth/logs must not write into another
  instance's directories.

## Scope and non-goals

This contract isolates process-level transport state. It does not hard-code or
rewrite User-Agent, body, tools, system prompts, model, beta values, CCH, or
session/request IDs. Those remain caller- and credential-lifecycle data as
defined by the canonical audit. The default `docker-compose.yml` remains
unchanged.
