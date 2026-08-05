# kandev-plugin-slack

Turn Slack messages into Kandev tasks.

Post `!kandev <instruction>` in Slack and the plugin picks it up, reads the
surrounding thread, asks your triage agent which workspace, workflow and
column the work belongs in, creates the task, and replies in-thread with what
it did.

```
you (in #eng):  !kandev the safari login redirect loops for SSO users
                :eyes:
kandev:         Filed this in Platform › Engineering › Backlog as
                "Fix SSO login redirect loop on Safari". PLAT-482
```

This plugin replaces the Slack integration that used to ship inside Kandev
itself, so it can move at its own pace.

## Install

Settings → Plugins → install `kandev-plugin-slack-<version>.tar.gz`, then fill
in Settings → Plugins → Slack.

## Choosing a token

The plugin reads the token's prefix to decide how to talk to Slack. There is
no separate mode picker — paste the credential you have.

| Token | Mode | How requests are found | Notes |
| --- | --- | --- | --- |
| `xoxp-…` | User token | `search.messages` for **your own** `!kandev` messages | The supported way to get parity with the old built-in integration. Scopes: `search:read`, `chat:write`, `reactions:write`. |
| `xoxb-…` | Bot token | `conversations.history` on the channels you list | Bot tokens **cannot** search — Slack only exposes `search.messages` to user tokens. Picks up `!kandev` from **anyone** in those channels. Scopes: `channels:history` (and/or `groups:history`), `chat:write`, `reactions:write`. Invite the bot to each channel. |
| `xoxc-…` + `d` cookie | Browser session | `search.messages`, same as a user token | Unofficial and unsupported by Slack: it is the credential pair your browser uses. No app install needed, but it breaks when you sign out and the `d` cookie rotates regularly. |

`xoxe-` (refresh) and `xapp-` (Socket Mode) tokens are rejected with a message
pointing at the right one — both are easy to copy by mistake from the same
Slack app page.

### Getting an `xoxp-` user token

Create a Slack app, add `search:read`, `chat:write` and `reactions:write`
under **User Token Scopes**, install it to your workspace, and copy the *User*
OAuth token.

### Getting the browser session pair

In a logged-in Slack tab: the token is the `xoxc-…` value in
`localStorage.localConfig_v2` (under `teams.<id>.token`), and the cookie is
the `d` cookie's value in Application → Cookies. Both must come from the same
session.

## Settings

| Field | Notes |
| --- | --- |
| Slack token | Secret. Selects the mode, see above. |
| Browser `d` cookie | Secret. Required for `xoxc-` only; rejected for the others so a stale paste cannot go unnoticed. |
| Reply token | Secret, optional `xoxb-`. Posts the reply and the :eyes: acknowledgement as your app instead of as you. |
| Command prefix | Default `!kandev`. Cannot start with `/` — Slack intercepts slash commands before they become messages. |
| Channels | Comma-separated channel **IDs**. Required for `xoxb-`; for `xoxp-`/`xoxc-` it narrows the search. |
| Poll interval | 5–600s, default 30s. |
| Start agent on the new task | Off by default; the task lands on the board instead. |
| Triage agent | Which utility agent makes the decision. |

Credentials are stored in Kandev's encrypted vault and are masked on read;
only the plugin subprocess ever sees them in cleartext.

## How triage works

Kandev's `InvokeUtilityAgent` is a one-shot completion with no tool loop, so
the plugin does the tool work itself:

1. Read the workspaces, workflows, columns and repositories you have.
2. Send the agent the Slack thread plus that topology, and ask for one JSON
   decision.
3. Validate the answer against the real topology — a hallucinated id falls
   back to the first workspace rather than dropping the request — and create
   the task through the Host API.
4. Post the agent's `reply` in-thread, with the task identifier appended.

A message is only marked processed once its task exists, so a Slack outage or
a failed completion retries on the next poll instead of silently swallowing
the request. The watermark lives in Host state, so restarts resume rather than
re-triaging every open request.

**Known gap:** the Host task-creation API has no repository field yet, so
triaged tasks are created without a repository attached even though the agent
is shown which repositories each workspace has.

## Developing against the SDK

`pkg/pluginsdk` is not published as a standalone module yet, so `go.mod`
resolves it from a sibling checkout of the Kandev monorepo:

```
~/kandev-plugins/
├── kandev/                    # kdlbs/kandev checkout
└── kandev-plugin-slack/       # this repo
```

```bash
make test          # go test ./server
make vet
make package-host  # local platform only — the fast loop
make package       # all five platforms declared in manifest.yaml
```

Kandev refuses to reinstall the same id and version, so bump `version` in
`manifest.yaml` (and `VERSION` in the Makefile) between iterations, or
uninstall first.
