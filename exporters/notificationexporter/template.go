// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter // import "github.com/ucpr/opentelemetry-collector-components/exporters/notificationexporter"

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"
)

// templateFuncs returns the FuncMap made available to body templates.
//
// toJson is the important one: log record bodies and attributes come from
// arbitrary, free-text Kubernetes event messages (or other log sources) that
// can contain quotes, newlines, and backslashes. Interpolating such a value
// directly into a hand-written JSON template (e.g. `{"text": "{{ .Body }}"}`)
// produces invalid JSON as soon as the text contains a `"` or a newline.
// Piping the value through toJson instead (`{"text": {{ .Body | toJson }}}`)
// marshals it with encoding/json, which escapes it into a valid JSON string
// literal (or, for a map/slice value, a valid JSON literal usable directly
// in the body).
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"toJson": func(v any) (string, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return "", fmt.Errorf("toJson: %w", err)
			}
			return string(b), nil
		},
		"toJsonIndent": func(v any, prefix, indent string) (string, error) {
			b, err := json.MarshalIndent(v, prefix, indent)
			if err != nil {
				return "", fmt.Errorf("toJsonIndent: %w", err)
			}
			return string(b), nil
		},
		"join":  strings.Join,
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
	}
}

// parseBodyTemplate builds the body template from exactly one of
// cfg.BodyTemplate (inline source) or cfg.BodyTemplateFile (path to a file
// containing the source). Config.Validate guarantees exactly one is set.
func parseBodyTemplate(cfg *Config) (*template.Template, error) {
	source := cfg.BodyTemplate
	name := "body"
	if cfg.BodyTemplateFile != "" {
		b, err := os.ReadFile(cfg.BodyTemplateFile)
		if err != nil {
			return nil, fmt.Errorf("read body_template_file: %w", err)
		}
		source = string(b)
		name = cfg.BodyTemplateFile
	}

	tmpl, err := template.New(name).Funcs(templateFuncs()).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse body template: %w", err)
	}
	return tmpl, nil
}
