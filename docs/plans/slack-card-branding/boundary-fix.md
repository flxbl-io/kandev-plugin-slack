# Consecutive card separation — 0.6.2

The live 0.6.1 screenshot showed the problem clearly: the divider separated each
card's title from its summary, but the next card started directly below the
previous action row. Slack's grouped sender header made that boundary ambiguous.

Move the single native divider to the start of every structured notification,
before optional branding. Keep title, context, summary, actions and reply footer
together. Both tools share this renderer; text-only messages, accessible fallback,
canonical payload fingerprints and durable reply bindings stay unchanged.

The local HTML preview also supplied a CSS top border that was not in the Slack
payload. Remove that border; every illustrated separator now comes from the
renderer. The older PNGs in this directory are historical 0.6.1 previews and are
not proof of native Slack layout.

## Verification

- RED: `go test ./server -run 'TestNotificationCardDeliveryAndFallback|TestNotificationCardBranding$' -count=1` failed at the missing leading boundary for unbranded cards and each branded attention state.
- GREEN: `go test -race ./server -run 'Test(Notification|BoundNotification|Notify|SessionFallback|Socket|SlashCommand|Mention)' -count=1` passed.
- `go vet ./server` passed.
- `python3 scripts/workfloor-manifest.py > /tmp/slack-boundaries-manifest.yaml` and `make package-host verify-package-host MANIFEST=/tmp/slack-boundaries-manifest.yaml` passed.
- Disposable host installed 0.6.1 then upgraded to 0.6.2: active, configuration and durable-data sentinel preserved; no live Slack credentials used.
- Desktop dark and mobile-width light illustrative previews: two leading dividers, two identities, no horizontal overflow; visually inspected. This is not native Slack E2E.

Fresh illustrative previews (synthetic task data, placeholder fox):

![Desktop preview](https://raw.githubusercontent.com/flxbl-io/kandev-plugin-slack/6851cff56286253dd9128c772db4ded59c9e4fbe/card-boundaries/desktop-dark.png)

![Mobile-width preview](https://raw.githubusercontent.com/flxbl-io/kandev-plugin-slack/6851cff56286253dd9128c772db4ded59c9e4fbe/card-boundaries/mobile-light.png)

Production installation and native Slack confirmation are separate, pending steps.
