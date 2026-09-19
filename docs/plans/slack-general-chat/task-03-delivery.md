---
id: task-03-delivery
title: Verify packaged general chat and pilot
status: pending
wave: 3
depends_on: [task-02-dm-router]
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
  - AC-SLACK-ASSISTANT-002.2
  - AC-SLACK-ASSISTANT-002.3
  - AC-SLACK-ASSISTANT-003.2
  - AC-SLACK-ASSISTANT-003.3
  - AC-SLACK-ASSISTANT-003.4
system_design:
  - ../../specs/slack/system-design/general-chat.md
---
# 3. Package and pilot

Own package version, rollout docs and evidence. Add a package-level test fixture driving authenticated fake Slack events through an extracted plugin process, real disposable host authorization/receipts and deterministic utility/agent fixtures. Prove discovery, clarification, create/import/start, threaded reply, duplicate event, restart, revocation and blocked formal interaction before installing. Then run a controlled live pilot on a disposable card; never continue real work as a test. Keep separate evidence for fake-provider E2E, browser regressions and real Slack.

From plugin root, with HOST_REPO pointing at the reviewed host checkout:
```sh
python3 scripts/workfloor-manifest.py > /tmp/slack-general-chat-manifest.yaml
make package verify-package KANDEV_BACKEND="$HOST_REPO/apps/backend" GO_BUILD_FLAGS=-modfile=/tmp/slack-general-chat.mod MANIFEST=/tmp/slack-general-chat-manifest.yaml
```

Record the fixture invocation and artifact/source SHA. Open ready PRs only against flxbl-io/kandev and flxbl-io/kandev-plugin-slack after local checks. On the host PR use `/e2e tests/auth/prompt-queue-authors.spec.ts tests/auth/mobile-prompt-queue-authors.spec.ts`; require both actual test passes at the exact final head. Use the current PR workflow for CI/review findings.

Acceptance: (1) packaged end-to-end flow passes and upgrade preserves existing journals/config; (2) exact-head desktop/mobile regressions pass; (3) after authorized host deployment, one verified pilot person can discover and start a disposable task, receive its answer in the same Slack thread on desktop/mobile, and survive replay/restart without duplication. Report any untested layer explicitly.

Configure verified app/team IDs and user mapping, then enable general chat for the pilot. Keep the existing observer schedule unchanged. Roll back by disabling general_chat_enabled while preserving state. Do not approve a protected deployment. Results: pending.
