package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// discordAPIBase is the Discord REST API root. It is a package var so tests can
// point it at a mock server.
var discordAPIBase = "https://discord.com/api/v10"

// DiscordGateway bridges hawk to Discord. Because no WebSocket library is present
// in go.mod, the gateway uses a REST long-poll-equivalent strategy: it does not
// open the Discord Gateway socket but instead relies on a poll over the bot's
// accessible channels for new @mentions/DMs. This keeps the bridge dependency-free
// while remaining bidirectional (it posts replies in-thread via the REST API).
//
// The polling transport is intentionally minimal and pluggable: fetchMessages is
// a field so a real Gateway-WebSocket implementation (or a test) can be swapped
// in without changing the message-handling logic.
type DiscordGateway struct {
	cfg        DiscordConfig
	daemonAddr string
	apiKey     string
	client     *http.Client
	auth       *authorizer

	// pollChannels lists channel IDs the bot watches. When empty the gateway
	// discovers DM channels lazily as it sees them via fetchMessages.
	pollChannels []string
	// lastSeen maps channelID -> last processed message ID for poll dedup.
	lastSeen map[string]string

	// fetchMessages retrieves new messages for a channel since the given message
	// ID. Overridable for tests. Default uses the Discord REST API.
	fetchMessages func(ctx context.Context, channelID, afterID string) ([]discordMessage, error)
}

// discordMessage is the subset of a Discord message object we use.
type discordMessage struct {
	ID        string        `json:"id"`
	ChannelID string        `json:"channel_id"`
	Content   string        `json:"content"`
	Author    discordUser   `json:"author"`
	Mentions  []discordUser `json:"mentions"`
	GuildID   string        `json:"guild_id,omitempty"`
}

type discordUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Bot      bool   `json:"bot,omitempty"`
}

// newDiscordGateway builds a Discord gateway from config.
func newDiscordGateway(cfg DiscordConfig, daemonAddr, apiKey string) *DiscordGateway {
	g := &DiscordGateway{
		cfg:        cfg,
		daemonAddr: daemonAddr,
		apiKey:     apiKey,
		client:     &http.Client{Timeout: 30 * time.Second},
		auth:       newAuthorizer(cfg.PairingCode, cfg.AllowList),
		lastSeen:   make(map[string]string),
	}
	g.fetchMessages = g.fetchMessagesREST
	return g
}

// Name implements Gateway.
func (g *DiscordGateway) Name() string { return "discord" }

// Stop implements Gateway. Polling is driven by the Start context.
func (g *DiscordGateway) Stop() error { return nil }

// Start implements Gateway: poll watched channels until ctx is cancelled.
func (g *DiscordGateway) Start(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			g.pollOnce(ctx)
		}
	}
}

func (g *DiscordGateway) pollOnce(ctx context.Context) {
	for _, ch := range g.pollChannels {
		msgs, err := g.fetchMessages(ctx, ch, g.lastSeen[ch])
		if err != nil {
			continue
		}
		for _, m := range msgs {
			g.lastSeen[m.ChannelID] = m.ID
			g.handleMessage(ctx, m)
		}
	}
}

// mentionsBot reports whether the message is a DM or @mentions the bot user.
func (g *DiscordGateway) mentionsBot(m discordMessage) bool {
	// DM: no guild.
	if m.GuildID == "" {
		return true
	}
	for _, u := range m.Mentions {
		if u.ID == g.cfg.AppID {
			return true
		}
	}
	return false
}

// stripMention removes a leading <@id> / <@!id> mention token from content.
func stripDiscordMention(content, appID string) string {
	content = strings.TrimSpace(content)
	for _, tok := range []string{"<@" + appID + ">", "<@!" + appID + ">"} {
		if strings.HasPrefix(content, tok) {
			return strings.TrimSpace(content[len(tok):])
		}
	}
	return content
}

func (g *DiscordGateway) handleMessage(ctx context.Context, m discordMessage) {
	if m.Author.Bot {
		return // ignore other bots / our own echoes
	}
	if !g.mentionsBot(m) {
		return
	}
	text := stripDiscordMention(m.Content, g.cfg.AppID)
	sender := m.Author.ID

	if isPair, ok := g.auth.tryPair(sender, text); isPair {
		if ok {
			_ = g.postMessage(ctx, m.ChannelID, "Paired. You can now chat with hawk.")
		} else {
			_ = g.postMessage(ctx, m.ChannelID, "Pairing failed: invalid code.")
		}
		return
	}
	if !g.auth.allowed(sender) {
		_ = g.postMessage(ctx, m.ChannelID, "Unauthorized. Send /pair <code> to authorize.")
		return
	}

	reply, err := forwardToHawk(ctx, g.client, g.daemonAddr, g.apiKey, text)
	if err != nil {
		reply = fmt.Sprintf("Error: %v", err)
	}
	if len(reply) > 1900 { // Discord 2000-char limit, leave headroom
		reply = reply[:1900] + "\n\n... (truncated)"
	}
	_ = g.postMessage(ctx, m.ChannelID, reply)
}

// postMessage sends a message to a Discord channel (in-thread reply context).
func (g *DiscordGateway) postMessage(ctx context.Context, channelID, content string) error {
	if g.cfg.Token == "" || channelID == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]string{"content": content})
	apiURL := fmt.Sprintf("%s/channels/%s/messages", discordAPIBase, channelID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bot "+g.cfg.Token)
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// fetchMessagesREST retrieves channel messages via the Discord REST API.
func (g *DiscordGateway) fetchMessagesREST(ctx context.Context, channelID, afterID string) ([]discordMessage, error) {
	apiURL := fmt.Sprintf("%s/channels/%s/messages?limit=20", discordAPIBase, channelID)
	if afterID != "" {
		apiURL += "&after=" + afterID
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+g.cfg.Token)
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("discord messages: HTTP %d: %s", resp.StatusCode, string(data))
	}
	var msgs []discordMessage
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}
