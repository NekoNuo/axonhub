package schema

import (
	"context"

	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/scopes"
)

type RuntimeLog struct {
	ent.Schema
}

func (RuntimeLog) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (RuntimeLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at").
			StorageKey("runtime_logs_by_created_at"),
		index.Fields("level", "created_at").
			StorageKey("runtime_logs_by_level_created_at"),
		index.Fields("logger", "created_at").
			StorageKey("runtime_logs_by_logger_created_at"),
	}
}

func (RuntimeLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("logger"),
		field.Enum("level").Values("debug", "info", "warn", "error"),
		field.String("message"),
		field.String("caller").Optional().Default(""),
		field.String("trace_id").Optional().Nillable(),
		field.String("request_id").Optional().Nillable(),
		field.String("operation_name").Optional().Nillable(),
		field.Int("channel_id").Optional().Nillable(),
		field.String("channel_name").Optional().Nillable(),
		field.String("model_id").Optional().Nillable(),
		field.JSON("fields_json", map[string]any{}).Default(map[string]any{}),
	}
}

func (RuntimeLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (RuntimeLog) Policy() ent.Policy {
	return scopes.Policy{
		Query: scopes.QueryPolicy{
			privacy.ContextQueryMutationRule(func(ctx context.Context) error {
				principal, ok := authz.GetPrincipal(ctx)
				if ok && (principal.IsSystem() || principal.IsTest()) {
					return privacy.Allow
				}

				return privacy.Skip
			}),
			scopes.UserProjectScopeReadRule(scopes.ScopeReadRequests),
			scopes.OwnerRule(),
			scopes.UserReadScopeRule(scopes.ScopeReadRequests),
		},
		Mutation: scopes.MutationPolicy{
			privacy.ContextQueryMutationRule(func(ctx context.Context) error {
				principal, ok := authz.GetPrincipal(ctx)
				if ok && (principal.IsSystem() || principal.IsTest()) {
					return privacy.Allow
				}

				return privacy.Skip
			}),
		},
	}
}
