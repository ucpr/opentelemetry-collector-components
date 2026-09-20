// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter/exportertest"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory()
	require.NotNil(t, factory)
	assert.Equal(t, "notification", factory.Type().String())
}

func TestCreateDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	require.NotNil(t, cfg)

	assert.Equal(t, "POST", cfg.Method)
	assert.Equal(t, defaultTimeout, cfg.ClientConfig.Timeout)
	assert.True(t, cfg.QueueSettings.HasValue())

	// The default config has neither an endpoint nor a body template, so it
	// is intentionally invalid until the user supplies both.
	assert.Error(t, cfg.Validate())
}

func TestCreateLogsExporter(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.ClientConfig.Endpoint = "https://example.com/webhook"
	cfg.BodyTemplate = `{"text": {{ .Body | toJson }}}`

	set := exportertest.NewNopSettings(exportertest.NopType)
	exp, err := createLogsExporter(context.Background(), set, cfg)
	require.NoError(t, err)
	require.NotNil(t, exp)

	require.NoError(t, exp.Start(context.Background(), componenttest.NewNopHost()))
	require.NoError(t, exp.Shutdown(context.Background()))
}

func TestCreateLogsExporter_InvalidCondition(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.ClientConfig.Endpoint = "https://example.com/webhook"
	cfg.BodyTemplate = `{}`
	cfg.Condition = `this is not valid OTTL(`

	set := exportertest.NewNopSettings(exportertest.NopType)
	_, err := createLogsExporter(context.Background(), set, cfg)
	assert.Error(t, err)
}

func TestCreateLogsExporter_InvalidTemplate(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.ClientConfig.Endpoint = "https://example.com/webhook"
	cfg.BodyTemplate = `{{ .NotClosed`

	set := exportertest.NewNopSettings(exportertest.NopType)
	_, err := createLogsExporter(context.Background(), set, cfg)
	assert.Error(t, err)
}
