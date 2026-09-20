// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/plog"
)

// newTestExporter builds a started notificationExporter pointed at server,
// using cfg as a template for everything except the endpoint.
func newTestExporter(t *testing.T, server *httptest.Server, cfg *Config) *notificationExporter {
	t.Helper()

	cfg.ClientConfig.Endpoint = server.URL

	e, err := newNotificationExporter(exportertest.NewNopSettings(exportertest.NopType), cfg)
	require.NoError(t, err)
	require.NoError(t, e.start(context.Background(), componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, e.shutdown(context.Background())) })

	return e
}

func newLogs(severity string, body string) plog.Logs {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("k8s.namespace.name", "default")
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	lr.SetSeverityText(severity)
	lr.Body().SetStr(body)
	return ld
}

type capturedRequest struct {
	body    string
	headers http.Header
}

func newCapturingServer(t *testing.T, status int) (*httptest.Server, *[]capturedRequest) {
	t.Helper()
	var mu sync.Mutex
	var got []capturedRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		mu.Lock()
		got = append(got, capturedRequest{body: string(b), headers: r.Header.Clone()})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestPushLogs_DeliversMatchingRecord(t *testing.T) {
	srv, got := newCapturingServer(t, http.StatusOK)
	e := newTestExporter(t, srv, &Config{
		Method:       http.MethodPost,
		BodyTemplate: `{"text": {{ .Body | toJson }}}`,
	})

	err := e.pushLogs(context.Background(), newLogs("Warning", "pod crashed"))
	require.NoError(t, err)

	require.Len(t, *got, 1)
	assert.JSONEq(t, `{"text": "pod crashed"}`, (*got)[0].body)
}

func TestPushLogs_SkipsRecordsNotMatchingCondition(t *testing.T) {
	srv, got := newCapturingServer(t, http.StatusOK)
	e := newTestExporter(t, srv, &Config{
		Method:       http.MethodPost,
		BodyTemplate: `{"text": {{ .Body | toJson }}}`,
		Condition:    `severity_text == "Warning"`,
	})

	err := e.pushLogs(context.Background(), newLogs("Normal", "everything is fine"))
	require.NoError(t, err)
	assert.Empty(t, *got, "Normal severity record must not be delivered when condition requires Warning")

	err = e.pushLogs(context.Background(), newLogs("Warning", "pod crashed"))
	require.NoError(t, err)
	require.Len(t, *got, 1)
}

func TestPushLogs_ReturnsPartialFailureForFailedDelivery(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusInternalServerError)
	e := newTestExporter(t, srv, &Config{
		Method:       http.MethodPost,
		BodyTemplate: `{"text": {{ .Body | toJson }}}`,
	})

	err := e.pushLogs(context.Background(), newLogs("Warning", "pod crashed"))
	require.Error(t, err)

	var logsErr consumererror.Logs
	require.ErrorAs(t, err, &logsErr)
	assert.Equal(t, 1, logsErr.Data().LogRecordCount())
}

func TestPushLogs_SlackApp_SetsBearerAuthHeader(t *testing.T) {
	srv, got := newCapturingServer(t, http.StatusOK)
	e := newTestExporter(t, srv, &Config{
		Type:         TypeSlackApp,
		Method:       http.MethodPost,
		SlackApp:     SlackAppConfig{Token: "xoxb-000", Channel: "#alerts"},
		BodyTemplate: `{"channel": "#alerts", "text": {{ .Body | toJson }}}`,
	})

	err := e.pushLogs(context.Background(), newLogs("Warning", "pod crashed"))
	require.NoError(t, err)

	require.Len(t, *got, 1)
	assert.Equal(t, "Bearer xoxb-000", (*got)[0].headers.Get("Authorization"))
	assert.JSONEq(t, `{"channel": "#alerts", "text": "pod crashed"}`, (*got)[0].body)
}

func TestPushLogs_DiscordBot_SetsBotAuthHeader(t *testing.T) {
	srv, got := newCapturingServer(t, http.StatusOK)
	e := newTestExporter(t, srv, &Config{
		Type:         TypeDiscordBot,
		Method:       http.MethodPost,
		DiscordBot:   DiscordBotConfig{Token: "Bot000", ChannelID: "123"},
		BodyTemplate: defaultDiscordBodyTemplate,
	})

	err := e.pushLogs(context.Background(), newLogs("Warning", "pod crashed"))
	require.NoError(t, err)

	require.Len(t, *got, 1)
	assert.Equal(t, "Bot Bot000", (*got)[0].headers.Get("Authorization"))
	assert.JSONEq(t, `{"content": "[Warning] pod crashed"}`, (*got)[0].body)
}

func TestPushLogs_Webhook_DoesNotSetAuthHeader(t *testing.T) {
	srv, got := newCapturingServer(t, http.StatusOK)
	e := newTestExporter(t, srv, &Config{
		Method:       http.MethodPost,
		BodyTemplate: `{"text": {{ .Body | toJson }}}`,
	})

	err := e.pushLogs(context.Background(), newLogs("Warning", "pod crashed"))
	require.NoError(t, err)

	require.Len(t, *got, 1)
	assert.Empty(t, (*got)[0].headers.Get("Authorization"))
}

func TestPushLogs_InvalidConditionSyntaxFailsAtConstruction(t *testing.T) {
	cfg := &Config{
		Method:       http.MethodPost,
		BodyTemplate: `{}`,
		Condition:    `this is not valid OTTL(`,
	}
	_, err := newNotificationExporter(exportertest.NewNopSettings(exportertest.NopType), cfg)
	assert.Error(t, err)
}
