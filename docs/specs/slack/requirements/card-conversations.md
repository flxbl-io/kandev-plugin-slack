---
status: draft
system: slack
created: 2026-09-19
owners:
  - flxbl-io
---
# Conversations on existing cards

An assigned human can reply in a Workfloor notification thread, continue the existing card's agent conversation, and read its response in Slack.

### REQ-SLACK-CONVERSATIONS-001: Correct conversation and human

- **AC-SLACK-CONVERSATIONS-001.1:** A reply from the verified notification recipient reaches exactly the notified card and session; no replacement card or session is created.
- **AC-SLACK-CONVERSATIONS-001.2:** Unknown users, another recipient, removed workspace access, changed assignee, archived cards, and superseded sessions are rejected before execution or conversation disclosure. The reply explains how to continue through Workfloor without exposing another user's task details.
- **AC-SLACK-CONVERSATIONS-001.3:** When general chat is disabled, unbound top-level DMs and old notifications without a binding receive usage guidance. The opt-in [general-chat contract](general-chat.md) owns unbound assistant threads when enabled. They never select the last active task implicitly.
- **AC-SLACK-CONVERSATIONS-001.4:** Workfloor records the verified human and Slack provenance. A text mention, pasted URL, or requested user ID cannot change the sender or target.

### REQ-SLACK-CONVERSATIONS-002: Useful two-way conversation

- **AC-SLACK-CONVERSATIONS-002.1:** A valid free-text reply is acknowledged as accepted or queued; busy agents finish their current turn before consuming queued input. Normal idle-session resume uses the existing agent/executor.
- **AC-SLACK-CONVERSATIONS-002.2:** The corresponding completed assistant response returns to the same private Slack thread with a Workfloor link. Tool output, hidden reasoning, system messages, and unrelated session turns are not mirrored.
- **AC-SLACK-CONVERSATIONS-002.3:** A pending structured clarification or permission request is identified and linked to Workfloor; ordinary chat text never masquerades as a formal answer or approval. The user can continue free-text discussion when no such interaction blocks it.
- **AC-SLACK-CONVERSATIONS-002.4:** Desktop and mobile Slack users can reply in-thread, see queued/sent/failed states, and open the same card in Workfloor. Long responses have an explicit truncation notice and link.

### REQ-SLACK-CONVERSATIONS-003: Recovery and compatibility

- **AC-SLACK-CONVERSATIONS-003.1:** Slack event replay and concurrent duplicate delivery cannot launch duplicate prompts. Reusing an identity with different content is rejected.
- **AC-SLACK-CONVERSATIONS-003.2:** Restart/upgrade preserves thread bindings, inbound dispatch receipts, and outbound progress. An uncertain outbound send is shown as uncertain and is never blindly resent.
- **AC-SLACK-CONVERSATIONS-003.3:** Lost Workfloor events are reconciled from durable dispatch/message records. Revoked access is rechecked before delayed output is sent.
- **AC-SLACK-CONVERSATIONS-003.4:** Existing three-argument notify_user calls, their duplicate suppression, channel mentions, and slash commands retain their behavior. Disabling conversation support preserves notification delivery.

## Exclusions

General assistant DMs and new-card creation from DMs are owned by the separately gated [general-chat extension](general-chat.md). Attachments, group DMs, Slack Connect, agent interruption, model switching, merging/deployment, and granting approvals from Slack remain excluded. Structured questions/buttons can be a later increment; their current Workfloor controls remain authoritative.
