// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter // import "github.com/ucpr/opentelemetry-collector-components/exporters/notificationexporter"

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/ucpr/opentelemetry-collector-components/exporters/notificationexporter/internal/metadata"
)

const defaultTimeout = 10 * time.Second

// NewFactory creates a factory for the notification exporter.
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		metadata.Type,
		createDefaultConfig,
		exporter.WithLogs(createLogsExporter, metadata.LogsStability),
	)
}

func createDefaultConfig() component.Config {
	clientConfig := confighttp.NewDefaultClientConfig()
	clientConfig.Timeout = defaultTimeout

	return &Config{
		ClientConfig:  clientConfig,
		BackOffConfig: configretry.NewDefaultBackOffConfig(),
		QueueSettings: configoptional.Some(exporterhelper.NewDefaultQueueConfig()),
		Method:        http.MethodPost,
	}
}

func createLogsExporter(ctx context.Context, set exporter.Settings, cfg component.Config) (exporter.Logs, error) {
	c := cfg.(*Config)

	e, err := newNotificationExporter(set, c)
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewLogs(
		ctx,
		set,
		cfg,
		e.pushLogs,
		// Disabled: request timeout is governed by ClientConfig.Timeout on
		// the underlying http.Client instead, same rationale as
		// splunkhecexporter/otlphttpexporter.
		exporterhelper.WithTimeout(exporterhelper.TimeoutConfig{Timeout: 0}),
		exporterhelper.WithRetry(c.BackOffConfig),
		exporterhelper.WithQueue(c.QueueSettings),
		exporterhelper.WithStart(e.start),
		exporterhelper.WithShutdown(e.shutdown),
	)
}
