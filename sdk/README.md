# Conversation SDK extension

The host implementation lives in flxbl-io/kandev. This directory contains only
its additive public Go SDK and protobuf messages; no private host implementation,
credentials, REST access, or database code is distributed here.

Until this extension is available in a published SDK, build against the pinned
public Kandev v0.95.0 checkout used by CI, then run:

```sh
python3 sdk/apply.py ../kandev/apps/backend
make -C ../kandev/apps/backend proto
make test
```

The script is repeatable and checks the existing SendMessage contract before
inserting the three optional conversation RPCs. Deploying this SDK overlay does
not implement them in an old host: the matching Workfloor host change is required.
Unimplemented RPCs fail closed. Existing notification and triage APIs remain usable.

The overlay also includes the additive optional `AttentionHost` read-only
`ResolveAttentionTarget(ConversationTarget)` RPC. The host verifies the current
assigned human and primary session before digest disclosure, including failed
sessions. It requires `api_read:attention`; old hosts return Unimplemented.

The overlay includes the optional AssistantHost API for human-scoped task
status, durable creation and operator-selected profile invocation. Its public
Go types define the bounded JSON RPC payload. It requires the corresponding
FLXBL host implementation and assistant read/write capabilities.
