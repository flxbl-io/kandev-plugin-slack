---
id: task-01-personal-digests
title: Personal attention digest delivery
status: in_progress
wave: 1
depends_on: []
plan: plan.md
requirements: [REQ-SLACK-DIGEST-001, REQ-SLACK-DIGEST-002, REQ-SLACK-DIGEST-003]
acceptance_criteria: [AC-SLACK-DIGEST-001.1, AC-SLACK-DIGEST-001.2, AC-SLACK-DIGEST-002.1, AC-SLACK-DIGEST-002.2, AC-SLACK-DIGEST-002.3, AC-SLACK-DIGEST-003.1, AC-SLACK-DIGEST-003.2]
system_design: ../../specs/slack/system-design/personal-digests.md
---
# Implement personal digests

Own server/digest*.go, conversation dispatch/lifecycle wiring, SDK overlay, manifest and docs. Preserve existing sender and conversation semantics. Test with `go test -race ./server -count=1` and `go vet ./server`, using the prepared public SDK module replacement, then `make package verify-package`. Risks: revoked access, DST repeated hours, ambiguous Slack posts and missing host RPC. No native settings UI or broad permission bypass.
