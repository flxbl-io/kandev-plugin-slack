---
id: task-01-notification-cards
title: Render and deliver notification cards
status: done
wave: 1
depends_on: []
plan: plan.md
requirements: [REQ-SLACK-CARD-001, REQ-SLACK-CARD-002]
acceptance_criteria: [AC-SLACK-CARD-001.1, AC-SLACK-CARD-001.2, AC-SLACK-CARD-001.3, AC-SLACK-CARD-002.1, AC-SLACK-CARD-002.2]
system_design: [../../specs/slack/system-design/notification-cards.md]
---
# Notification card implementation

Implement optional card metadata in both notification tools and one plugin-owned Block Kit renderer. Update the automation prompt only after the package passes tests. No host, digest, general-chat, schedule or card-state changes.

Acceptance:
1. Exact long title, safe metadata, navigation buttons and matching accessible fallback work on Slack desktop/mobile.
2. Existing text-only callers, conversation bindings, old journal replay and ambiguous-post suppression retain behavior. Tests cover changed-card key conflicts, unsafe URLs/mentions, optional PR omission and action acknowledgements without mutations.
3. Package passes install smoke in a disposable instance before the production upgrade; one controlled DM proves real blocks and button destinations. Preserve live configuration/journals and avoid notifying other users during the pilot.

Likely files: server/notify.go, server/conversation_notify.go, new card renderer/tests, server/socket_test.go, manifest.yaml, Makefile, README.md, scripts/workfloor-manifest.py if it transforms agent schemas. Operational observer prompt lives outside this repository. Re-check upstream/PR #4 state before branching or opening the implementation PR.

Verification (existing local SDK overlay; confirm paths at execution):
```sh
go test -race -modfile=/tmp/slack-digest.mod ./server -count=1
go vet -modfile=/tmp/slack-digest.mod ./server
python3 scripts/workfloor-manifest.py > /tmp/slack-cards-manifest.yaml
make package verify-package KANDEV_BACKEND=/Users/azlam/.codex/worktrees/plugin-attention-read/kandev/apps/backend GO_BUILD_FLAGS=-modfile=/tmp/slack-digest.mod MANIFEST=/tmp/slack-cards-manifest.yaml
git diff --check
```

Record disposable install identity and results, artifact hash, exact PR head/CI, production version, unchanged settings, single DM receipt and actual Slack visual/button proof separately. API receipt alone is not visual proof.

Dependencies: existing 0.4.1 fix. Risks: malformed metadata, Slack text limits, fingerprint compatibility, missing configured Workfloor origin. Sequential. Results: [verification](verification.md).
