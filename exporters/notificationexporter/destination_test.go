// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package notificationexporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDestinations_RegistryMatchesSupportedTypes guards against a common
// mistake when adding a new destination: registering it in destinations
// (or supportedTypes) but forgetting the other, which would silently break
// Config.Validate's error message or make a type unreachable.
func TestDestinations_RegistryMatchesSupportedTypes(t *testing.T) {
	for _, typ := range supportedTypes {
		_, ok := destinations[typ]
		assert.True(t, ok, "supportedTypes entry %q has no destinations registration", typ)
	}
	assert.Len(t, destinations, len(supportedTypes), "destinations has entries not listed in supportedTypes")
}
