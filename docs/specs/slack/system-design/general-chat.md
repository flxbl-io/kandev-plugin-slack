---
status: draft
system: slack
requirements:
  - REQ-SLACK-ASSISTANT-001
  - REQ-SLACK-ASSISTANT-002
  - REQ-SLACK-ASSISTANT-003
  - REQ-SLACK-ASSISTANT-004
created: 2026-09-19
owners: [flxbl-io]
---
# General Slack chat design

## Plugin owns the conversation

Extend the standalone plugin with `general_chat_enabled`, independent of card conversations and digests. Reuse Socket Mode, verified team/app/user mapping, durable inbox/outbox and a general-chat agent profile selected directly in the plugin settings. The proposed `general_chat_agent_profile_id` is plugin-scoped; implement its native profile picker and validate that the profile can run the bounded utility flow. Reuse the host utility execution tier, extending its public selection contract if required, rather than relying on a global default or making the person configure a separate utility agent first. `Host.InvokeUtilityAgent` is already a one-shot completion; it can interpret a bounded transcript into a typed intent, not invoke tools itself. No new executor is needed for the router. Coding requests use normal task execution.

Dispatch deterministic digest commands first, existing card bindings second, general-chat threads third, then current guidance. A top-level DM creates an assistant thread rooted at its Slack timestamp. Persist a thread record keyed by team/channel/root/sender with bounded recent turns, authorized result references, pending clarification and operation receipt. Never use a DM-wide “last active card”. A card thread remains bound; a request to switch cards explains how to open a fresh assistant thread.

Supported intents: help, list my tasks, attention, task status, create task, start/import issue and continue card. First classify the request without sharing the entire instance topology. Retrieve authorized candidates, then interpret any necessary follow-up against those candidates. Treat titles, descriptions and quoted Slack content as data. Validate every model-produced action and identifier; render authoritative operation status from receipts. A statement like “what would happen if we started it?” is read-only. An explicit unambiguous start is sufficient authorization; ask only for missing choices.

## Host owns human-scoped operations

Confirmed source gaps: `TaskFilter` has no actor/assignee; `CreateTaskInput` has no verified human or idempotency key and `StartAgent` is best-effort. `AttentionHost.Resolve` requires an assigned card with a primary session. Existing `Conversations.Resolve/Submit/Get` securely handles already-selected sessions. Generic plugin readers or the old triage path must not become a human permission check.

Propose one optional capability-gated Host accessor for external human task operations: discover/read scoped task and workspace/workflow/repository projections; execute an explicit create/import/start operation; look up its durable receipt. Exact SDK/proto names are finalized in the host work order. Read and write capabilities are separate. The host derives the active human from the trusted plugin mapping argument, applies existing workspace/task/GitHub permissions, and returns only permitted fields. Discovery can show accessible read-only cards; continuation still requires the current assignee under the existing conversation contract. No raw transcript is needed for the initial status view.

Use the existing services for issue resolution/import, task creation and launch. Keep human attribution plus plugin/Slack provenance. Do not use admin REST credentials, direct database reads in the plugin, a second GitHub credential store, or unscoped topology in the model prompt. This follows Kandev's existing Host-data and utility-agent boundaries (ADRs 0043 and 0048); it does not introduce a new general agent runtime.

A mutation key includes plugin, verified actor, Slack team/event and operation ordinal, plus an immutable input fingerprint. Host persistence must atomically claim the operation and link created task/session before reconciliation can retry. Reuse normal issue deduplication as well, since issue watchers may race the DM. Creation success and launch success are separate receipt fields. Resume/start ownership and queue behavior remain in the host; uncertain launch is not retried blindly. An older host yields a typed unavailable result, not a fallback to unrestricted task creation.

## Bind and return safely

After successful selection or creation, persist the thread-to-card target and reuse `Conversations.Submit/Get` for task instructions and replies. Do not submit the initial instruction again if the create/start operation already consumed it; record that consumption in the receipt. If the target is not currently continuable, retain the assistant thread and explain the Workfloor recovery/assignment step. Surface formal questions and permissions using existing Workfloor links.

Store operation and output progress before acknowledging completion. Serialize a thread, preserve server-side dedup across different threads, and reauthorize referenced tasks before later model calls or Slack delivery. Cap stored history to the most recent 20 conversational messages and 16 KiB; expired result selections must be looked up again. Output uses the existing bounded plain-text/thread delivery rules. Personal digest preferences remain independent.

REQ-001 maps to scoped discovery and thread context; REQ-002 to typed operations and card handoff; REQ-003 to identity, durable receipts, routing and delivery. A packaged process/host/fake-Slack fixture proves the full path before a disposable live Slack pilot; a build or credential probe alone is insufficient.

## 0.6.0 delivery scope

This delivery implements the requested general DM status and new-task creation
paths plus the plugin-owned profile selector. Verified issue import and starting
arbitrary existing tasks remain a subsequent delivery; those requests direct
the user to the ordinary Workfloor issue-import flow. New task creation can
start its workflow or leave the task queued. General chat remains disabled until
the host/plugin pair is deployed and piloted.
