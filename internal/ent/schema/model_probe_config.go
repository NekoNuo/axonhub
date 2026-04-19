package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelProbeConfig struct {
	ent.Schema
}

func (ModelProbeConfig) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (ModelProbeConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("display_model"),
		field.Int("channel_id"),
		field.String("actual_model_id"),
		field.Bool("probe_enabled").Default(false),
		field.Int("consecutive_failures").Default(0),
		field.Int64("auto_disabled_at").Optional().Nillable(),
	}
}

func (ModelProbeConfig) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("channel", Channel.Type).
			Ref("model_probe_configs").
			Field("channel_id").
			Unique().
			Required(),
	}
}

func (ModelProbeConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("display_model", "channel_id", "actual_model_id").
			Unique(),
	}
}
