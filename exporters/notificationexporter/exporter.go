// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter // import "github.com/ucpr/opentelemetry-collector-components/exporters/notificationexporter"

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"text/template"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/multierr"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"
)

// notificationExporter renders matched log records through a body template
// and delivers the result to an HTTP endpoint.
type notificationExporter struct {
	config    *Config
	settings  component.TelemetrySettings
	tmpl      *template.Template
	condition *ottl.ConditionSequence[*ottllog.TransformContext]

	client *http.Client
}

// templateData is the value passed to the body template for each matched
// log record.
type templateData struct {
	Body               string
	SeverityText       string
	SeverityNumber     int32
	Timestamp          time.Time
	Attributes         map[string]any
	ResourceAttributes map[string]any
	ScopeAttributes    map[string]any
}

func newNotificationExporter(set exporter.Settings, cfg *Config) (*notificationExporter, error) {
	tmpl, err := parseBodyTemplate(cfg)
	if err != nil {
		return nil, err
	}

	cond, err := parseCondition(cfg.Condition, set.TelemetrySettings)
	if err != nil {
		return nil, err
	}

	return &notificationExporter{
		config:    cfg,
		settings:  set.TelemetrySettings,
		tmpl:      tmpl,
		condition: cond,
	}, nil
}

func (e *notificationExporter) start(ctx context.Context, host component.Host) error {
	client, err := e.config.ClientConfig.ToClient(ctx, host.GetExtensions(), e.settings)
	if err != nil {
		return fmt.Errorf("build HTTP client: %w", err)
	}
	e.client = client
	return nil
}

func (*notificationExporter) shutdown(context.Context) error {
	return nil
}

// pushLogs evaluates the exporter's condition (if any) against every log
// record in ld and delivers the matched ones. Records that fail delivery are
// collected into a partial-failure error so the queue/retry layer only
// reprocesses the records that actually failed.
func (e *notificationExporter) pushLogs(ctx context.Context, ld plog.Logs) error {
	var sendErrs error
	failed := plog.NewLogs()

	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			sl := sls.At(j)
			lrs := sl.LogRecords()
			for k := 0; k < lrs.Len(); k++ {
				lr := lrs.At(k)

				matched, err := e.matches(ctx, rl, sl, lr)
				if err != nil {
					sendErrs = multierr.Append(sendErrs, fmt.Errorf("evaluate condition: %w", err))
					continue
				}
				if !matched {
					continue
				}

				if err := e.send(ctx, rl, sl, lr); err != nil {
					sendErrs = multierr.Append(sendErrs, err)
					appendFailedRecord(failed, rl, sl, lr)
				}
			}
		}
	}

	if sendErrs != nil {
		return consumererror.NewLogs(sendErrs, failed)
	}
	return nil
}

func (e *notificationExporter) matches(ctx context.Context, rl plog.ResourceLogs, sl plog.ScopeLogs, lr plog.LogRecord) (bool, error) {
	if e.condition == nil {
		return true, nil
	}
	tCtx := ottllog.NewTransformContextPtr(rl, sl, lr)
	defer tCtx.Close()
	return e.condition.Eval(ctx, tCtx)
}

func (e *notificationExporter) send(ctx context.Context, rl plog.ResourceLogs, sl plog.ScopeLogs, lr plog.LogRecord) error {
	data := templateData{
		Body:               lr.Body().AsString(),
		SeverityText:       lr.SeverityText(),
		SeverityNumber:     int32(lr.SeverityNumber()),
		Timestamp:          lr.Timestamp().AsTime(),
		Attributes:         lr.Attributes().AsRaw(),
		ResourceAttributes: rl.Resource().Attributes().AsRaw(),
		ScopeAttributes:    sl.Scope().Attributes().AsRaw(),
	}

	var buf bytes.Buffer
	if err := e.tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("render body template: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, e.config.Method, e.config.ClientConfig.Endpoint, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	e.setAuthHeaders(req)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("notification endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

// setAuthHeaders adds the Authorization/Content-Type headers implied by the
// exporter's Type (see destination.go), for destinations authenticated via a
// bot token rather than a pre-authenticated webhook URL. confighttp's
// `headers:` config is applied by the HTTP client's transport when the
// request is actually sent (after this runs), so an explicit `headers:`
// entry still wins over these defaults if the user sets one.
func (e *notificationExporter) setAuthHeaders(req *http.Request) {
	if d, ok := destinations[e.config.effectiveType()]; ok {
		d.setAuthHeaders(e.config, req)
	}
}

// appendFailedRecord copies rl/sl/lr into dst, preserving resource and scope
// context, so a partial-failure error carries enough structure for the
// condition/template to be re-evaluated correctly on retry.
func appendFailedRecord(dst plog.Logs, rl plog.ResourceLogs, sl plog.ScopeLogs, lr plog.LogRecord) {
	outRL := dst.ResourceLogs().AppendEmpty()
	rl.Resource().CopyTo(outRL.Resource())
	outRL.SetSchemaUrl(rl.SchemaUrl())

	outSL := outRL.ScopeLogs().AppendEmpty()
	sl.Scope().CopyTo(outSL.Scope())
	outSL.SetSchemaUrl(sl.SchemaUrl())

	lr.CopyTo(outSL.LogRecords().AppendEmpty())
}
