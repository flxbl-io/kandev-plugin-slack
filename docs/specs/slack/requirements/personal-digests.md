---
status: draft
system: slack
created: 2026-09-19
owners: [flxbl-io]
---
# Personal attention digests

### REQ-SLACK-DIGEST-001: Personal control

- **AC-SLACK-DIGEST-001.1:** A mapped human can DM the bot to enable a digest at their chosen HH:MM, IANA timezone and daily/weekdays schedule, inspect it or disable it. Only the authenticated Slack sender's preference is changed. Unmapped and foreign-team messages cannot configure delivery.
- **AC-SLACK-DIGEST-001.2:** Preferences survive restarts and upgrades; changed settings supersede older replayed events. No user is opted in automatically.

### REQ-SLACK-DIGEST-002: Useful private digest

- **AC-SLACK-DIGEST-002.1:** One digest combines accessible workspaces, listing current assigned tasks needing input, human review, or recovery from failure. Each item begins with its verified issue number/title when metadata provides it, otherwise its card title, and links to Workfloor.
- **AC-SLACK-DIGEST-002.2:** The host verifies active mapped identity, workspace access, current assignment, primary session and nonarchived task before disclosure, including failed sessions. Prompt execution and approvals are never authorized by this read.
- **AC-SLACK-DIGEST-002.3:** Inaccessible tasks are omitted, incomplete scans are never claimed complete, and empty digests remain quiet. Bounded output states when additional items are omitted.

### REQ-SLACK-DIGEST-003: Reliable delivery

- **AC-SLACK-DIGEST-003.1:** Per-user local-time schedules handle DST and deduplicate by local date. Catch-up is limited to two hours; restarts do not repeat a sent or ambiguous delivery.
- **AC-SLACK-DIGEST-003.2:** The exact outbound digest is durable before delivery; mapping, opt-in and all included target permissions are rechecked immediately before sending. Uncertain posts require reconciliation. Existing notifications and bound card replies continue working.
