# Personal attention digests

Implement the approved personal digest proposal. Deliver plugin-owned preferences, timer, authorized aggregation and Slack output; add only the missing read authorization to the host. Validate identities, primary sessions, DST, replay, access revocation, package upgrade and real Slack pilot. No automatic opt-in or host deployment approval.

1. Add and test optional read-only AttentionHost authorization.
2. Implement Slack preference commands, scheduler, bounded formatting and durable delivery with TDD.
3. Verify source and packages, open the FLXBL PRs, install the updated plugin after checks, and report any host rollout dependency separately.

Local implementation checks: full plugin race tests and vet passed against the
prepared public SDK overlay. Targeted host authorization and SDK gRPC checks
passed, including failed-session read vs prompt denial and old-host refusal.
Package/installation and production digest proof are tracked separately; the
production host currently predates both conversation and attention support.
