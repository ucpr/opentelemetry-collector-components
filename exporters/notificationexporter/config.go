// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter // import "github.com/ucpr/opentelemetry-collector-components/exporters/notificationexporter"

import (
	"errors"
	"net/http"
	"strings"

	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.uber.org/multierr"
)

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

	// BodyTemplate is the inline Go text/template source used to render the
	// HTTP request body for each matched log record. Exactly one of
	// BodyTemplate or BodyTemplateFile must be set.
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

// Validate checks the notification exporter configuration for structural
// correctness. It does not validate the syntax of BodyTemplate or Condition:
// those require a template.FuncMap / OTTL function set and telemetry
// settings that aren't available at config-validation time, so they are
// parsed when the exporter is created instead.
func (cfg *Config) Validate() error {
	var errs error

	if cfg.ClientConfig.Endpoint == "" {
		errs = multierr.Append(errs, errors.New("endpoint must be specified"))
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
