# FLXBL customization register

## Notification card boundaries

Version 0.6.2 moves the native Block Kit divider from inside the card to its
start, before the optional app identity. This separates grouped messages while
keeping each title, context, summary and actions together. Preserve delivery
fingerprints, text fallbacks and reply bindings. No host changes are required.

## Notification card identity

Version 0.6.1 adds optional operator-owned app name/logo settings to the standalone
Slack plugin and displays them inside structured notification cards. Both notify
tools share the renderer. Branding stays outside the delivery fingerprint so
configuration edits preserve old receipts and reply bindings. Existing
notifications are not rewritten or replayed. No host change or Slack scope is
required. See the [design](docs/specs/slack/system-design/notification-cards.md)
and [delivery plan](docs/plans/slack-card-branding/plan.md).

Preserve existing FLXBL notification delivery, conversations, personal digests
and general chat customizations during upstream integration.
