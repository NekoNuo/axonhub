package gql

import (
	"context"
	"testing"
	"time"

	"entgo.io/contrib/entgql"
	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/runtimelog"
)

func TestQueryResolver_RuntimeLogs(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	resolver := &queryResolver{&Resolver{client: client}}

	start := time.Date(2026, 4, 6, 15, 0, 0, 0, time.UTC)

	_, err := client.RuntimeLog.Create().
		SetCreatedAt(start.Add(-2 * time.Hour)).
		SetUpdatedAt(start.Add(-2 * time.Hour)).
		SetLogger("axonhub").
		SetLevel(runtimelog.LevelWarn).
		SetMessage("too early").
		SetFieldsJSON(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.RuntimeLog.Create().
		SetCreatedAt(start.Add(10 * time.Minute)).
		SetUpdatedAt(start.Add(10 * time.Minute)).
		SetLogger("axonhub").
		SetLevel(runtimelog.LevelInfo).
		SetMessage("info should be filtered out").
		SetFieldsJSON(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	expected, err := client.RuntimeLog.Create().
		SetCreatedAt(start.Add(20 * time.Minute)).
		SetUpdatedAt(start.Add(20 * time.Minute)).
		SetLogger("axonhub").
		SetLevel(runtimelog.LevelWarn).
		SetMessage("request process failed").
		SetChannelName("ggboom").
		SetFieldsJSON(map[string]any{"error": "503"}).
		Save(ctx)
	require.NoError(t, err)

	conn, err := resolver.RuntimeLogs(
		ctx,
		nil,
		intPtr(10),
		nil,
		nil,
		&ent.RuntimeLogOrder{
			Field:     ent.RuntimeLogOrderFieldCreatedAt,
			Direction: entgql.OrderDirectionDesc,
		},
		&ent.RuntimeLogWhereInput{
			CreatedAtGTE: &start,
			LevelIn:      []runtimelog.Level{runtimelog.LevelWarn, runtimelog.LevelError},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Len(t, conn.Edges, 1)
	require.NotNil(t, conn.Edges[0].Node)
	require.Equal(t, expected.ID, conn.Edges[0].Node.ID)
	require.Equal(t, "request process failed", conn.Edges[0].Node.Message)
	require.NotNil(t, conn.Edges[0].Node.ChannelName)
	require.Equal(t, "ggboom", *conn.Edges[0].Node.ChannelName)
}

func TestRuntimeLogResolver_ID(t *testing.T) {
	resolver := &runtimeLogResolver{&Resolver{}}

	guid, err := resolver.ID(context.Background(), &ent.RuntimeLog{ID: 42})
	require.NoError(t, err)
	require.NotNil(t, guid)
	require.Equal(t, ent.TypeRuntimeLog, guid.Type)
	require.Equal(t, 42, guid.ID)
}

func intPtr(v int) *int {
	return &v
}
