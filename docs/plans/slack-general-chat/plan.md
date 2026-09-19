---
created: 2026-09-19
status: draft
requirements:
  - REQ-SLACK-ASSISTANT-001
  - REQ-SLACK-ASSISTANT-002
  - REQ-SLACK-ASSISTANT-003
  - REQ-SLACK-ASSISTANT-004
system_design:
  - ../../specs/slack/system-design/general-chat.md
legacy_specs: []
---
# General Workfloor DM chat

Make the Workfloor app useful before a notification arrives. Examples:

- “What needs my attention?” — a personal cross-workspace list.
- “What is happening with flxbl-io/sfp-pro#2375?” — current status and card link.
- “Start issue #123 in sf-core-staging” — reuse/import the verified issue and start the correct workflow.
- “Create a task in Codev to investigate the failing validation” — ask only for missing routing choices, then create work.
- “The second one” — continue within the same private thread.

Keep natural-language routing, thread context and Slack delivery in the plugin. Add the missing human-scoped discovery and durable creation/start boundary to the host. Expose the general-chat agent-profile selector in plugin settings and reuse the host utility execution tier plus existing card agents, rather than spinning up an executor for every DM. See the [requirements](../../specs/slack/requirements/general-chat.md) and [design](../../specs/slack/system-design/general-chat.md).

## Delivery order

- [ ] [1. Human-scoped host operations](task-01-human-task-operations.md)
- [ ] [2. General DM router and card handoff](task-02-dm-router.md)
- [ ] [3. Package, regressions and live pilot](task-03-delivery.md)

## Verification and rollout

Work order 1 proves authorization and durable mutation behavior. Work order 2 proves natural-language routing, thread isolation and compatibility. Work order 3 exercises the packaged plugin against a disposable host and Slack fixture, then the normal exact-head desktop/mobile author regressions and a controlled real Slack pilot. Use FLXBL repositories only. Preserve current config, journals and notification schedules; general chat starts disabled until host capability, identity mapping and pilot are verified. No automatic protected deployment approvals.

## Results

Design only. Implementation and runtime changes have not started. Main risk: treating plugin-wide reads/writes or a model choice as permission to act for a human. The host operation contract resolves that risk and must precede plugin activation.
