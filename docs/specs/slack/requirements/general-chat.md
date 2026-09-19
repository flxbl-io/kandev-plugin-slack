---
status: draft
system: slack
created: 2026-09-19
owners: [flxbl-io]
---
# General Workfloor chat in Slack

A mapped person can DM Workfloor to find, understand and start work without first receiving a card notification. The Slack integration owns this conversational experience; Workfloor owns permissions, task state and execution. This is an opt-in extension of [card conversations](card-conversations.md), not a change to their default routing.

### REQ-SLACK-ASSISTANT-001: Personal discovery and follow-ups

- **AC-SLACK-ASSISTANT-001.1:** “Show my tasks” and “What needs my attention?” shall return current assigned work across accessible workspaces, grouped by workspace with issue number/title when available and card links. A partial read shall be identified as incomplete.
- **AC-SLACK-ASSISTANT-001.2:** A person shall be able to ask about an accessible card by link, repository/issue reference or name, including cards without sessions. An inaccessible result shall not disclose its title, existence or content.
- **AC-SLACK-ASSISTANT-001.3:** “The second one” shall resolve only against that sender's current result list in that Slack thread. Missing or ambiguous context shall produce a concise clarification without starting work.

### REQ-SLACK-ASSISTANT-002: Requested work reaches the right card

- **AC-SLACK-ASSISTANT-002.1:** An explicit request to create or start work with a uniquely identified target shall execute without an extra generic confirmation. Missing repository, workspace, workflow or target choices shall be clarified first; the assistant shall never select the first workspace as a fallback.
- **AC-SLACK-ASSISTANT-002.2:** Starting a linked issue shall reuse its existing card when unambiguous. If no card exists, the configured issue integration shall verify the issue and import it through the ordinary creation path, assigning the requesting person when permitted. Unavailable issue access shall cause a useful refusal rather than an invented card.
- **AC-SLACK-ASSISTANT-002.3:** The response shall distinguish created, queued, running and failed-to-start outcomes, show the resolved issue/title and link, and allow follow-up discussion on that card in the same thread. Existing assignment, current-session and structured-interaction rules remain applicable.

### REQ-SLACK-ASSISTANT-003: Private and recoverable conversations

- **AC-SLACK-ASSISTANT-003.1:** Unknown or inactive senders shall receive account-linking guidance without task disclosure. Access shall be rechecked before actions and delayed output; conversation state shall not cross people, teams or threads.
- **AC-SLACK-ASSISTANT-003.2:** Duplicate events and process restarts shall not duplicate cards, issue imports, agent starts or prompts. An uncertain outcome shall be reported as uncertain and reconciled before retrying.
- **AC-SLACK-ASSISTANT-003.3:** Digest commands, existing bound threads, channel mentions and slash commands shall retain their behavior. Disabling general chat shall preserve existing notifications and card conversations.
- **AC-SLACK-ASSISTANT-003.4:** Desktop and mobile Slack users shall receive readable threaded results, explicit truncation/pagination and actionable failures. Formal approvals and structured questions shall link to Workfloor; conversational text shall not grant approval.

### REQ-SLACK-ASSISTANT-004: Plugin-selected agent profile

- **AC-SLACK-ASSISTANT-004.1:** Workfloor plugin settings shall provide an explicit general-chat agent-profile selector. The selected profile governs DM interpretation and replies, independently of the agent chosen for a coding card.
- **AC-SLACK-ASSISTANT-004.2:** The selected profile shall survive restart/upgrade. An unavailable or deleted profile shall show an actionable configuration error rather than silently switching to another agent.

## Exclusions

Merging/deploying, changing permissions or credentials, arbitrary shell execution from the DM router, group DMs, attachments and Slack Connect. The initial assistant manages Workfloor work; it is not a general web-research bot. Actual coding runs in the chosen card's existing agent and executor.
