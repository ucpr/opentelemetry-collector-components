// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBodyTemplate_Inline(t *testing.T) {
	cfg := &Config{BodyTemplate: `{"text": {{ .Body | toJson }}}`}

	tmpl, err := parseBodyTemplate(cfg)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, tmpl.Execute(&buf, templateData{Body: `hello "world"` + "\nline2"}))

	var out map[string]string
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out), "rendered body must be valid JSON: %s", buf.String())
	assert.Equal(t, "hello \"world\"\nline2", out["text"])
}

func TestParseBodyTemplate_File(t *testing.T) {
	cfg := &Config{BodyTemplateFile: "testdata/body.tmpl"}

	tmpl, err := parseBodyTemplate(cfg)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, tmpl.Execute(&buf, templateData{Body: "hi"}))
	assert.JSONEq(t, `{"content": "hi"}`, buf.String())
}

func TestParseBodyTemplate_MissingFile(t *testing.T) {
	cfg := &Config{BodyTemplateFile: "testdata/does-not-exist.tmpl"}

	_, err := parseBodyTemplate(cfg)
	assert.Error(t, err)
}

func TestTemplateFuncs_ToJsonEscapesRawText(t *testing.T) {
	// This is the failure mode the toJson helper exists to prevent: naive
	// interpolation of free-text into a hand-written JSON template breaks as
	// soon as the text contains a quote or newline.
	naive := `{"text": "{{ .Body }}"}`
	tmpl, err := parseBodyTemplate(&Config{BodyTemplate: naive})
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, tmpl.Execute(&buf, templateData{Body: `bad "quote"`}))
	assert.Error(t, json.Unmarshal(buf.Bytes(), &map[string]string{}), "naive interpolation is expected to produce invalid JSON")

	safe := `{"text": {{ .Body | toJson }}}`
	tmpl, err = parseBodyTemplate(&Config{BodyTemplate: safe})
	require.NoError(t, err)

	buf.Reset()
	require.NoError(t, tmpl.Execute(&buf, templateData{Body: `bad "quote"`}))
	assert.NoError(t, json.Unmarshal(buf.Bytes(), &map[string]string{}))
}

func TestTemplateFuncs_JoinLowerUpper(t *testing.T) {
	// join takes its slice from template data rather than a literal, since
	// text/template (unlike sprig) has no built-in `list` constructor.
	tmpl, err := parseBodyTemplate(&Config{BodyTemplate: `{{ join .Tags "," }}|{{ lower "AbC" }}|{{ upper "AbC" }}`})
	require.NoError(t, err)

	var buf bytes.Buffer
	data := struct {
		Tags []string
	}{Tags: []string{"a", "b"}}
	require.NoError(t, tmpl.Execute(&buf, data))
	assert.Equal(t, "a,b|abc|ABC", buf.String())
}
