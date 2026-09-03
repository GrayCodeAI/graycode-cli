package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// DiscordGateway bridges graycode to Discord via the official Gateway (WebSocket),
// using bwmarrin/discordgo. Unlike a REST-poll bridge it receives message events
// in real time — guild @mentions and direct messages — with reconnection,
// heartbeats, and rate limiting handled by the library. Authorized prompts are
// forwarded to the daemon's /v1/chat endpoint and the reply is posted back to the
// originating channel.
type DiscordGateway struct {
	cfg        DiscordConfig
	daemonAddr string
	apiKey     string
	client     *http.Client // for forwardToGraycode
	auth       *authorizer
	dispatch   *asyncDispatcher

	// openSession is overridable in tests; production opens a real Gateway socket.
	openSession func(ctx context.Context) (*discordgo.Session, error)
}

// newDiscordGateway builds a Discord gateway from config.
func newDiscordGateway(cfg DiscordConfig, daemonAddr, apiKey string) *DiscordGateway {
	g := &DiscordGateway{
		cfg:        cfg,
		daemonAddr: daemonAddr,
		apiKey:     apiKey,
		client:     &http.Client{Timeout: 30 * time.Second},
		auth:       newAuthorizer(cfg.PairingCode, cfg.AllowList),
		dispatch:   newAsyncDispatcher(8),
	}
	g.openSession = g.openGatewaySession
	return g
}

// Name implements Gateway.
func (g *DiscordGateway) Name() string { return "discord" }

// setDaemonURL implements daemonURLSetter.
func (g *DiscordGateway) setDaemonURL(url string) { g.daemonAddr = url }

// Stop implements Gateway. The Gateway connection is driven by the Start context.
func (g *DiscordGateway) Stop() error { return nil }

// Start opens the Discord Gateway, registers the message handler, and blocks
// until ctx is cancelled, then drains in-flight handlers and closes the socket.
func (g *DiscordGateway) Start(ctx context.Context) error {
	s, err := g.openSession(ctx)
	if err != nil {
		return err
	}
	s.AddHandler(func(sess *discordgo.Session, m *discordgo.MessageCreate) {
		g.onMessageCreate(ctx, sess, m)
	})
	if err := s.Open(); err != nil {
		return fmt.Errorf("discord gateway open: %w", err)
	}
	defer func() { _ = s.Close() }()

	<-ctx.Done()
	g.dispatch.wait()
	return ctx.Err()
}

// openGatewaySession creates a discordgo session configured with the intents the
// bridge needs: guild messages, message content (privileged — enable it in the
// Discord developer portal), and direct messages.
func (g *DiscordGateway) openGatewaySession(_ context.Context) (*discordgo.Session, error) {
	s, err := discordgo.New("Bot " + g.cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("discord session: %w", err)
	}
	s.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentMessageContent |
		discordgo.IntentsDirectMessages
	return s, nil
}

// onMessageCreate filters inbound events and dispatches authorized messages to
// the shared handler. Replies go back through the live Gateway session.
func (g *DiscordGateway) onMessageCreate(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate) {
	selfID := ""
	if s.State != nil && s.State.User != nil {
		selfID = s.State.User.ID
	}
	if !wantsDiscordMessage(m, selfID) {
		return
	}
	text := stripDiscordMention(m.Content, selfID)
	channelID := m.ChannelID
	send := func(content string) error {
		_, err := s.ChannelMessageSend(channelID, content)
		return err
	}
	g.dispatch.run(ctx, func() {
		g.handleMessage(ctx, m.Author.ID, channelID, text, send)
	})
}

// wantsDiscordMessage reports whether a message should be processed: skip our own
// messages and other bots; accept direct messages and any message that @mentions
// the bot. selfID is the bot's own user ID (empty until the READY event lands).
func wantsDiscordMessage(m *discordgo.MessageCreate, selfID string) bool {
	if m == nil || m.Author == nil || m.Author.Bot {
		return false
	}
	if selfID != "" && m.Author.ID == selfID {
		return false
	}
	if m.GuildID == "" {
		return true // direct message
	}
	for _, u := range m.Mentions {
		if u != nil && u.ID == selfID {
			return true
		}
	}
	return false
}

// handleMessage applies the pairing/allowlist policy and forwards authorized
// prompts to the daemon. It is transport-agnostic: send delivers the reply, so
// the policy/forwarding logic is unit-testable without a live Gateway session.
func (g *DiscordGateway) handleMessage(ctx context.Context, senderID, channelID, text string, send func(string) error) {
	reply := func(content string) {
		if err := send(content); err != nil {
			slog.Error("discord reply failed", "channel", channelID, "error", err)
		}
	}

	if isPair, ok := g.auth.tryPair(senderID, text); isPair {
		if ok {
			reply("Paired. You can now chat with graycode.")
		} else {
			reply("Pairing failed: invalid code.")
		}
		return
	}
	if !g.auth.allowed(senderID) {
		reply("Unauthorized. Send /pair <code> to authorize.")
		return
	}

	resp, err := forwardToGraycode(ctx, g.client, g.daemonAddr, g.apiKey, text)
	if err != nil {
		resp = fmt.Sprintf("Error: %v", err)
	}
	if len([]rune(resp)) > 1900 { // Discord 2000-char limit, leave headroom
		resp = string([]rune(resp)[:1900]) + "\n\n... (truncated)"
	}
	reply(resp)
}

// stripDiscordMention removes a leading <@id> / <@!id> mention token from content.
func stripDiscordMention(content, selfID string) string {
	content = strings.TrimSpace(content)
	if selfID == "" {
		return content
	}
	for _, tok := range []string{"<@" + selfID + ">", "<@!" + selfID + ">"} {
		if strings.HasPrefix(content, tok) {
			return strings.TrimSpace(content[len(tok):])
		}
	}
	return content
}
