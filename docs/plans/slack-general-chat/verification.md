# General DM verification

2026-09-19 implementation evidence; general chat is not yet enabled in production.

- Plugin: `go test -race ./server -count=1` and `go vet ./server` passed. Tests cover real inbox/reconcile/Slack HTTP card delivery against fixtures, task creation, typed intent rejection, revoked users, thread isolation and replay after plugin restart. Existing conversation, digest and notification tests pass in the same suite.
- SDK: public extension builds against the pinned upstream SDK used in CI. Host wire tests cover Query/Create/InvokeProfile and old-host refusal.
- Package: `make package verify-package` passed for all five declared platforms. The Workfloor manifest variant also passed using the updated FLXBL packer.
- Disposable host: upgraded the actual Workfloor package from 0.5.0 to 0.6.0; active state, settings and a data sentinel survived. No Slack credentials were used. This verifies installation, not a real-user DM.
- Host: targeted race tests, changed-code lint, profile schema tests, typecheck, docs and spec checks passed. Desktop and mobile profile selection/save/reload preserve secret settings; neighboring mobile plugin settings controls pass.
- Production: the existing 0.5.0 reply bridge remains installed. The user-authorized existing attention automation is enabled on its 15-minute schedule with its existing pilot recipient mapping. General DM deployment and a real Slack pilot remain outstanding.

The new-task path can start the selected workflow or leave it queued. A request that was interrupted before its task identity was recorded is explicitly unconfirmed and is not retried as another creation. Importing existing GitHub issues and starting arbitrary existing tasks are outside this release.
