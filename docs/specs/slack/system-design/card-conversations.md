---
status: draft
system: slack
requirements:
  - REQ-SLACK-CONVERSATIONS-001
  - REQ-SLACK-CONVERSATIONS-002
  - REQ-SLACK-CONVERSATIONS-003
---
# Card conversation design

## What changes

Add a private thread bridge to the existing standalone plugin. Keep routing deterministic: Slack team, DM channel, and root timestamp identify a durable binding. Message text never chooses the task. This design covers the three requirements in [the requirement document](../requirements/card-conversations.md).

## Confirmed boundaries

The current plugin only decodes app_mention; its notify_user journal stores a recipient and Slack delivery result but no target card. An automation's tool context identifies the observer itself, so it must not become the conversation target. Workfloor's current public Messages().Send takes task/session/text, returns session/status, and stamps plugin provenance. It lacks a verified human argument, idempotency receipt, and turn correlation. Messages().List, Sessions().List, and Interactions().ListPending already exist. Generic plugin reads are not per-human authorization.

Use public Host gRPC extensions for missing capabilities, implemented and reviewed in flxbl-io/kandev. Do not use the plugin's environment, an administrator API token, internal REST endpoints, or direct database access as a shortcut. Keep private core source out of this public plugin repository.

## Routing and identity

Configure an operator-managed mapping from (Slack team ID, Slack user ID) to Workfloor user ID; never infer identity from names or email in message content. An additive, explicitly capability-gated Host conversation API resolves an authorized target from that verified actor, and checks active user, workspace access, current human assignee, task/session relationship, current session, and task lifecycle. The same checks apply immediately before dispatch and before returning any transcript to Slack. Check queued work again at consumption. Removed access invalidates the binding.

Add a separate notify_task_user tool for bound notifications; preserve notify_user's exact existing schema/journal. Its structured target fields are validated against the trusted calling workspace and the target's current assignment; automation callers may target eligible cards in that workspace, while a normal task caller is restricted to its own task. Resolve the target before posting. Persist the intended binding before Slack delivery and commit its root timestamp after delivery. An ambiguous post cannot create a usable binding until reconciled. No parsing target URLs from notification text, no backfilling old notifications by guessing.

Binding fields: schema version, Slack team/channel/root timestamp/recipient, Workfloor workspace/task/session/actor, source request ID, and notification key. Use the existing plugin data directory with durable atomic writes and exclusive claims; maintain separate inbound/outbound records. No credentials in records.

## Inbound and host dispatch

Subscribe to message.im; decode only type=message, channel_type=im, supported team/app, human text without edit/delete/bot subtypes. Persist a bounded inbox record before acknowledging the Socket Mode envelope; deduplicate on (team_id,event_id), not envelope ID. Process off the socket loop. Ignore self/bot events and reject unknown threads with one deduplicated guidance response.

Proposed additive SDK methods: resolve authorized conversation, submit external message, and read delivery receipt/output. Final proto names belong to the host work order. Submit carries mapped actor, task/session, Slack provenance, text, and stable external event key; the host stamps plugin identity. It durably accepts one command per plugin/event key and content fingerprint, returns a receipt, and preserves the current prompt/queue/resume path. Receipt state links the accepted message/queue entry to actual turn IDs. Crashes between acceptance and execution reconcile against durable message/queue IDs; uncertain dispatch is blocked, never automatically relaunched. Do not promise exactly-once execution from a plugin-side file alone.

Pending structured clarifications/permissions return a typed requires_interaction result plus an authorized Workfloor link. Do not feed 'yes' to an approval resolver or let an LLM choose an approval outcome. Failed/cancelled or superseded sessions return an actionable refusal.

## Output and recovery

Subscribe to turn.completed and relevant message/session events as wakeups, not as the source of truth. A periodic bounded reconciliation loop reads outstanding authorized receipts, traverses their linked turn/message IDs, and posts only completed user-visible assistant text to the original thread. Capture fast completions even if the event arrives before the submit response. Queue receipts remain pending until consumed. Persist dedup per binding/message ID and outbound chunk; handle rate limits with Retry-After. On ambiguous Slack delivery, keep an uncertain record and surface reconciliation required instead of claiming success or resending.

Serialize requests per binding. Multiple Slack threads for the same session must follow receipt/turn correlation rather than broadcasting a reply to all threads. Status acknowledgements never claim an agent has completed. Convert/split safe Slack text with a fixed tested limit, suppress mass mentions/unfurls, and link to Workfloor for long content. Renewed assignee/access checks prevent delayed output reaching a former assignee.

## Slack and deployment

Keep Socket Mode. The app needs im:history plus message.im subscription; the Messages tab has already been enabled by the operator. Updated scopes require Slack app reinstallation. See [message.im](https://docs.slack.dev/reference/events/message.im/) and [Socket Mode](https://docs.slack.dev/apis/events-api/using-socket-mode/). Record tested host minimum version; fail closed on older hosts. Start opt-in with one mapped pilot user, one disposable task, and the notification schedule paused. Expand only after real bidirectional delivery and replay/restart tests pass.
