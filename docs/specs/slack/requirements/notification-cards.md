---
status: active
system: slack
created: 2026-09-19
owners: [flxbl-io]
---
# Notification cards

The Slack plugin owns the presentation of task attention notifications.

### REQ-SLACK-CARD-001: Readable attention cards

- **AC-SLACK-CARD-001.1:** Task notification content begins with issue number and full title when available, otherwise task title; an optional app identity strip may precede it. Workspace and repository appear beneath it, followed by a labeled attention status and concise reason. Unknown stage or CI results are omitted, never inferred.
- **AC-SLACK-CARD-001.2:** A primary Open in Workfloor button opens the exact task/session. View issue and View PR appear only when corresponding URLs are supplied. Long raw URLs do not appear in the visible card body.
- **AC-SLACK-CARD-001.3:** Desktop and mobile layouts wrap long titles and remain readable in light/dark themes. Screen-reader and notification fallback text includes the same information and destinations. Color is never the only status indicator.

### REQ-SLACK-CARD-002: Compatible delivery

- **AC-SLACK-CARD-002.1:** Existing plain-text callers remain supported. Card delivery preserves recipient targeting, duplicate suppression, conversation bindings and disabled previews/mentions; it does not claim thread replies are enabled when they are not.
- **AC-SLACK-CARD-002.2:** Invalid metadata is rejected before sending. Changed card content under the same delivery key conflicts; restarting does not resend old deliveries. Link buttons navigate only and never approve, resume or merge work.

Out of scope: digest redesign, general chat, host changes, retroactive edits and schedule changes.

## Proposed app identity addition

### REQ-SLACK-CARD-003: Identity on every notification card

- **AC-SLACK-CARD-003.1:** When app branding is configured, every new structured attention notification shows the app name and its small logo above the issue/title, including consecutive messages grouped by Slack. Existing title, status, actions and reply guidance remain readable on desktop and mobile.
- **AC-SLACK-CARD-003.2:** The app name remains visible without an image, and accessible fallback text includes the identity. Branding lookup/image failure shall not suppress a notification.
- **AC-SLACK-CARD-003.3:** Existing deliveries are not edited or resent when branding changes; replay of an existing notification key retains its recorded outcome. Unconfigured branding and plain-text notifications retain their current presentation.

This addition covers structured notification cards only, not general DM replies or personal digest layouts. It does not change Slack's native sender grouping.
