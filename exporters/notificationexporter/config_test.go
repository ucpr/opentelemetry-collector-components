// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/confmaptest"

	"github.com/ucpr/opentelemetry-collector-components/exporters/notificationexporter/internal/metadata"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	cm, err := confmaptest.LoadConf(filepath.Join("testdata", "config.yaml"))
	require.NoError(t, err)

	inlineClientConfig := confighttp.NewDefaultClientConfig()
	inlineClientConfig.Timeout = defaultTimeout
	inlineClientConfig.Endpoint = "https://hooks.slack.com/services/T000/B000/XXXX"
	inlineClientConfig.Headers = configopaque.MapList{
		{Name: "content-type", Value: "application/json"},
	}

	fileClientConfig := confighttp.NewDefaultClientConfig()
	fileClientConfig.Timeout = defaultTimeout
	fileClientConfig.Endpoint = "https://example.com/webhook"

	tests := []struct {
		id       component.ID
		expected component.Config
	}{
		{
			id: component.NewIDWithName(metadata.Type, "inline"),
			expected: func() *Config {
				cfg := createDefaultConfig().(*Config)
				cfg.ClientConfig = inlineClientConfig
				cfg.Condition = `severity_text == "Warning"`
				cfg.BodyTemplate = "{\"text\": {{ .Body | toJson }}}\n"
				return cfg
			}(),
		},
		{
			id: component.NewIDWithName(metadata.Type, "file"),
			expected: func() *Config {
				cfg := createDefaultConfig().(*Config)
				cfg.ClientConfig = fileClientConfig
				cfg.BodyTemplateFile = "testdata/body.tmpl"
				return cfg
			}(),
		},
		{
			id: component.NewIDWithName(metadata.Type, "slack-app"),
			expected: func() *Config {
				cfg := createDefaultConfig().(*Config)
				cfg.Type = TypeSlackApp
				cfg.SlackApp = SlackAppConfig{
					Token:   "xoxb-000-000-abc",
					Channel: "#alerts",
				}
				cfg.ClientConfig.Endpoint = slackPostMessageEndpoint
				cfg.BodyTemplate = `{"channel": "#alerts", "text": {{ printf "[%s] %s" .SeverityText .Body | toJson }}}`
				return cfg
			}(),
		},
		{
			id: component.NewIDWithName(metadata.Type, "discord-bot"),
			expected: func() *Config {
				cfg := createDefaultConfig().(*Config)
				cfg.Type = TypeDiscordBot
				cfg.DiscordBot = DiscordBotConfig{
					Token:     "Bot000.abc",
					ChannelID: "123456789012345678",
				}
				cfg.ClientConfig.Endpoint = "https://discord.com/api/v10/channels/123456789012345678/messages"
				cfg.BodyTemplate = defaultDiscordBodyTemplate
				return cfg
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.id.String(), func(t *testing.T) {
			factory := NewFactory()
			cfg := factory.CreateDefaultConfig()

			sub, err := cm.Sub(tt.id.String())
			require.NoError(t, err)
			require.NoError(t, sub.Unmarshal(cfg))
			require.NoError(t, confmap.Validate(cfg))

			assert.Equal(t, tt.expected, cfg)
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	validClientConfig := func() confighttp.ClientConfig {
		c := confighttp.NewDefaultClientConfig()
		c.Endpoint = "https://example.com/webhook"
		return c
	}

	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid with inline body_template",
			cfg: &Config{
				ClientConfig: validClientConfig(),
				Method:       http.MethodPost,
				BodyTemplate: `{"text": {{ .Body | toJson }}}`,
			},
		},
		{
			name: "valid with body_template_file",
			cfg: &Config{
				ClientConfig:     validClientConfig(),
				Method:           http.MethodPost,
				BodyTemplateFile: "testdata/body.tmpl",
			},
		},
		{
			name: "missing endpoint",
			cfg: &Config{
				Method:       http.MethodPost,
				BodyTemplate: `{}`,
			},
			wantErr: true,
		},
		{
			name: "invalid method",
			cfg: &Config{
				ClientConfig: validClientConfig(),
				Method:       http.MethodGet,
				BodyTemplate: `{}`,
			},
			wantErr: true,
		},
		{
			name: "neither body_template nor body_template_file set",
			cfg: &Config{
				ClientConfig: validClientConfig(),
				Method:       http.MethodPost,
			},
			wantErr: true,
		},
		{
			name: "both body_template and body_template_file set",
			cfg: &Config{
				ClientConfig:     validClientConfig(),
				Method:           http.MethodPost,
				BodyTemplate:     `{}`,
				BodyTemplateFile: "testdata/body.tmpl",
			},
			wantErr: true,
		},
		{
			name: "unknown type",
			cfg: &Config{
				Type:   "carrier-pigeon",
				Method: http.MethodPost,
			},
			wantErr: true,
		},
		{
			name: "slack_app: valid, defaults endpoint and body_template",
			cfg: &Config{
				Type:   TypeSlackApp,
				Method: http.MethodPost,
				SlackApp: SlackAppConfig{
					Token:   "xoxb-000",
					Channel: "#alerts",
				},
			},
		},
		{
			name: "slack_app: missing token",
			cfg: &Config{
				Type:     TypeSlackApp,
				Method:   http.MethodPost,
				SlackApp: SlackAppConfig{Channel: "#alerts"},
			},
			wantErr: true,
		},
		{
			name: "slack_app: missing channel",
			cfg: &Config{
				Type:     TypeSlackApp,
				Method:   http.MethodPost,
				SlackApp: SlackAppConfig{Token: "xoxb-000"},
			},
			wantErr: true,
		},
		{
			name: "discord_webhook: valid, defaults body_template",
			cfg: &Config{
				Type:         TypeDiscordWebhook,
				Method:       http.MethodPost,
				ClientConfig: validClientConfig(),
			},
		},
		{
			name: "discord_webhook: missing endpoint",
			cfg: &Config{
				Type:   TypeDiscordWebhook,
				Method: http.MethodPost,
			},
			wantErr: true,
		},
		{
			name: "discord_bot: valid, defaults endpoint from channel_id and body_template",
			cfg: &Config{
				Type:   TypeDiscordBot,
				Method: http.MethodPost,
				DiscordBot: DiscordBotConfig{
					Token:     "Bot000",
					ChannelID: "123",
				},
			},
		},
		{
			name: "discord_bot: missing token",
			cfg: &Config{
				Type:       TypeDiscordBot,
				Method:     http.MethodPost,
				DiscordBot: DiscordBotConfig{ChannelID: "123"},
			},
			wantErr: true,
		},
		{
			name: "discord_bot: missing channel_id",
			cfg: &Config{
				Type:       TypeDiscordBot,
				Method:     http.MethodPost,
				DiscordBot: DiscordBotConfig{Token: "Bot000"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}

	t.Run("slack_app: fills in endpoint and body_template", func(t *testing.T) {
		cfg := &Config{
			Type:   TypeSlackApp,
			Method: http.MethodPost,
			SlackApp: SlackAppConfig{
				Token:   "xoxb-000",
				Channel: "#alerts",
			},
		}
		require.NoError(t, cfg.Validate())
		assert.Equal(t, slackPostMessageEndpoint, cfg.ClientConfig.Endpoint)
		assert.Equal(t, `{"channel": "#alerts", "text": {{ printf "[%s] %s" .SeverityText .Body | toJson }}}`, cfg.BodyTemplate)
	})

	t.Run("slack_app: explicit body_template is not overwritten", func(t *testing.T) {
		cfg := &Config{
			Type:   TypeSlackApp,
			Method: http.MethodPost,
			SlackApp: SlackAppConfig{
				Token:   "xoxb-000",
				Channel: "#alerts",
			},
			BodyTemplate: `{"channel": "#alerts", "blocks": []}`,
		}
		require.NoError(t, cfg.Validate())
		assert.Equal(t, `{"channel": "#alerts", "blocks": []}`, cfg.BodyTemplate)
	})

	t.Run("discord_bot: fills in endpoint from channel_id and body_template", func(t *testing.T) {
		cfg := &Config{
			Type:   TypeDiscordBot,
			Method: http.MethodPost,
			DiscordBot: DiscordBotConfig{
				Token:     "Bot000",
				ChannelID: "123456789012345678",
			},
		}
		require.NoError(t, cfg.Validate())
		assert.Equal(t, "https://discord.com/api/v10/channels/123456789012345678/messages", cfg.ClientConfig.Endpoint)
		assert.Equal(t, defaultDiscordBodyTemplate, cfg.BodyTemplate)
	})

	t.Run("lower-case method is normalized to canonical upper-case", func(t *testing.T) {
		cfg := &Config{
			ClientConfig: validClientConfig(),
			Method:       "post",
			BodyTemplate: `{}`,
		}
		require.NoError(t, cfg.Validate())
		assert.Equal(t, http.MethodPost, cfg.Method, "Validate must normalize Method so a non-canonical case never reaches the wire")
	})
}
