---
id: task-01-card-identity
title: Configure and render card identity
status: pending
wave: 1
depends_on: []
plan: plan.md
requirements: [REQ-SLACK-CARD-003]
acceptance_criteria: [AC-SLACK-CARD-003.1, AC-SLACK-CARD-003.2, AC-SLACK-CARD-003.3]
system_design: [../../specs/slack/system-design/notification-cards.md]
---
# Task 01: Card identity

Implement the proposed identity strip in the existing shared notification renderer. Add the two optional plugin settings, concise configuration documentation, and a package patch-version bump. Do not change host code or other message renderers.

Acceptance:
1. Both structured notification tools show configured name/icon with safe text, correct block order and matching accessible fallback; missing/invalid branding degrades without suppressing delivery.
2. Existing text-only behavior, old-key replay after a brand change, changed-card conflicts, session binding, buttons and unknown-delivery handling retain their contracts.
3. Built package passes disposable install/upgrade checks; two controlled notification cards demonstrate the repeated identity in real Slack desktop/mobile. Record exact artifact, PR head and terminal CI separately from live proof.

Likely files: `server/notification_card.go`, `server/notification_card_test.go`, `server/notify_test.go`, `server/endtoend_test.go`, `manifest.yaml`, `Makefile`, `README.md`, `VENDORING.md`. Add focused tests before changed logic. Inspect nearby tests before choosing final test names.

Verification from plugin root (sibling SDK checkout must have the existing public SDK overlay applied):
```sh
go test -race ./server -run 'Test(Notification|Notify|EndToEnd)' -count=1
go vet ./server
make build
python3 scripts/workfloor-manifest.py > /tmp/slack-branding-manifest.yaml
make package-host verify-package-host MANIFEST=/tmp/slack-branding-manifest.yaml
make package verify-package MANIFEST=/tmp/slack-branding-manifest.yaml
git diff --check
```

Inspect rendered Block Kit with long titles, all attention states, icon absent/invalid, and config changed between original send and replay. Reuse existing disposable package smoke harness. Real Slack verification must preserve live ledger/config and notify only the existing Azlam pilot; do not replay old notifications merely for appearance. No artificial Workfloor browser E2E is needed for a Slack-only layout; verify the native plugin config fields if their host rendering is affected. CI must target the implementation head.

Dependencies: current plugin master and sibling `../kandev/apps/backend` SDK overlay. Risks: stale operator icon URL, inaccessible images, Block Kit layout drift and accidental fingerprint changes. Parallelism: sequential. Results: pending.
