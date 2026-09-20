// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter // import "github.com/ucpr/opentelemetry-collector-components/exporters/notificationexporter"

import (
	"fmt"

	"go.opentelemetry.io/collector/component"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
)

// parseCondition compiles an optional OTTL boolean condition string into a
// ConditionSequence over the log record context, using the same standard
// function set as filterprocessor's log_record conditions. An empty
// condition string is valid and returns (nil, nil): callers should treat a
// nil sequence as "match everything".
func parseCondition(condition string, telemetrySettings component.TelemetrySettings) (*ottl.ConditionSequence[*ottllog.TransformContext], error) {
	if condition == "" {
		return nil, nil
	}

	parser, err := ottllog.NewParser(ottlfuncs.StandardFuncs[*ottllog.TransformContext](), telemetrySettings)
	if err != nil {
		return nil, fmt.Errorf("build OTTL log parser: %w", err)
	}

	cond, err := parser.ParseCondition(condition)
	if err != nil {
		return nil, fmt.Errorf("parse condition %q: %w", condition, err)
	}

	seq := ottllog.NewConditionSequence([]*ottl.Condition[*ottllog.TransformContext]{cond}, telemetrySettings)
	return &seq, nil
}
