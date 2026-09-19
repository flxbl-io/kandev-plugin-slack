---
status: draft
system: slack
specification_version: 1
migration: complete
owners:
  - flxbl-io
---
# Slack conversations

The standalone Slack plugin owns the binding between a Slack thread and a Workfloor conversation, provider delivery, and recovery. Workfloor owns human identity, authorization, task/session state, and prompt execution. The retired in-tree Slack integration is not extended.

- [Requirements](requirements/card-conversations.md)
- [System design](system-design/card-conversations.md)
- [Delivery plan](../../plans/slack-conversations/plan.md)

Existing channel mentions and slash-command task creation remain supported. This first release adds conversations on existing cards, not a general assistant or a replacement permission UI.
