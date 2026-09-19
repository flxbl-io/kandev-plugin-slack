---
id: "02-slack-bridge"
title: "Bind Slack threads to existing card conversations"
status: pending
wave: 2
depends_on:
  - 01-host-conversations
plan: "plan.md"
requirements:
  - REQ-SLACK-CONVERSATIONS-001
  - REQ-SLACK-CONVERSATIONS-002
  - REQ-SLACK-CONVERSATIONS-003
acceptance_criteria:
  - AC-SLACK-CONVERSATIONS-001.1
  - AC-SLACK-CONVERSATIONS-001.2
  - AC-SLACK-CONVERSATIONS-001.3
  - AC-SLACK-CONVERSATIONS-001.4
  - AC-SLACK-CONVERSATIONS-002.1
  - AC-SLACK-CONVERSATIONS-002.2
  - AC-SLACK-CONVERSATIONS-002.3
  - AC-SLACK-CONVERSATIONS-002.4
  - AC-SLACK-CONVERSATIONS-003.1
  - AC-SLACK-CONVERSATIONS-003.2
  - AC-SLACK-CONVERSATIONS-003.3
  - AC-SLACK-CONVERSATIONS-003.4
system_design:
  - ../../specs/slack/system-design/card-conversations.md
---
# 02: Slack bridge and proof

## Scope and acceptance
Implement the bound notification tool, operator mapping, DM inbox, host dispatch, output relay, and upgrade-safe journals in this fork. Update the observer prompt in the private instance setup only after new tool validation; no private identities in this public repository.

1. Durable binding and verified Slack sender determine target; retries/restarts cannot duplicate prompts, route across tasks, or loop on bot messages.
2. Correlated final replies return in-thread; stale access and structured interactions get explicit guidance. The old notify_user contract and channel triage remain covered.
3. Package installs and survives upgrade in a disposable host, with successful bidirectional agent execution via real Host RPCs and Slack HTTP/Socket fixtures. Record the resulting PR head and artifact digest. Live pilot follows authorized deployment and scope reinstallation, with no automatic expansion to all users.

## Likely files
server/socket.go, notify.go, client.go, config.go, plugin.go, new conversation/inbox/outbox code and tests; manifest.yaml; slack-app-manifest.yaml; go.mod; Makefile; README.md; package CI; disposable-host smoke harness. Increment version and document required host build. Do not copy private core sources into public CI; use an approved SDK publication strategy.

## Verification
From the plugin checkout:
```sh
go test -race ./server -count=1
go vet ./server
go build ./server
make package-host verify-package-host
make package verify-package
git diff --check
```
The installed-artifact harness must drive a real hidden observer notification naming a different target card, a user reply through Socket Mode, and a mock agent's completed turn returned through Host RPC into that same Slack thread. Include duplicate envelopes, write failures, partial journals, two threads/one session, two cards, queued turn, lost events, restart, upgrade, invalid mappings, removed access, and rate limits. No credentials or live messages in fixtures.

For the authorized live pilot, preserve the paused schedule and first restrict to one operator and disposable card. Verify Slack desktop/mobile replies and matching Workfloor transcript, then repeat a recorded event and restart to prove no second prompt/message. Keep failures and unexercised paths explicit.

## Dependencies and risks
01-host-conversations. im:history/message.im and a Slack app reinstall are required before live input works. Public CI must build without private checkout dependencies. Slack send ambiguity must remain visible, not silently retried. Old unbound notifications cannot be inferred safely.

## Parallelism
sequential

## Exclusions
No general assistant, permission approvals, or production rollout in this work order.

## Results
Pending implementation.
