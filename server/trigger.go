package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// acknowledgeReaction is added to a matched message before the agent runs, so
// the requester sees their message was picked up during the seconds the
// triage completion takes.
const acknowledgeReaction = "eyes"

// probeInterval is how often the stored credentials are re-validated,
// matching the 90s cadence Kandev's built-in integrations use for their
// auth-health polling.
const probeInterval = 90 * time.Second

// baseTick is how often the loop wakes to check whether a scan is due. The
// configured poll interval is the real cadence; this only bounds how quickly
// a configuration change or a manual scan is noticed.
const baseTick = 5 * time.Second

// trigger owns the polling loop that turns matched Slack messages into
// Kandev tasks.
type trigger struct {
	// host is resolved lazily: Serve injects the Host from a background
	// goroutine after the broker connection is up, so it can still be nil
	// when the loop starts.
	host func() pluginsdk.Host

	// scanMu serializes scans so a manual "Scan now" cannot interleave with
	// the timer-driven pass and double-triage the same message.
	scanMu sync.Mutex

	scanNow chan struct{}

	mu        sync.Mutex
	lastScan  time.Time
	lastProbe time.Time
	// probedFor fingerprints the credentials the last probe validated, so
	// pasting a new token re-probes immediately instead of waiting out
	// probeInterval with a stale "connected" banner.
	probedFor string
}

func newTrigger(host func() pluginsdk.Host) *trigger {
	return &trigger{host: host, scanNow: make(chan struct{}, 1)}
}

// Run drives the loop until ctx is cancelled.
func (t *trigger) Run(ctx context.Context) {
	ticker := time.NewTicker(baseTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.scanNow:
			t.runScan(ctx, true)
		case <-ticker.C:
			t.runScan(ctx, false)
		}
	}
}

// ScanNow requests an immediate pass. Non-blocking: a pending request already
// covers the caller's intent.
func (t *trigger) ScanNow() {
	select {
	case t.scanNow <- struct{}{}:
	default:
	}
}

// runScan performs one pass when it is due, logging rather than propagating
// failures — the loop must survive a Slack outage or a half-finished config.
func (t *trigger) runScan(ctx context.Context, force bool) {
	host := t.host()
	if host == nil {
		return
	}
	raw, err := host.GetConfig(ctx)
	if err != nil {
		log.Printf("slack: read config: %v", err)
		return
	}
	cfg, err := loadConfig(raw)
	if err != nil {
		t.recordUnconfigured(ctx, host, err)
		return
	}
	if !force && !t.due(cfg.PollInterval) {
		return
	}
	t.markScanned()
	t.scanMu.Lock()
	defer t.scanMu.Unlock()
	if err := t.scan(ctx, host, cfg); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("slack: scan failed: %v", err)
	}
}

// recordUnconfigured persists the reason the plugin is idle so the operator
// sees it on the plugin page rather than only in the backend log.
func (t *trigger) recordUnconfigured(ctx context.Context, host pluginsdk.Host, cause error) {
	st := readStatus(ctx, host)
	message := ""
	if !errors.Is(cause, errNotConfigured) {
		message = cause.Error()
	}
	if !st.Configured && st.Error == message {
		return
	}
	st.Configured = false
	st.OK = false
	st.Error = message
	st.CheckedAt = nowRFC3339()
	if err := writeStatus(ctx, host, st); err != nil {
		log.Printf("slack: persist status: %v", err)
	}
}

func (t *trigger) due(interval time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastScan.IsZero() || time.Since(t.lastScan) >= interval
}

func (t *trigger) markScanned() {
	t.mu.Lock()
	t.lastScan = time.Now()
	t.mu.Unlock()
}

// scan runs one full pass: validate credentials if due, collect fresh
// matches, triage each, then advance the watermarks.
func (t *trigger) scan(ctx context.Context, host pluginsdk.Host, cfg *config) error {
	cl := newClient(cfg.Token, cfg.Cookie)
	st := readStatus(ctx, host)
	st.Configured = true
	st.Mode = cfg.Mode.String()
	st.ModeLabel = cfg.Mode.Label()

	if err := t.ensureProbed(ctx, cl, cfg, &st); err != nil {
		return err
	}
	if !st.OK {
		st.ScannedAt = nowRFC3339()
		return writeStatus(ctx, host, st)
	}

	marks := readWatermarks(ctx, host)
	matches, err := collect(ctx, cl, cfg, st.UserID, marks)
	if err != nil {
		st.Error = err.Error()
		st.ScannedAt = nowRFC3339()
		_ = writeStatus(ctx, host, st)
		return err
	}
	st.ScannedAt = nowRFC3339()
	if len(matches) == 0 {
		return writeStatus(ctx, host, st)
	}
	t.process(ctx, host, cl, cfg, matches, marks, &st)
	if err := writeWatermarks(ctx, host, marks); err != nil {
		log.Printf("slack: persist watermarks: %v", err)
	}
	return writeStatus(ctx, host, st)
}

// ensureProbed refreshes the auth-health fields when the probe is stale or
// the credentials changed.
func (t *trigger) ensureProbed(ctx context.Context, cl *client, cfg *config, st *status) error {
	fingerprint := credentialFingerprint(cfg)
	t.mu.Lock()
	fresh := t.probedFor == fingerprint && time.Since(t.lastProbe) < probeInterval
	t.mu.Unlock()
	if fresh && st.CheckedAt != "" {
		return nil
	}
	res, err := cl.AuthTest(ctx)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.lastProbe = time.Now()
	t.probedFor = fingerprint
	t.mu.Unlock()

	st.OK = res.OK
	st.Error = res.Error
	st.CheckedAt = nowRFC3339()
	if res.OK {
		st.TeamName = res.TeamName
		st.TeamID = res.TeamID
		st.UserID = res.UserID
		st.UserName = res.DisplayName
	}
	return nil
}

// credentialFingerprint identifies a credential set without retaining it.
// Only lengths and the last few characters are used: enough to notice a
// rotation, never enough to reconstruct a secret if it reaches a log.
func credentialFingerprint(cfg *config) string {
	return fmt.Sprintf("%d:%s|%d|%s", len(cfg.Token), tail(cfg.Token), len(cfg.Cookie), tail(cfg.ReplyToken))
}

func tail(s string) string {
	const n = 4
	if len(s) <= n {
		return ""
	}
	return s[len(s)-n:]
}

// collect gathers messages newer than the watermark, using whichever trigger
// strategy the auth mode supports.
func collect(ctx context.Context, cl *client, cfg *config, userID string, marks map[string]string) ([]message, error) {
	var found []message
	var err error
	if cfg.Mode.searchCapable() {
		found, err = collectBySearch(ctx, cl, cfg, userID, marks[searchWatermarkKey])
	} else {
		found, err = collectByHistory(ctx, cl, cfg, marks)
	}
	if err != nil {
		return nil, err
	}
	fresh := make([]message, 0, len(found))
	for _, m := range found {
		if !hasCommandPrefix(m.Text, cfg.CommandPrefix) {
			continue
		}
		fresh = append(fresh, m)
	}
	sort.SliceStable(fresh, func(i, j int) bool {
		return compareTS(fresh[i].TS, fresh[j].TS) < 0
	})
	return fresh, nil
}

// collectBySearch is the user-token / cookie path. It scopes the search to
// the authenticated user's own messages, matching the semantics of the
// integration this plugin replaces: you triage your own requests, not
// everyone else's.
func collectBySearch(ctx context.Context, cl *client, cfg *config, userID, watermark string) ([]message, error) {
	if userID == "" {
		return nil, errors.New("no authenticated Slack user id yet — the credential probe has not succeeded")
	}
	queries := searchQueries(cfg, userID)
	var out []message
	for _, q := range queries {
		matches, err := cl.SearchMessages(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("search Slack: %w", err)
		}
		for _, m := range matches {
			if compareTS(m.TS, watermark) <= 0 {
				continue
			}
			out = append(out, m)
		}
	}
	return out, nil
}

// searchQueries builds one query per configured channel, or a single
// workspace-wide query when no channels are set. Slack's `in:` modifiers
// combine with AND, so two channels in one query match nothing.
func searchQueries(cfg *config, userID string) []string {
	base := fmt.Sprintf("from:<@%s> %q", userID, cfg.CommandPrefix)
	if len(cfg.Channels) == 0 {
		return []string{base}
	}
	out := make([]string, 0, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		out = append(out, fmt.Sprintf("%s in:<#%s>", base, ch))
	}
	return out
}

// collectByHistory is the bot-token path: read each configured channel's
// history past its own watermark. Unlike search mode this picks up requests
// from anyone in the channel, since a bot has no "own messages" to filter to.
func collectByHistory(ctx context.Context, cl *client, cfg *config, marks map[string]string) ([]message, error) {
	var out []message
	for _, ch := range cfg.Channels {
		msgs, err := cl.ChannelHistory(ctx, ch, marks[ch])
		if err != nil {
			return nil, fmt.Errorf("read history for %s: %w", ch, err)
		}
		out = append(out, msgs...)
	}
	return out, nil
}

// process triages each match in order, advancing the watermark only past
// messages that finished. A recoverable failure stops the batch so the next
// pass retries from the same point rather than skipping the request.
func (t *trigger) process(
	ctx context.Context, host pluginsdk.Host, cl *client, cfg *config,
	matches []message, marks map[string]string, st *status,
) {
	replyClient := cl
	if token, cookie := cfg.ReplyCredentials(); token != cfg.Token || cookie != cfg.Cookie {
		replyClient = newClient(token, cookie)
	}
	topology, err := readTopology(ctx, host)
	if err != nil {
		st.Error = err.Error()
		return
	}
	for _, m := range matches {
		if ctx.Err() != nil {
			return
		}
		entry, err := t.triage(ctx, host, cl, replyClient, cfg, topology, m)
		if err != nil {
			st.note(recentEntry{At: nowRFC3339(), Text: firstLine(m.Text), Error: err.Error()})
			st.Error = err.Error()
			log.Printf("slack: triage %s failed: %v", m.TS, err)
			return
		}
		st.Error = ""
		st.Triaged++
		st.note(entry)
		advance(marks, cfg, m)
	}
}

// advance moves the watermark this message belongs to. Search mode keeps a
// single cross-channel watermark; bot mode keeps one per channel, because
// each channel's history is read independently.
func advance(marks map[string]string, cfg *config, m message) {
	key := searchWatermarkKey
	if !cfg.Mode.searchCapable() {
		key = m.ChannelID
	}
	if compareTS(m.TS, marks[key]) > 0 {
		marks[key] = m.TS
	}
}

// triage runs the full per-message flow: acknowledge, gather context, ask the
// agent, create the task, reply in-thread.
func (t *trigger) triage(
	ctx context.Context, host pluginsdk.Host, read, write *client,
	cfg *config, topology []workspaceTopology, m message,
) (recentEntry, error) {
	if err := write.AddReaction(ctx, m.ChannelID, m.TS, acknowledgeReaction); err != nil {
		// A missing reactions:write scope should not block the actual work.
		log.Printf("slack: acknowledge reaction failed: %v", err)
	}
	thread, err := read.ThreadContext(ctx, m.ChannelID, m.ThreadTS, m.TS)
	if err != nil {
		return recentEntry{}, fmt.Errorf("fetch thread: %w", err)
	}
	permalink := m.Permalink
	if permalink == "" {
		permalink, _ = read.Permalink(ctx, m.ChannelID, m.TS)
	}
	instruction := stripCommandPrefix(m.Text, cfg.CommandPrefix)
	if instruction == "" {
		return recentEntry{}, errors.New("message carried the prefix but no instruction")
	}
	prompt, err := buildTriagePrompt(topology, m, instruction, permalink, thread)
	if err != nil {
		return recentEntry{}, err
	}
	response, err := host.InvokeUtilityAgent(ctx, prompt)
	if err != nil {
		return recentEntry{}, fmt.Errorf("triage agent: %w", err)
	}
	decision, err := parseDecision(response)
	if err != nil {
		return recentEntry{}, err
	}
	task, err := createTask(ctx, host, cfg, decision, topology, m, permalink)
	if err != nil {
		return recentEntry{}, err
	}
	t.reply(ctx, write, m, decision, task)
	return recentEntry{
		At:        nowRFC3339(),
		Text:      firstLine(instruction),
		TaskTitle: task.Title,
		TaskID:    task.ID,
		Permalink: permalink,
	}, nil
}

func createTask(
	ctx context.Context, host pluginsdk.Host, cfg *config,
	decision *triageDecision, topology []workspaceTopology, m message, permalink string,
) (*pluginsdk.Task, error) {
	workspaceID, workflowID, stepID := decision.resolve(topology)
	task, err := host.Tasks().Create(ctx, pluginsdk.CreateTaskInput{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          decision.Title,
		Description:    buildDescription(decision, m, permalink),
		StartAgent:     cfg.StartAgent,
	})
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return task, nil
}

// reply posts the agent's summary back into the thread. A failure here is
// logged but not returned: the task exists, and failing the message would
// re-triage it into a duplicate on the next pass.
func (t *trigger) reply(ctx context.Context, write *client, m message, decision *triageDecision, task *pluginsdk.Task) {
	threadTS := m.ThreadTS
	if threadTS == "" {
		threadTS = m.TS
	}
	body := strings.TrimSpace(decision.Reply)
	if body == "" {
		body = "Created a Kandev task for this."
	}
	if task != nil && task.Identifier != "" {
		body += fmt.Sprintf("\n\n*%s* — %s", task.Identifier, task.Title)
	} else if task != nil {
		body += "\n\n*" + task.Title + "*"
	}
	if err := write.PostMessage(ctx, m.ChannelID, threadTS, body); err != nil {
		log.Printf("slack: reply failed: %v", err)
	}
}

// hasCommandPrefix reports whether text opens with the command marker. The
// leading "> " strip handles Slack rendering a quoted message, and the
// delimiter check stops "!kandevish" from matching "!kandev".
func hasCommandPrefix(text, prefix string) bool {
	t := normalizeLeading(text)
	if !strings.HasPrefix(strings.ToLower(t), strings.ToLower(prefix)) {
		return false
	}
	rest := t[len(prefix):]
	if rest == "" {
		return true
	}
	switch rest[0] {
	case ' ', '\t', '\n', ':', ',':
		return true
	default:
		return false
	}
}

// stripCommandPrefix returns the instruction with the marker and any
// separator punctuation removed.
func stripCommandPrefix(text, prefix string) string {
	t := normalizeLeading(text)
	if len(t) >= len(prefix) && strings.EqualFold(t[:len(prefix)], prefix) {
		t = t[len(prefix):]
	}
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(t), ":, "))
}

func normalizeLeading(text string) string {
	t := strings.TrimSpace(text)
	t = strings.TrimPrefix(t, "> ")
	return strings.TrimSpace(t)
}

// firstLine truncates a message for the activity feed.
func firstLine(text string) string {
	const maxLen = 120
	line := strings.TrimSpace(text)
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	if len(line) > maxLen {
		return line[:maxLen] + "…"
	}
	return line
}
