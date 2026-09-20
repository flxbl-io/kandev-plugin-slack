---
created: 2026-09-20
status: draft
requirements: [REQ-SLACK-CARD-003]
system_design: [../../specs/slack/system-design/notification-cards.md]
legacy_specs: []
---
# Flux identity on notification cards

Show a small Flux logo and name inside every new attention card so consecutive Slack messages remain identifiable. The standalone Slack plugin owns this presentation. Use operator settings for branding and native Slack blocks; no host change is needed.

- [ ] [Task 01: Configure and render card identity](task-01-card-identity.md)

Scope is notification cards only. Existing conversations, delivery records, schedules and card actions retain their behavior. General chat replies and personal digest layouts are excluded.

Verification combines targeted renderer/delivery tests and package checks with two consecutive controlled Slack notifications viewed on desktop and mobile. A mocked provider response is not visual Slack evidence. Publish the implementation PR to flxbl-io/kandev-plugin-slack after local checks; production installation is a separate delivery step.

Design checkpoint only: no runtime files changed and no plugin installed. Verification results: pending implementation.
