---
status: draft
system: slack
specification_version: 1
migration: complete
owners:
  - flxbl-io
---
# Slack work management

The standalone Slack plugin owns the binding between a Slack thread and a Workfloor conversation, provider delivery, and recovery. Workfloor owns human identity, authorization, task/session state, and prompt execution. The retired in-tree Slack integration is not extended.

- [Requirements](requirements/card-conversations.md)
- [System design](system-design/card-conversations.md)
- [Delivery plan](../../plans/slack-conversations/plan.md)

Existing channel mentions and slash-command task creation remain supported. Card conversations remain the default. General chat is a separately gated proposed extension; formal permissions remain in Workfloor.

- [Personal digest requirements](requirements/personal-digests.md) and [design](system-design/personal-digests.md).
- [General chat requirements](requirements/general-chat.md), [design](system-design/general-chat.md), and [delivery plan](../../plans/slack-general-chat/plan.md).

- [Notification card requirements](requirements/notification-cards.md)
- [Notification card design](system-design/notification-cards.md)
- [Notification card plan and preview](../../plans/slack-notification-cards/plan.md)


- [Per-card app identity plan](../../plans/slack-card-branding/plan.md)
