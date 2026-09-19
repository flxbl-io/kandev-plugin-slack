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

- [x] [01: Host conversation contract](task-01-host-conversations.md)
- [ ] [02: Plugin bridge and end-to-end proof](task-02-slack-bridge.md)

Open implementation PRs against flxbl-io repositories after relevant local tests. No PR to public upstream. Keep host and plugin CI evidence distinct; neither a mock test nor Slack auth.test proves live conversation delivery. Merge/deploy only with user authorization.

## End-to-end acceptance

Use a disposable host and fake Slack first: idle reply → original agent → same Slack thread; busy reply → queue → matched response; two simultaneous cards; restart/replay; revoked access; stale session; pending formal interaction; existing mention/task creation and notification regression. Then use a controlled pilot account on a disposable production card after deployment. Verify desktop/mobile Slack in-thread reply and Workfloor transcript attribution. Browser-only tests are not sufficient for Socket Mode delivery.

## Results

Implementation is prepared in paired FLXBL draft PRs. Local host policy,
persistence, queue and gRPC checks, plugin HTTP/Socket fixture tests, replay and
restart tests, race checks, and five-platform package verification pass. A real
packaged subprocess invokes the new Host API and refuses an unauthorized target
without delivering to Slack. See [validation](validation.md) for exact commands.

Integrated instance installation/upgrade and the real Slack pilot remain pending.
No production settings, user mappings, observer prompts, or schedule were changed.
