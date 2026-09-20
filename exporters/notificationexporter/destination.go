// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter // import "github.com/ucpr/opentelemetry-collector-components/exporters/notificationexporter"

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/multierr"
)

const (
	slackPostMessageEndpoint             = "https://slack.com/api/chat.postMessage"
	discordChannelMessagesEndpointFormat = "https://discord.com/api/v10/channels/%s/messages"

	defaultDiscordBodyTemplate = `{"content": {{ printf "[%s] %s" .SeverityText .Body | toJson }}}`
)

// destination encapsulates the behavior that's specific to a Config.Type: how
// to validate/default its config fields, and how to authenticate outgoing
// requests. Supporting a new destination means adding a new implementation
// and registering it in destinations, without touching Config.Validate or
// notificationExporter.send.
type destination interface {
	// configure validates the destination-specific config fields (e.g.
	// required tokens/channels) and fills in defaults - endpoint,
	// body_template - that depend on them. Called once from Config.Validate.
	configure(cfg *Config) error

	// setAuthHeaders sets any Authorization/Content-Type headers this
	// destination needs on an outgoing request. Called once per delivered
	// log record, before the request is sent.
	setAuthHeaders(cfg *Config, req *http.Request)
}

// destinations maps every supported Config.Type to its implementation.
var destinations = map[string]destination{
	TypeWebhook:        webhookDestination{},
	TypeSlackApp:       slackAppDestination{},
	TypeDiscordWebhook: discordWebhookDestination{},
	TypeDiscordBot:     discordBotDestination{},
}

// supportedTypes lists the valid Config.Type values, in the order they
// should be reported in error messages.
var supportedTypes = []string{TypeWebhook, TypeSlackApp, TypeDiscordWebhook, TypeDiscordBot}

func quotedSupportedTypes() string {
	quoted := make([]string, len(supportedTypes))
	for i, t := range supportedTypes {
		quoted[i] = strconv.Quote(t)
	}
	return strings.Join(quoted, ", ")
}

// webhookDestination is the fully generic mode: no destination-specific
// fields, just an explicit, pre-authenticated endpoint.
type webhookDestination struct{}

func (webhookDestination) configure(cfg *Config) error {
	if cfg.ClientConfig.Endpoint == "" {
		return errors.New("endpoint must be specified")
	}
	return nil
}

func (webhookDestination) setAuthHeaders(*Config, *http.Request) {}

// slackAppDestination delivers via the Slack Web API (chat.postMessage),
// authenticated as a Slack App/bot token, rather than a fixed Incoming
// Webhook URL.
type slackAppDestination struct{}

func (slackAppDestination) configure(cfg *Config) error {
	var errs error
	if cfg.SlackApp.Token == "" {
		errs = multierr.Append(errs, errors.New("slack_app.token must be specified"))
	}
	if cfg.SlackApp.Channel == "" {
		errs = multierr.Append(errs, errors.New("slack_app.channel must be specified"))
	}
	if cfg.ClientConfig.Endpoint == "" {
		cfg.ClientConfig.Endpoint = slackPostMessageEndpoint
	}
	if cfg.BodyTemplate == "" && cfg.BodyTemplateFile == "" {
		// channel is config-supplied (not per-record log data), so a single
		// json.Marshal here is enough to embed it safely as a JSON string
		// literal in the generated template source.
		channel, _ := json.Marshal(cfg.SlackApp.Channel)
		cfg.BodyTemplate = fmt.Sprintf(`{"channel": %s, "text": {{ printf "[%%s] %%s" .SeverityText .Body | toJson }}}`, channel)
	}
	return errs
}

func (slackAppDestination) setAuthHeaders(cfg *Config, req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+string(cfg.SlackApp.Token))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
}

// discordWebhookDestination is webhookDestination with Discord-shaped
// defaults (a default body_template producing {"content": ...}) so the
// common case of posting to a Discord Incoming Webhook URL doesn't require
// hand-writing the JSON template.
type discordWebhookDestination struct{}

func (discordWebhookDestination) configure(cfg *Config) error {
	if cfg.ClientConfig.Endpoint == "" {
		return errors.New("endpoint must be specified")
	}
	if cfg.BodyTemplate == "" && cfg.BodyTemplateFile == "" {
		cfg.BodyTemplate = defaultDiscordBodyTemplate
	}
	return nil
}

func (discordWebhookDestination) setAuthHeaders(_ *Config, req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
}

// discordBotDestination delivers via the Discord Bot API, authenticated as a
// bot token, rather than a Discord Incoming Webhook URL.
type discordBotDestination struct{}

func (discordBotDestination) configure(cfg *Config) error {
	var errs error
	if cfg.DiscordBot.Token == "" {
		errs = multierr.Append(errs, errors.New("discord_bot.token must be specified"))
	}
	if cfg.DiscordBot.ChannelID == "" {
		errs = multierr.Append(errs, errors.New("discord_bot.channel_id must be specified"))
	} else if cfg.ClientConfig.Endpoint == "" {
		cfg.ClientConfig.Endpoint = fmt.Sprintf(discordChannelMessagesEndpointFormat, cfg.DiscordBot.ChannelID)
	}
	if cfg.BodyTemplate == "" && cfg.BodyTemplateFile == "" {
		cfg.BodyTemplate = defaultDiscordBodyTemplate
	}
	return errs
}

func (discordBotDestination) setAuthHeaders(cfg *Config, req *http.Request) {
	req.Header.Set("Authorization", "Bot "+string(cfg.DiscordBot.Token))
	req.Header.Set("Content-Type", "application/json")
}
