# kandev-plugin-slack

Turn Slack conversations into Kandev tasks.

Mention the bot or run the slash command, and the plugin reads the surrounding
thread, asks your triage agent which workspace, workflow and column the work
belongs in, creates the task, and replies in-thread.

```
you (in #eng):  @Kandev the safari login redirect loops for SSO users
                :eyes:
Kandev:         Filed this in Platform › Engineering › Backlog as
                "Fix SSO login redirect loop on Safari". PLAT-482
```

> The exchange above is what the plugin is built to do, written out rather
> than screenshotted: no live Slack workspace has exercised it yet, and a
> mocked-up screenshot would claim more than has been verified. The Kandev
> side below is real.

Two ways to trigger it:

- **`@Kandev <what needs doing>`** in any channel the bot is in. Mention it
  inside a thread and the agent reads that thread; mention it in the channel
  and it reads the ~20 messages leading up to you, so "@Kandev file what Bob
  just said" works.
- **`/kandev <what needs doing>`** anywhere, including channels the bot is not
  a member of. The answer comes back privately to you, since that is how you
  asked. It reads the recent channel conversation for context.

## Setup

Two tokens, about two minutes.

1. **Create the app.** Go to [api.slack.com/apps](https://api.slack.com/apps) →
   **Create New App** → **From a manifest**, pick your workspace, and paste
   [`slack-app-manifest.yaml`](slack-app-manifest.yaml) from this repo. It
   declares the scopes, the `/kandev` command, the `app_mention` subscription,
   and Socket Mode.
2. **App-level token.** Basic Information → App-Level Tokens → **Generate**,
   with the `connections:write` scope. That is the `xapp-…` token.
3. **Install and copy the bot token.** **Install to Workspace**, then OAuth &
   Permissions → **Bot User OAuth Token**. That is the `xoxb-…` token.
4. **Paste both** into Settings → Plugins → Slack, pick a triage agent, save.
5. **Invite the bot** to the channels you want it to read: `/invite @Kandev`.

![Settings → Plugins → Slack](https://raw.githubusercontent.com/kdlbs/kandev-plugin-slack/5599242e8ce50aa68fd7dc0f10025c33193148b0/docs/settings.png)

The badge reads **Listening** once the WebSocket is open. Credentials are
stored in Kandev's encrypted vault and masked on read; only the plugin
subprocess ever sees them in cleartext.

Both tokens come from the same Slack app but different pages, so swapping them
is the easy mistake. The plugin says which one it got:

![Token validation](https://raw.githubusercontent.com/kdlbs/kandev-plugin-slack/5599242e8ce50aa68fd7dc0f10025c33193148b0/docs/validation.png)

### Why Socket Mode

Kandev usually runs on localhost, and the ordinary Events API needs a public
HTTPS request URL that Slack can POST to. [Socket
Mode](https://docs.slack.dev/apis/events-api/using-socket-mode/) exists for
exactly this: the app opens a WebSocket to Slack and events are pushed down it,
so nothing has to be publicly reachable and nothing has to be polled.

The trade-off is that Socket Mode apps cannot be listed in the public Slack
Marketplace — irrelevant here, since each install is your own app in your own
workspace.

There is no "Add to Slack" button because [OAuth
v2](https://docs.slack.dev/authentication/installing-with-oauth/) requires a
public HTTPS redirect URL to receive the authorization code, which a
self-hosted install does not have. Creating your own app from the manifest is
the standard alternative, and it keeps the tokens in your workspace rather than
routing an install through someone else's server.

## Fallback: browser session

Some workspaces forbid app installs outright. For those, leave the app fields
empty and fill in the two **Fallback** fields instead: the `xoxc-…` token and
the `d` cookie from a logged-in Slack tab (the token is in
`localStorage.localConfig_v2` under `teams.<id>.token`; the cookie is in
Application → Cookies). Both must come from the same session.

This mode is unofficial and unsupported by Slack. It cannot receive events, so
it polls `search.messages` for your own messages starting with `!kandev`, and
it breaks when you sign out or when the `d` cookie rotates. Prefer the app.

If both paths are filled in, the app wins — silently dropping to a polling
fallback because a stale cookie was still saved would be a confusing way to
lose real-time events.

## Settings

| Field | Notes |
| --- | --- |
| App-level token | Secret. `xapp-`, scope `connections:write`. |
| Bot token | Secret. `xoxb-`, from OAuth & Permissions after install. |
| Triage agent | Which utility agent makes the decision. |
| Start agent on the new task | Off by default; the task lands on the board. |
| Fallback: session token / `d` cookie | Secret. Only for workspaces that forbid apps. |
| Fallback: command prefix / channels / poll interval | Fallback only; ignored by the app path. |

## How triage works

Kandev's `InvokeUtilityAgent` is a one-shot completion with no tool loop, so
the plugin does the tool work itself:

1. Read the workspaces, workflows, columns and repositories you have.
2. Send the agent the Slack thread plus that topology, and ask for one JSON
   decision.
3. Validate the answer against the real topology — a hallucinated id falls back
   to the first workspace rather than dropping the request — and create the task
   through the Host API.
4. Reply with the task identifier appended — in-thread for a mention, and
   privately through the command's `response_url` for `/kandev`, which also
   works in channels the bot was never invited to.

Slack redelivers any Socket Mode envelope it does not see acknowledged within
three seconds, so envelopes are acknowledged before triage starts and requests
are deduplicated by channel, timestamp and instruction. Slack also cycles
connections deliberately; a `disconnect` frame is treated as routine and
redialled without backoff.

**Known gap:** the Host task-creation API has no repository field, so triaged
tasks are created without a repository attached even though the agent is shown
which repositories each workspace has.

## Agent notifications

Kandev 0.94.0 or newer can expose the plugin's `notify_user` agent tool as
`kandev_kandev_plugin_slack_notify_user` in Kanban and Office task sessions.
This includes task-backed scheduled automations and SSH executors: delivery
runs in the host plugin process, never on the runner. Configuration/external
MCP clients do not receive this tool. No send webhook is added.

Example arguments:

```json
{
  "user_id": "U12345678",
  "text": "Your review is needed on Example card: https://workfloor.example/task/123",
  "idempotency_key": "task-123:waiting-episode-456"
}
```

Use a verified Slack user ID (`U…` or `W…`), never a channel, email or display
name. The automation owns assignee mapping and deciding whether a card really
needs human attention; skip unassigned/unmapped people. Include a plain card URL
and concise reason. Text is limited to 4,000 Unicode characters. Slack markup,
angle brackets, NUL, and mass mentions are rejected; markdown parsing, mention
expansion, link previews and media previews are disabled. Bare URLs remain
clickable, including card links with query parameters. The tool does not itself
read cards or resolve email addresses.

The existing configured `xoxb-` bot token and `chat:write` scope are reused.
[Slack documents direct user-ID posting](https://docs.slack.dev/reference/methods/chat.postMessage/#post-to-a-direct-message-channel)
through `chat.postMessage`; the tool does not call `conversations.open` and
therefore does not request `im:write`, `users:read` or `users:read.email`.
A bot may still be forbidden from entering a particular DM; `channel_not_found`
reports that failure. Browser fallback credentials are never used to send agent
notifications. Existing incoming triage and its fallback are unchanged.

### Delivery and duplicate suppression

The host supplies the caller's verified task, session and workspace. Duplicate
keys are scoped to that workspace and Slack recipient, so the same automation
can run in new task sessions without re-sending an unchanged notification. Use a
stable key for each waiting episode, and change it when the card leaves and
later returns to waiting or acquires a new actionable question. Reusing a key
with different text returns `idempotency_conflict`; do not include changing
clock times in otherwise identical messages.

The result contains `status`, `recipient`, and `duplicate`; confirmed `sent`
results also contain `channel` and `timestamp`. The same safe JSON is present
in fallback text for hosts that omit structured content on MCP errors. Only
`sent` is success. A replay
returns the original result with `duplicate: true` and makes no Slack request.
Permission and credential failures return fixed safe codes such as
`missing_scope`, `invalid_auth`, `token_revoked`, `account_inactive`,
`user_not_found` or `channel_not_found`. `rate_limited` includes
`retry_after_seconds` when Slack provides a positive Retry-After header.
`not_configured` means a bot token or Host configuration was unavailable.
Raw Slack bodies, transport errors and credentials are never returned.

Claims and results are stored under `KANDEV_PLUGIN_DATA_DIR/notifications-v1`
in private, append-only journal files. Exclusive file creation arbitrates
concurrent processes sharing that directory; a result is flushed before return.
The directory survives restart/upgrade and disable. Uninstall removes it.
Include it in backups; do not delete it as routine cache cleanup. Records contain
content hashes and delivery metadata, not message text or tokens. Records have
no automatic expiry; an operator may retire old records only when their keys
can no longer be issued. Use a local filesystem with reliable exclusive-create
and flush semantics; a shared network filesystem is not supported.

A timeout, cancellation during sending, malformed response, Slack internal
error, incomplete journal or failed result persistence returns `unknown`.
That may mean Slack accepted the message. The plugin never retries, follows a
redirect, or expires an uncertain claim. **Do not replace an unknown key to
force a retry:** reconcile with the recipient/operator first. Concurrent calls
can observe `unknown` while the first call is in progress; repeat the same key
later to read its committed result. This favors avoiding duplicate pings over
guaranteed delivery; it is not an exactly-once promise across network/power
failures. A crash before sending can also leave a blocked claim.

Confirmed failures are retained too. After fixing configuration/permissions or
waiting out a reported rate limit, an operator may authorize a new attempt with
a new key only for an explicitly rejected (`rate_limited`, permission or auth)
result, never `unknown`. `storage_unavailable` means no send was attempted by
that invocation; preserve any existing journal until its state is understood.

## Developing against the SDK

`pkg/pluginsdk` is not published as a standalone module yet, so `go.mod`
resolves it from a sibling checkout of the Kandev monorepo. CI pins SDK
revision `f92877b4be2724c0bfa1f1cdbdd35edd68a24fa6` (v0.95.0); use the same
revision locally. The agent-tool wire contract is already present in v0.94.0:


```
~/kandev-plugins/
├── kandev/                    # kdlbs/kandev checkout
└── kandev-plugin-slack/       # this repo
```

```bash
make test          # go test ./server
make vet
make package-host verify-package-host  # local platform only — the fast loop
make package verify-package            # all five manifest platforms
```

CI mirrors these on every pull request (`ci.yml` also enforces `go mod tidy`
and `gofmt`; `build.yml` packages and verifies all five platforms). Releases
are cut by running the **release** workflow from Actions on `master`: it calculates the
next version, rewrites `manifest.yaml`/`Makefile`/`README.md`, updates the
changelog, tags, and publishes `kandev-plugin-slack-<version>.tar.gz` with its
`checksums.txt` to a GitHub Release.

Kandev refuses to reinstall the same id and version. Uninstall first while
iterating — the version is a release number, not an iteration counter, and
nothing here has been released yet:

```bash
curl -X DELETE localhost:<port>/api/plugins/kandev-plugin-slack
curl -F package=@kandev-plugin-slack-0.2.1.tar.gz localhost:<port>/api/plugins/install
```


### Hidden automation runs

The notification handler also accepts host-verified `automation` context.
The default package continues to declare Kanban and Office surfaces for
compatibility with released upstream hosts. Enabling hidden automation use
requires a host and agentctl that support automation plugin tools; a plugin
restart alone does not add that host capability.

For such a deployment, copy `manifest.yaml` to a deployment-owned manifest and
add `automation` to `notify_user.surfaces`, preserving the other fields. Build
with `make package verify-package MANIFEST=/absolute/path/to/manifest.yaml
KANDEV_BACKEND=/absolute/path/to/supporting-host/apps/backend` (one command).
Install only after upgrading the host and agentctl. Record the source revision,
manifest overlay and package checksum; the resulting package differs from the
default release. Do not expose the tool by pretending the run is a Kanban task.

Automation runs do not need task-plan APIs for a notification ledger. Use an
atomically replaced file in the reusable run's scratch directory. Key each
notification by workspace, card, current session, assigned person and stable
pending-request ID (never poll time). Persist the exact recipient, text and
idempotency key before sending; on interruption reuse those exact arguments.
The plugin's durable journal prevents repeat delivery even after an observer
replacement loses its local ledger. An uncertain/failed result remains blocked
for human reconciliation; never invent another key to retry it. Validate a
controlled send and identical repeated call before enabling a schedule.


## Reply to an existing card from a Slack DM

Version 0.3.0 adds opt-in card conversations. A reply in a **new bound card
notification thread** continues that card's existing agent session. The agent's
completed answer returns to the same thread. Existing `notify_user` notifications
remain compatible, but do not acquire a target automatically.

1. Deploy a Workfloor host implementing `ConversationHost` (ResolveConversation,
   SubmitExternalMessage, GetExternalMessage). The minimum-version field alone
   is not sufficient; old hosts fail closed when these RPCs are unavailable.
2. Build using the [public SDK extension](sdk/README.md), then package and install
   the plugin. No private host source is included in the plugin repository.
3. Update the Slack app from `slack-app-manifest.yaml`: enable the Messages tab,
   subscribe to `message.im`, and add `im:history`. Reinstall the app so the new
   scope takes effect. Socket Mode remains enabled; no public webhook is needed.
4. Set `conversation_team_id`, `conversation_app_id`, and the HTTPS
   `workfloor_url` origin. Set `conversation_users` to a JSON object mapping
   `TEAM_ID:SLACK_USER_ID` to a Workfloor user ID. Only administrators should
   maintain this mapping. Enable `conversations_enabled` for a controlled pilot.
5. Have the observer call `notify_task_user` with `user_id`, explicit `task_id`
   and `session_id`, `text`, and a stable `idempotency_key`. Its trusted workspace
   context must match the card. A normal task agent can notify only its own
   task/session; an automation can notify eligible cards in its workspace.
6. Reply in that notification's thread. The mapped human must still be the
   assignee, retain workspace access, and target the current session. Formal
   questions and permission requests are answered in Workfloor initially.

For Workfloor automation tools, render the explicit surface-enabled manifest:

```sh
python3 scripts/workfloor-manifest.py > /tmp/slack-workfloor-manifest.yaml
make package MANIFEST=/tmp/slack-workfloor-manifest.yaml KANDEV_BACKEND=/path/to/workfloor/apps/backend
make verify-package
```

Keep the notification automation paused during the first pilot. Verify one
assigned disposable card, its answer, replay deduplication, and a plugin restart
before expanding. General top-level DMs receive guidance; they do not create new
cards or choose the last card implicitly. Channel mentions and slash-command
triage keep their existing behavior.

### Recovery and privacy

The plugin saves inbound events before Socket Mode acknowledgement. It polls
outstanding host receipts every five seconds, so event delivery is not required
for recovery. Replies within one binding are serialized. Human mapping and host
access are rechecked before output; only the accepted turn's completed visible
answer is delivered, with long responses shortened and linked to the card.

The `conversations-v1` directory under `KANDEV_PLUGIN_DATA_DIR` contains private
inbound text, thread bindings, and delivery journals. Back it up with the plugin
state. It is not a credentials store. Completed-event and delivery tombstones
are retained to prevent replay; automatic retention pruning is not implemented.
The pending inbox is limited to 128 records and 32 KB per input message.

A Slack 429 waits for Retry-After. A lost or ambiguous post is never blindly
resent, even after restart. Logs identify the affected event digest; inspect its
outbox record and the actual Slack thread before any manual recovery. Likewise,
an uncertain host receipt requires inspecting the card before issuing a new
instruction. Never bypass either journal by inventing a fresh idempotency key.

## Personal daily attention digests (0.4.0)

Each mapped person can configure a combined digest across their accessible
workspaces by sending the Workfloor bot a **new DM**, outside a card thread:

```text
digest at 09:00 Australia/Melbourne weekdays
digest at 17:30 Europe/London daily
digest status
digest off
digest help
```

Times use the specified IANA timezone, including daylight saving. Delivery is
once per local date, with at most two hours of catch-up; no historical backlog
is sent after a long outage. No message is sent when nothing needs attention.
Settings and delivery records survive plugin restart/upgrade. Each Slack sender
changes only their own settings; no one is opted in automatically. Workspace
subsets and arbitrary weekday combinations are not part of this initial version.

An administrator enables `digests_enabled` and configures the existing
`conversation_team_id`, `conversation_app_id`, `conversation_users` verified
human mapping, bot token and `workfloor_url`. Digests can run while card
conversations are disabled. No additional Slack scopes beyond `message.im` and
`im:history` used by conversations are needed. Use the Messages tab on desktop
or mobile; no separate native Workfloor settings page is introduced.

The host must implement the optional `AttentionHost` / `ResolveAttentionTarget`
RPC, requiring `api_read:attention`. This extension only authorizes disclosure
of current assigned task/session data, including failed sessions. Preferences,
scheduling and aggregation remain in this plugin. There is no host scheduler
change. The plugin additionally uses public task/session/interaction readers.
An older host fails closed and does not deliver digests.

The digest covers current primary sessions awaiting input, pending formal
interactions, idle human-review steps and failed/blocked work. Historical
sessions, ordinary running work, unassigned/inaccessible/completed/archived cards
are excluded. Tasks without a primary session cannot currently be authorized
for this digest. Items lead with the linked issue number/title when verified
metadata is available; otherwise the card title is used. The digest links to
Workfloor; it never answers permissions, resumes agents or selects a card from
a multi-card reply. An incomplete scan is withheld, never reported as complete.

Do not delete digest-day or outbox records to retry an uncertain Slack post.
They prevent duplicates. Inspect the bot DM and reconcile manually. A stale
prepared digest is held if any included task, assignment, mapping or attention
state changes before delivery; a later date can produce a fresh digest.
