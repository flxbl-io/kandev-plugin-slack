---
id: task-01-human-task-operations
title: Human-scoped task discovery and operations
status: pending
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-SLACK-ASSISTANT-001
  - REQ-SLACK-ASSISTANT-002
  - REQ-SLACK-ASSISTANT-003
  - REQ-SLACK-ASSISTANT-004
acceptance_criteria:
  - AC-SLACK-ASSISTANT-004.1
  - AC-SLACK-ASSISTANT-004.2
  - AC-SLACK-ASSISTANT-001.1
  - AC-SLACK-ASSISTANT-001.2
  - AC-SLACK-ASSISTANT-002.2
  - AC-SLACK-ASSISTANT-002.3
  - AC-SLACK-ASSISTANT-003.1
  - AC-SLACK-ASSISTANT-003.2
system_design:
  - ../../specs/slack/system-design/general-chat.md
---
# 1. Human-scoped task operations

Own the additive SDK/gRPC contract and host adapter/persistence in flxbl-io/kandev. Base on merged Attention support; use a fresh worktree and scoped instructions. Define host-owned API requirements/design and link this plugin design, update public Host documentation and VENDORING. No Slack routing logic or credentials in core.

Acceptance: (1) actor-scoped discovery handles sessionless, inaccessible, revoked and ambiguous issue targets; (2) create/import/start uses ordinary services with human attribution and durable fingerprinted receipts, including watcher races, restart and partial-launch failures; (3) existing conversation permission rules and old-host refusal remain intact. Use TDD.

Likely files: apps/backend/pkg/pluginsdk, proto/kandev/plugin/v1/plugin.proto, internal/plugins, internal/backendapp/adapters_plugin*, existing task/issue creation and execution services. Reuse the external-conversation persistence pattern. Do not claim exactly-once execution from a plugin file lock. Exact service integration points must be resolved before modifying them.

From apps/backend:
```sh
go test -race ./internal/backendapp ./internal/plugins/... ./pkg/pluginsdk -run 'ExternalAssistant|ExternalConversation|Attention' -count=1
golangci-lint run ./... --new-from-rev=origin/main --timeout=5m
```
From host root: `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`, `python3 scripts/lint-spec-files.py --all`, `git diff --check`.

Dependencies: none. Risk: leaking instance-wide candidates or duplicate starts across receipt/task transactions. Results: pending.
