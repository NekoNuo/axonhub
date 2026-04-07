package biz

import (
	"fmt"
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
