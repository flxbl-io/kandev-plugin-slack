---
created: 2026-09-19
status: complete
requirements: [REQ-SLACK-CARD-001, REQ-SLACK-CARD-002]
system_design: [../../specs/slack/system-design/notification-cards.md]
legacy_specs: []
---
# Slack notification cards

Use native Slack cards with the full issue title, workspace/repository context, a clear attention reason and destination buttons. [Open the visual preview](preview.html); it uses synthetic sample content. The native payload is in [sample-blocks.json](sample-blocks.json).

[Design](../../specs/slack/system-design/notification-cards.md) · [Requirements](../../specs/slack/requirements/notification-cards.md)

- [x] [01: Render, test and deliver notification cards](task-01-notification-cards.md)

Validation: targeted formatter/delivery tests, full plugin server race tests, packaging, then one controlled real DM with desktop/mobile visual and destination checks. No host UI suite is needed. Long titles, absent PR links, literal mentions, duplicate keys and preserved old journals are the main regression cases.

Implementation and 0.5.0 rollout complete. See [verification](verification.md) for separate local, package, live-delivery and visual evidence. CI results are recorded in the PR.
