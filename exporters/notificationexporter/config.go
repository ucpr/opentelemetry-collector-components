// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter // import "github.com/ucpr/opentelemetry-collector-components/exporters/notificationexporter"

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.uber.org/multierr"
)

// Supported values for Config.Type.
const (
	// TypeWebhook is the fully generic mode: deliver the rendered
	// body_template to an arbitrary HTTP endpoint. This is also what powers
	// a Slack Incoming Webhook or a Discord Incoming Webhook, since those
	// are just JSON-over-HTTP endpoints. It is the default when Type is
	// unset, for backward compatibility.
	TypeWebhook = "webhook"

	// TypeSlackApp delivers via the Slack Web API (chat.postMessage),
	// authenticated as a Slack App/bot token, rather than a fixed Incoming
	// Webhook URL. Requires SlackApp.Token and SlackApp.Channel.
	TypeSlackApp = "slack_app"

	// TypeDiscordWebhook is TypeWebhook with Discord-shaped defaults (a
	// default body_template producing {"content": ...}) so the common case
	// of posting to a Discord Incoming Webhook URL doesn't require
	// hand-writing the JSON template.
	TypeDiscordWebhook = "discord_webhook"

	// TypeDiscordBot delivers via the Discord Bot API, authenticated as a
	// bot token, rather than a Discord Incoming Webhook URL. Requires
	// DiscordBot.Token and DiscordBot.ChannelID.
	TypeDiscordBot = "discord_bot"
)

const (
	slackPostMessageEndpoint             = "https://slack.com/api/chat.postMessage"
	discordChannelMessagesEndpointFormat = "https://discord.com/api/v10/channels/%s/messages"

	defaultDiscordBodyTemplate = `{"content": {{ printf "[%s] %s" .SeverityText .Body | toJson }}}`
)

// SlackAppConfig holds settings for Config.Type == TypeSlackApp.
type SlackAppConfig struct {
	// Token is the Slack Bot User OAuth Token (xoxb-...) used to
	// authenticate against the Slack Web API. Sent as an `Authorization:
	// Bearer <token>` header.
	Token configopaque.String `mapstructure:"token"`

	// Channel is the Slack channel ID or name to post to (e.g. "C0123456"
	// or "#alerts").
	Channel string `mapstructure:"channel"`
}

// DiscordBotConfig holds settings for Config.Type == TypeDiscordBot.
type DiscordBotConfig struct {
	// Token is the Discord bot token used to authenticate against the
	// Discord API. Sent as an `Authorization: Bot <token>` header.
	Token configopaque.String `mapstructure:"token"`

	// ChannelID is the Discord channel ID to post to.
	ChannelID string `mapstructure:"channel_id"`
}

// Config defines configuration for the notification exporter.
type Config struct {
	// ClientConfig holds the HTTP client settings (endpoint, headers, TLS,
	// auth, compression, ...) used to deliver the rendered notification.
	ClientConfig confighttp.ClientConfig `mapstructure:",squash"`

	QueueSettings configoptional.Optional[exporterhelper.QueueBatchConfig] `mapstructure:"sending_queue"`
	BackOffConfig configretry.BackOffConfig                                `mapstructure:"retry_on_failure"`

	// Method is the HTTP method used to deliver the rendered notification.
	// Defaults to POST.
	Method string `mapstructure:"method"`

	// Type selects the destination integration this exporter delivers to:
	// one of TypeWebhook (default), TypeSlackApp, TypeDiscordWebhook, or
	// TypeDiscordBot. The non-webhook types pre-fill endpoint/auth/body
	// defaults for that specific destination; body_template /
	// body_template_file can still be set to override the default payload.
	Type string `mapstructure:"type"`

	// SlackApp holds settings for Type == TypeSlackApp.
	SlackApp SlackAppConfig `mapstructure:"slack_app"`

	// DiscordBot holds settings for Type == TypeDiscordBot.
	DiscordBot DiscordBotConfig `mapstructure:"discord_bot"`

	// BodyTemplate is the inline Go text/template source used to render the
	// HTTP request body for each matched log record. Exactly one of
	// BodyTemplate or BodyTemplateFile must be set, except for the
	// slack_app/discord_webhook/discord_bot types, which fall back to a
	// built-in default template when neither is set.
	BodyTemplate string `mapstructure:"body_template"`

	// BodyTemplateFile is a path to a file containing the Go text/template
	// source, as an alternative to inlining it via BodyTemplate.
	BodyTemplateFile string `mapstructure:"body_template_file"`

	// Condition is an optional OTTL boolean condition, evaluated against the
	// log record context (same semantics as filterprocessor's log_record
	// conditions). Log records that don't match are skipped by this
	// exporter and are not delivered. When unset, every log record matches.
	//
	// This is intended as defense-in-depth for standalone use; the primary
	// filtering/routing is normally expected to happen upstream in the
	// pipeline (filter processor, routing connector).
	Condition string `mapstructure:"condition"`
}

// effectiveType returns cfg.Type, defaulting to TypeWebhook when unset. It
// does not mutate cfg.Type, so a Config built without going through
// Validate() (e.g. in tests) still resolves consistently.
func (cfg *Config) effectiveType() string {
	if cfg.Type == "" {
		return TypeWebhook
	}
	return cfg.Type
}

// Validate checks the notification exporter configuration for structural
// correctness. It does not validate the syntax of BodyTemplate or Condition:
// those require a template.FuncMap / OTTL function set and telemetry
// settings that aren't available at config-validation time, so they are
// parsed when the exporter is created instead.
func (cfg *Config) Validate() error {
	var errs error

	switch cfg.effectiveType() {
	case TypeWebhook:
		if cfg.ClientConfig.Endpoint == "" {
			errs = multierr.Append(errs, errors.New("endpoint must be specified"))
		}
	case TypeSlackApp:
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
			// channel is config-supplied (not per-record log data), so a
			// single json.Marshal here is enough to embed it safely as a
			// JSON string literal in the generated template source.
			channel, _ := json.Marshal(cfg.SlackApp.Channel)
			cfg.BodyTemplate = fmt.Sprintf(`{"channel": %s, "text": {{ printf "[%%s] %%s" .SeverityText .Body | toJson }}}`, channel)
		}
	case TypeDiscordWebhook:
		if cfg.ClientConfig.Endpoint == "" {
			errs = multierr.Append(errs, errors.New("endpoint must be specified"))
		}
		if cfg.BodyTemplate == "" && cfg.BodyTemplateFile == "" {
			cfg.BodyTemplate = defaultDiscordBodyTemplate
		}
	case TypeDiscordBot:
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
	default:
		errs = multierr.Append(errs, fmt.Errorf("type must be one of %q, %q, %q, %q, got %q", TypeWebhook, TypeSlackApp, TypeDiscordWebhook, TypeDiscordBot, cfg.Type))
	}

	switch strings.ToUpper(cfg.Method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		// Normalize so the exact configured casing (e.g. "post") never reaches
		// the wire: HTTP methods are case-sensitive, and sending a
		// non-canonical method breaks against strict servers.
		cfg.Method = strings.ToUpper(cfg.Method)
	default:
		errs = multierr.Append(errs, errors.New(`method must be one of "POST", "PUT", "PATCH"`))
	}

	hasInline := cfg.BodyTemplate != ""
	hasFile := cfg.BodyTemplateFile != ""
	if hasInline == hasFile {
		errs = multierr.Append(errs, errors.New("exactly one of body_template or body_template_file must be set"))
	}

	return errs
}
