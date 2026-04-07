package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelHealthSnapshot struct {
	ent.Schema
}

func (ModelHealthSnapshot) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (ModelHealthSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.String("display_model"),
		field.Int("channel_id"),
		field.String("actual_model_id"),
		field.String("source").Default("associated"),
		field.Bool("is_healthy").Default(false),
		field.Bool("manual_override").Default(false),
		field.Int64("probed_at"),
	}
}

func (ModelHealthSnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("channel", Channel.Type).
			Ref("model_health_snapshots").
			Field("channel_id").
			Unique().
			Required(),
	}
}

func (ModelHealthSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("display_model", "channel_id", "actual_model_id").
			Unique(),
	}
}
