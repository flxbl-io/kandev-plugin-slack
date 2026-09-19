---
status: draft
system: slack
requirements: [REQ-SLACK-DIGEST-001, REQ-SLACK-DIGEST-002, REQ-SLACK-DIGEST-003]
created: 2026-09-19
owners: [flxbl-io]
---
# Personal digest design

Implements REQ-SLACK-DIGEST-001 through REQ-SLACK-DIGEST-003. Slack top-level DMs use deterministic `digest at HH:MM Area/City daily|weekdays`, `digest status`, `digest off` and `digest help` commands. They are consumed by the existing authenticated, durable inbox before ordinary unbound-message guidance. Settings use the same operator-verified Slack/human mapping, with Slack event timestamps preventing an older replay from replacing newer preferences. Replies inside a card thread remain conversation text.

The plugin's existing lifecycle loop runs a bounded scheduler. It stores preferences and per-local-day delivery records in its private durable directory, with no new host scheduler or general-purpose agent. Read candidate tasks/sessions/workflows using public Host readers, and require the separate optional AttentionHost read authorization for every primary session before classification/disclosure. The small host extension preserves failed-session visibility without relaxing conversation prompt policy. Fail closed on old hosts.

Use the existing Slack bot and durable notification journal. Store exact text/target list before sending, recheck access and mapping before each attempt, and hold rather than regenerate stale or uncertain messages. No credentials in preferences or journal. No automatic historical backfill; two-hour wall-clock catch-up, one claim per local date, skip nonselected days. Users control settings directly in Slack on desktop or mobile; plugin page documentation shows command usage. A native Workfloor settings form and arbitrary weekday/workspace subsets are deferred.
