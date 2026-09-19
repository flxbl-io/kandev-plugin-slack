# Local validation: 2026-09-19

## Plugin

- `go test -race ./server -count=1`: passed against the pinned public SDK plus
  `sdk/apply.py`. Local runs used an equivalent isolated module replacement.
- `go vet ./server`: passed.
- Existing notification and channel/slash triage regressions pass.
- Conversation cases cover explicit card routing, restart/replay, one outstanding
  reply per thread, malformed/foreign/bot events, persistence failure before
  Socket acknowledgement, uncertain delivery without repost, and Retry-After.
- `make package verify-package` using the Workfloor manifest and updated host
  packager: all five binaries build, required assets and checksums pass.
- Final source revision and archive SHA256 are recorded in the PR after packaging.
- A temporary supervisor harness launched the extracted macOS ARM64 binary,
  completed the plugin handshake, invoked notify_task_user over gRPC, and received
  its ResolveConversation call over the host broker. A refused target returned
  target_unavailable with zero Slack sends. The fixture waits for Host injection
  before invocation, as the real host must do during startup.

## Host

Focused race tests cover active/mapped human checks, assignment and session
changes, workspace permission loss, formal interaction refusal, idempotent
acceptance, changed-content rejection, separate queue receipts, completed-turn
output filtering, and neighboring QueueUserPrompt behavior. A disposable SQLite
host plus real Host gRPC and mock executor covers immediate completion before
Submit returns and repeated submission without duplicate execution. The host's
normal lint/commit hooks and docs checks pass.

The environment-gated PostgreSQL test is present but was not executed locally;
the Docker daemon was unavailable. No browser or production execution proof is
claimed. Full instance installation/upgrade and a controlled real Slack pilot
remain required before rollout. Keep conversations disabled and the notification
schedule paused until those deployment checks pass.

The first CI test job passed, while the separate build job initially omitted
SDK preparation. The build and release jobs now apply the same public SDK
extension before compiling; subsequent CI evidence must use the updated head.
