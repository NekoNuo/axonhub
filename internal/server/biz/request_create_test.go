package biz

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/project"
	"github.com/looplj/axonhub/internal/ent/user"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

func TestRequestService_CreateRequest_UsesAPIKeyProjectIDFallback(t *testing.T) {
	svc, client, ctx := setupTestRequestService(t)
	defer client.Close()

	proj, err := client.Project.Create().
		SetName("test-project").
		SetStatus(project.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	owner, err := client.User.Create().
		SetEmail(fmt.Sprintf("request-create-%d@example.com", time.Now().UnixNano())).
		SetPassword("password").
		SetFirstName("Test").
		SetLastName("Owner").
		SetStatus(user.StatusActivated).
		Save(ctx)
	require.NoError(t, err)

	apiKeyEntity, err := client.APIKey.Create().
		SetName("test-key").
		SetKey("ah-test-key").
		SetProjectID(proj.ID).
		SetUserID(owner.ID).
		SetType(apikey.TypeUser).
		SetStatus(apikey.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	ctx = contexts.WithAPIKey(ctx, &ent.APIKey{
		ID:        apiKeyEntity.ID,
		ProjectID: proj.ID,
	})

	req, err := svc.CreateRequest(
		ctx,
		&llm.Request{
			Model: "gpt-5.4",
		},
		&httpclient.Request{
			JSONBody: []byte(`{"model":"gpt-5.4"}`),
		},
		llm.APIFormatOpenAIResponse,
	)
	require.NoError(t, err)
	require.Equal(t, proj.ID, req.ProjectID)
}

func TestRequestService_CreateRequest_PersistsEndpointAndUserAgent(t *testing.T) {
	svc, client, ctx := setupTestRequestService(t)
	defer client.Close()

	proj, err := client.Project.Create().
		SetName("request-log-project").
		SetStatus(project.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	ctx = contexts.WithProjectID(ctx, proj.ID)

	req, err := svc.CreateRequest(
		ctx,
		&llm.Request{
			Model: "gpt-5.4",
		},
		&httpclient.Request{
			Path: "/v1/responses",
			Headers: http.Header{
				"User-Agent": []string{"codex-test/1.0"},
			},
			JSONBody: []byte(`{"model":"gpt-5.4"}`),
		},
		llm.APIFormatOpenAIResponse,
	)
	require.NoError(t, err)
	require.Equal(t, "/v1/responses", readRequestStringField(t, req, "RequestPath"))
	require.Equal(t, "codex-test/1.0", readRequestStringField(t, req, "UserAgent"))
}

func readRequestStringField(t *testing.T, req *ent.Request, fieldName string) string {
	t.Helper()

	value := reflect.ValueOf(req)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	field := value.FieldByName(fieldName)
	require.True(t, field.IsValid(), "expected request to have field %s", fieldName)
	require.Equal(t, reflect.String, field.Kind(), "expected %s to be a string field", fieldName)

	return field.String()
}
