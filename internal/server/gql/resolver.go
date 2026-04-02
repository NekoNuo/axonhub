package gql

import (
	"context"
	"errors"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/samber/lo"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/backup"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/internal/server/gc"
	"github.com/looplj/axonhub/internal/server/orchestrator"
	"github.com/looplj/axonhub/llm/httpclient"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

// ErrNotOwner is returned when a non-owner user attempts an owner-only operation.
var ErrNotOwner = errors.New("permission denied: owner access required")

// Resolver is the resolver root.
type Resolver struct {
	client                         *ent.Client
	authService                    *biz.AuthService
	apiKeyService                  *biz.APIKeyService
	userService                    *biz.UserService
	systemService                  *biz.SystemService
	channelService                 *biz.ChannelService
	requestService                 *biz.RequestService
	projectService                 *biz.ProjectService
	dataStorageService             *biz.DataStorageService
	roleService                    *biz.RoleService
	traceService                   *biz.TraceService
	threadService                  *biz.ThreadService
	channelOverrideTemplateService *biz.ChannelOverrideTemplateService
	modelService                   *biz.ModelService
	backupService                  *backup.BackupService
	channelProbeService            *biz.ChannelProbeService
	promptService                  *biz.PromptService
	promptProtectionRuleService    *biz.PromptProtectionRuleService
	providerQuotaService           *biz.ProviderQuotaService
	modelFetcher                   *biz.ModelFetcher
	TestChannelOrchestrator        *orchestrator.TestChannelOrchestrator
	gcWorker                       *gc.Worker
}

// NewSchema creates a graphql executable schema.
func NewSchema(
	client *ent.Client,
	authService *biz.AuthService,
	apiKeyService *biz.APIKeyService,
	userService *biz.UserService,
	systemService *biz.SystemService,
	channelService *biz.ChannelService,
	requestService *biz.RequestService,
	projectService *biz.ProjectService,
	dataStorageService *biz.DataStorageService,
	roleService *biz.RoleService,
	traceService *biz.TraceService,
	threadService *biz.ThreadService,
	usageLogService *biz.UsageLogService,
	channelOverrideTemplateService *biz.ChannelOverrideTemplateService,
	modelService *biz.ModelService,
	backupService *backup.BackupService,
	channelProbeService *biz.ChannelProbeService,
	promptService *biz.PromptService,
	promptProtectionRuleService *biz.PromptProtectionRuleService,
	providerQuotaService *biz.ProviderQuotaService,
	httpClient *httpclient.HttpClient,
	gcWorker *gc.Worker,
) graphql.ExecutableSchema {
	modelFetcher := biz.NewModelFetcher(httpClient, channelService)
	testChannelOrchestrator := orchestrator.NewTestChannelOrchestrator(
		channelService,
		requestService,
		systemService,
		usageLogService,
		promptProtectionRuleService,
		httpClient,
	)
	channelProbeService.SetIdleChannelModelProber(func(
		ctx context.Context,
		ch *ent.Channel,
		modelID string,
	) (time.Duration, bool, error) {
		start := time.Now()
		result, err := testChannelOrchestrator.TestChannel(
			ctx,
			objects.GUID{Type: "Channel", ID: ch.ID},
			lo.ToPtr(modelID),
			nil,
		)
		if err != nil {
			return time.Since(start), false, err
		}
		if result == nil || !result.Success {
			if result != nil && result.Error != nil {
				return time.Since(start), false, errors.New(*result.Error)
			}
			return time.Since(start), false, errors.New("model probe failed")
		}

		return time.Since(start), true, nil
	})

	return NewExecutableSchema(Config{
		Resolvers: &Resolver{
			client:                         client,
			authService:                    authService,
			apiKeyService:                  apiKeyService,
			userService:                    userService,
			systemService:                  systemService,
			channelService:                 channelService,
			requestService:                 requestService,
			projectService:                 projectService,
			dataStorageService:             dataStorageService,
			roleService:                    roleService,
			traceService:                   traceService,
			threadService:                  threadService,
			channelOverrideTemplateService: channelOverrideTemplateService,
			modelService:                   modelService,
			backupService:                  backupService,
			channelProbeService:            channelProbeService,
			promptService:                  promptService,
			promptProtectionRuleService:    promptProtectionRuleService,
			providerQuotaService:           providerQuotaService,
			modelFetcher:                   modelFetcher,
			TestChannelOrchestrator:        testChannelOrchestrator,
			gcWorker:                       gcWorker,
		},
	})
}
