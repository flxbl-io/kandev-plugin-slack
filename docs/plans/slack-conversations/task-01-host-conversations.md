---
id: "01-host-conversations"
title: "Authorize and deduplicate external conversation replies"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SLACK-CONVERSATIONS-001
  - REQ-SLACK-CONVERSATIONS-002
  - REQ-SLACK-CONVERSATIONS-003
acceptance_criteria:
  - AC-SLACK-CONVERSATIONS-001.1
  - AC-SLACK-CONVERSATIONS-001.2
  - AC-SLACK-CONVERSATIONS-001.4
  - AC-SLACK-CONVERSATIONS-002.1
  - AC-SLACK-CONVERSATIONS-002.3
  - AC-SLACK-CONVERSATIONS-003.1
  - AC-SLACK-CONVERSATIONS-003.3
system_design:
  - ../../specs/slack/system-design/card-conversations.md
---
# 01: Host conversation contract

## Scope and acceptance
Implement in a fresh flxbl-io/kandev worktree, following its scoped instructions. Own additive public SDK/proto contracts, capability checks, typed authorization/target resolution, durable external-message acceptance and correlation, and per-actor output reads. Existing Messages().Send remains compatible.

1. Wrong actor, stale target, revoked access, and formal interactions cause zero unauthorized execution/disclosure. Enforce before dispatch, queue consumption, and output read.
2. Duplicate event/content returns the original durable receipt; conflicting content is rejected. Test crashes at acceptance, queueing, dispatch, and completion without duplicate agent work.
3. Idle resume, active queue, fast completion, and multiple threads identify exact message/turn output with human and plugin provenance.

## Likely files
apps/backend/pkg/pluginsdk; apps/backend/proto/kandev/plugin/v1/plugin.proto; apps/backend/internal/plugins/host_write.go and new scoped handlers; apps/backend/internal/backendapp/adapters_plugin_messenger.go; task/orchestrator queue persistence as needed; docs/public/plugins-authoring.md, plugins-manifest.md, Host contract docs, VENDORING.md. Add core-owned contract specifications without copying this integration's requirements.

## Verification
From the Kandev checkout, write failing focused tests first, then run:
```sh
cd apps/backend
go test -race ./pkg/pluginsdk ./internal/plugins/manifest ./internal/plugins ./internal/backendapp -run 'ExternalConversation|ExternalMessage|Plugin.*Message|Plugin.*Conversation' -count=1
```
Run affected repository/queue tests for newly added durable receipt paths as named in the implementation PR. From checkout root:
```sh
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
git diff --check
```
Ensure test filters actually select the new tests. Exercise real gRPC transport and capability denial, not just direct helper calls.

## Dependencies and risks
None. Current SDK has no human-aware send receipt. Never substitute plugin-global reads or administrator credentials. Recheck current authorization policy and queue persistence during implementation; stop if the design cannot preserve the stated invariants. Keep host code private to its owning repository.

## Parallelism
sequential

## Exclusions
No general assistant, permission approvals, or production rollout in this work order.

## Results
Implemented in the paired FLXBL host branch. Focused race tests, capability denial,
human/target policy checks, SQLite persistence/replay, real bidirectional gRPC with
a mock agent, queue provenance and adjacent QueueUserPrompt tests pass. Normal
host commit hooks and documentation checks pass. PostgreSQL behavior coverage is
present but skipped locally because no database runtime is available.
