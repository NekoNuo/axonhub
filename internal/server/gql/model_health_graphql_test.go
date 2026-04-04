package gql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelHealthGraphQL(t *testing.T) {
	t.Run("system model settings expose enable model probe", func(t *testing.T) {
		systemSchema := mustReadGraphQLFile(t, "system.graphql")

		require.Contains(t, systemSchema, "enableModelProbe: Boolean!")
		require.Contains(t, systemSchema, "enableModelProbe: Boolean")
	})

	t.Run("model health query types are declared", func(t *testing.T) {
		modelHealthSchema := mustReadGraphQLFile(t, "model_health.graphql")

		require.Contains(t, modelHealthSchema, "ModelHealthSnapshot")
		require.Contains(t, modelHealthSchema, "ModelHealthHistory")
		require.Contains(t, modelHealthSchema, "modelHealthSnapshots")
		require.Contains(t, modelHealthSchema, "modelHealthHistory")
	})

	t.Run("manual model probe mutation is declared", func(t *testing.T) {
		modelHealthSchema := mustReadGraphQLFile(t, "model_health.graphql")

		require.Contains(t, modelHealthSchema, "manualModelProbe")
		require.Contains(t, modelHealthSchema, "input ManualModelProbeInput")
	})
}

func mustReadGraphQLFile(t *testing.T, name string) string {
	t.Helper()

	root, err := os.Getwd()
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(root, name))
	require.NoError(t, err)

	return strings.TrimSpace(string(content))
}
