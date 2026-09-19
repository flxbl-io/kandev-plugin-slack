---
id: task-02-dm-router
title: General Slack DM router and card handoff
status: pending
wave: 2
depends_on: [task-01-human-task-operations]
plan: plan.md
requirements:
  - REQ-SLACK-ASSISTANT-001
  - REQ-SLACK-ASSISTANT-002
  - REQ-SLACK-ASSISTANT-003
  - REQ-SLACK-ASSISTANT-004
acceptance_criteria:
  - AC-SLACK-ASSISTANT-004.1
  - AC-SLACK-ASSISTANT-004.2
  - AC-SLACK-ASSISTANT-001.3
  - AC-SLACK-ASSISTANT-002.1
  - AC-SLACK-ASSISTANT-002.3
  - AC-SLACK-ASSISTANT-003.1
  - AC-SLACK-ASSISTANT-003.2
  - AC-SLACK-ASSISTANT-003.3
  - AC-SLACK-ASSISTANT-003.4
system_design:
  - ../../specs/slack/system-design/general-chat.md
---
# 2. General DM router

Own server/assistant*.go and tests, conversation dispatch/store/config, public additive SDK overlay, manifest and README in this standalone FLXBL plugin. Preserve digest commands, bound conversations and old notification transport. Do not copy the old triage “pick first workspace” fallback.

Acceptance: (1) typed utility-agent intents use only authorized candidates and sender/thread-scoped history; (2) explicit create/start runs once, ambiguous or hypothetical requests do not mutate, and card handoff never repeats the first instruction; (3) unknown users, replay/restart, changed permissions, unavailable host/agent and Slack rate limits have tested outcomes. Cover two users, two threads and same-number issues in different repositories with TDD.

Prepare a task-specific temporary Go modfile against the pinned public SDK plus updated sdk/apply.py overlay; do not depend on private core code. From plugin root:
```sh
go test -race -modfile=/tmp/slack-general-chat.mod ./server -count=1
go vet -modfile=/tmp/slack-general-chat.mod ./server
git diff --check
```

Dependency: task 1 contract. Risk: stale conversational references or model-generated identifiers causing unintended actions. Results: pending.
