---
status: active
system: slack
created: 2026-09-19
owners: [flxbl-io]
---
# Notification cards

The Slack plugin owns the presentation of task attention notifications.

### REQ-SLACK-CARD-001: Readable attention cards

- **AC-SLACK-CARD-001.1:** Task notifications begin with issue number and full title when available, otherwise task title. Workspace and repository appear beneath it, followed by a labeled attention status and concise reason. Unknown stage or CI results are omitted, never inferred.
- **AC-SLACK-CARD-001.2:** A primary Open in Workfloor button opens the exact task/session. View issue and View PR appear only when corresponding URLs are supplied. Long raw URLs do not appear in the visible card body.
- **AC-SLACK-CARD-001.3:** Desktop and mobile layouts wrap long titles and remain readable in light/dark themes. Screen-reader and notification fallback text includes the same information and destinations. Color is never the only status indicator.

### REQ-SLACK-CARD-002: Compatible delivery

- **AC-SLACK-CARD-002.1:** Existing plain-text callers remain supported. Card delivery preserves recipient targeting, duplicate suppression, conversation bindings and disabled previews/mentions; it does not claim thread replies are enabled when they are not.
- **AC-SLACK-CARD-002.2:** Invalid metadata is rejected before sending. Changed card content under the same delivery key conflicts; restarting does not resend old deliveries. Link buttons navigate only and never approve, resume or merge work.

Out of scope: digest redesign, general chat, host changes, retroactive edits and schedule changes.
