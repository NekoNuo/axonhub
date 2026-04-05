package gql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelSettingsSchemaExposesProbeEnabled(t *testing.T) {
	schemaPath := filepath.Join("model.graphql")

	content, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	schema := string(content)
	if !strings.Contains(schema, "type ModelSettings {\n  probeEnabled: Boolean!") {
		t.Fatalf("ModelSettings must expose probeEnabled, schema:\n%s", schema)
	}

	if !strings.Contains(schema, "input ModelSettingsInput {\n  probeEnabled: Boolean") {
		t.Fatalf("ModelSettingsInput must accept probeEnabled, schema:\n%s", schema)
	}
}
