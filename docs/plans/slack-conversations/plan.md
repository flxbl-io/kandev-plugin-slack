---
created: 2026-09-19
status: draft
requirements:
  - REQ-SLACK-CONVERSATIONS-001
  - REQ-SLACK-CONVERSATIONS-002
  - REQ-SLACK-CONVERSATIONS-003
system_design:
  - ../../specs/slack/system-design/card-conversations.md
legacy_specs: []
---
# Work from Slack: delivery plan

Reply to a card's notification thread, continue that card's existing agent, and receive the response in-thread. [Design and limitations](../../specs/slack/system-design/card-conversations.md).

Build the human-aware, retry-safe host path first, then the plugin bridge. Keep the current production notification plugin working while this is developed. Scope is existing-card free-text conversation; formal questions and permissions retain their Workfloor controls.

- [ ] [01: Host conversation contract](task-01-host-conversations.md)
- [ ] [02: Plugin bridge and end-to-end proof](task-02-slack-bridge.md)

Open implementation PRs against flxbl-io repositories after relevant local tests. No PR to public upstream. Keep host and plugin CI evidence distinct; neither a mock test nor Slack auth.test proves live conversation delivery. Merge/deploy only with user authorization.

## End-to-end acceptance

Use a disposable host and fake Slack first: idle reply → original agent → same Slack thread; busy reply → queue → matched response; two simultaneous cards; restart/replay; revoked access; stale session; pending formal interaction; existing mention/task creation and notification regression. Then use a controlled pilot account on a disposable production card after deployment. Verify desktop/mobile Slack in-thread reply and Workfloor transcript attribution. Browser-only tests are not sufficient for Socket Mode delivery.

## Results

Design only. Current source/SDK boundaries inspected; no implementation, tests of new behavior, Slack scope change, or production deployment performed. Notification schedule remains as previously paused. The main delivery dependency is a new Host contract, rather than a Slack-only configuration toggle.
