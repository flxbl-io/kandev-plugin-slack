---
status: current
system: slack
requirements:
  - REQ-SLACK-CARD-001
  - REQ-SLACK-CARD-002
  - REQ-SLACK-CARD-003
---
# Notification card design

Use native Slack Block Kit, owned and rendered by the plugin. The preview in the delivery plan is illustrative; Slack controls typography, spacing and theme. No host change is required.

## Rendering and inputs

For REQ-SLACK-CARD-001, extend notify_user and notify_task_user with optional `card` metadata: required title, workspace_name, attention (`input`, `review`, `error`), summary and workfloor_url; optional repository, issue_number, issue_url, pr_url. Caller reads current card state; renderer does not derive success, CI or stage from prose. Do not accept arbitrary blocks from agents.

Use a bold full-title section, muted context for workspace/repository, divider, a labeled status and summary section, and an actions row. The primary URL button opens Workfloor. Secondary buttons open supplied issue/PR URLs. Use plain_text for summary/context and a rich_text text element with bold style for the title, preserving punctuation literally without parsing mentions or formatting from caller content. No caller-supplied graphics or fake status badges; operator-owned identity branding is described below. Render titles in a section instead of a header to avoid the header's shorter text limit.

Generate the top-level text from the same card data for push notifications and screen readers, including full URLs. Keep mrkdwn=false and previews/mention expansion disabled at message level. Normal Slack link parsing remains enabled. URL-button interaction envelopes are acknowledged by the current Socket Mode default branch; add a regression test confirming no task mutation.

## Compatibility and delivery

For REQ-SLACK-CARD-002, omitted card retains existing text behavior and exact old fingerprint. Card calls use a versioned canonical serialization of text and validated metadata for the durable notification fingerprint; include all displayed fields and destinations. Preserve the existing journal path/key and conservative ambiguous-send behavior. Both notify paths use the same renderer. Do not change conversational outbox/digest formats in this increment.

Validate bounded strings and total Slack block limits before creating a journal claim. URLs require HTTPS, no userinfo/control characters; Workfloor URL origin must match configured workfloor_url, GitHub buttons must target github.com issue/pull paths. If card mode needs an absent configured Workfloor origin, fail clearly rather than infer trust from caller input. Text-only callers remain unaffected.

After testing and installing the new package, update only the existing observer's delivery arguments to supply card metadata; preserve schedule, assignee logic and waiting-episode keys. Pilot one DM to the requesting user and inspect actual desktop/mobile Slack output before wider use.

References: [Slack Block Kit](https://docs.slack.dev/block-kit/), [URL buttons](https://docs.slack.dev/reference/block-kit/block-elements/button-element/).

## Identity strip (REQ-SLACK-CARD-003)

The Slack plugin owns the rendered card, so this change stays in the standalone plugin. Add optional operator settings `notification_brand_name` and `notification_brand_icon_url`, blank by default. For this instance, configure Flux and the existing Flux Slack icon's public HTTPS URL. Agents cannot supply or override branding through card arguments. No new Slack API scope, host API or per-notification profile lookup is needed.

Prepend one native context block with an image element and plain-text name. Use the configured name for image alt text; include the name once at the beginning of the top-level accessible fallback. Keep the existing title and remaining block order. A name without an icon renders text only; an icon without a valid name is ignored. Bound names to 80 characters using the existing plain-text safety checks. Accept only bounded absolute HTTPS image URLs with no credentials, fragment or control characters. An invalid image is omitted; the server never fetches that URL. Slack loads the image. Invalid branding falls back to the current card rather than blocking task notification delivery. Omit the strip if its accessible label would exceed the existing fallback text limit.

Branding is presentation configuration, not caller task content. Preserve the current canonical request fingerprint and journal keys: brand edits must not create idempotency conflicts or authorize resending old records. Resolve brand metadata from the same config snapshot already loaded by `notificationMessage`; do not change tool schemas, journal serialization, bindings or ambiguous-send handling. New messages use current settings; previously sent messages remain untouched.

Validate layout in actual Slack desktop and mobile, especially consecutive messages, long issue titles, dark/light themes, and image-unavailable fallback. The issue title remains the first task-specific content; the identity strip stays visually secondary.

Provider reference: [context blocks support images and text](https://docs.slack.dev/reference/block-kit/blocks/context-block/).
