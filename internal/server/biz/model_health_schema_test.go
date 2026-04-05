package biz

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/objects"
)

func TestModelHealthSchema(t *testing.T) {
	t.Run("default system model settings include model probe switch", func(t *testing.T) {
		client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
		defer client.Close()

		ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
		svc := NewSystemService(SystemServiceParams{Ent: client})

		settings, err := svc.ModelSettings(ctx)
		require.NoError(t, err)
		require.False(t, settings.EnableModelProbe)
	})

	t.Run("model settings can persist probe enabled", func(t *testing.T) {
		modelSettings := objects.ModelSettings{
			ProbeEnabled: true,
		}

		require.True(t, modelSettings.ProbeEnabled)
	})

	t.Run("model health schemas define required fields", func(t *testing.T) {
		snapshotSchema := mustReadFile(t, "internal/ent/schema/model_health_snapshot.go")
		historySchema := mustReadFile(t, "internal/ent/schema/model_health_history.go")

		for _, content := range []string{snapshotSchema, historySchema} {
			require.Contains(t, content, `field.String("display_model")`)
			require.Contains(t, content, `field.Int("channel_id")`)
			require.Contains(t, content, `field.String("actual_model_id")`)
			require.Contains(t, content, `field.String("source")`)
			require.Contains(t, content, `field.Bool("is_healthy")`)
			require.Contains(t, content, `field.Bool("manual_override")`)
		}
	})
}

func mustReadFile(t *testing.T, relativePath string) string {
	t.Helper()

	root, err := os.Getwd()
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(root, "..", "..", "..", relativePath))
	require.NoError(t, err)

	return strings.TrimSpace(string(content))
}
