package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/migrate"
	"github.com/looplj/axonhub/internal/ent/migrate/datamigrate"
	"github.com/looplj/axonhub/internal/ent/migrate/schemahook"
	_ "github.com/looplj/axonhub/internal/ent/runtime"
	_ "github.com/looplj/axonhub/internal/pkg/sqlite"
)

func shouldUseDestructiveSchemaMigration(dialectName string) bool {
	switch dialectName {
	case "sqlite3", "sqlite":
		return false
	default:
		return true
	}
}

func shouldAutoMigrateSchema(cfg Config) bool {
	return cfg.AutoMigrate == nil || *cfg.AutoMigrate
}

func schemaMigrateOptions(cfg Config) []schema.MigrateOption {
	opts := []schema.MigrateOption{
		migrate.WithGlobalUniqueID(false),
		migrate.WithForeignKeys(false),
		schema.WithHooks(schemahook.V0_3_0),
	}

	// SQLite destructive schema reconciliation can trigger expensive table/index
	// rebuilds on startup. Keep startup migration additive-only there.
	if shouldUseDestructiveSchemaMigration(cfg.Dialect) {
		opts = append(opts,
			migrate.WithDropIndex(true),
			migrate.WithDropColumn(true),
		)
	}

	return opts
}

func startupProgressf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "[startup] "+format+"\n", args...)
}

func NewEntClient(cfg Config) *ent.Client {
	var opts []ent.Option
	if cfg.Debug {
		opts = append(opts, ent.Debug())
	}

	var (
		sqlDB     *sql.DB
		dbDialect string
		err       error
	)

	switch cfg.Dialect {
	case "postgres", "pgx", "postgresdb", "pg", "postgresql":
		sqlDB, err = sql.Open("pgx", cfg.DSN)
		if err != nil {
			panic(err)
		}

		dbDialect = dialect.Postgres
	case "sqlite3", "sqlite":
		sqlDB, err = sql.Open("sqlite3", cfg.DSN)
		if err != nil {
			panic(err)
		}

		dbDialect = dialect.SQLite
	case "mysql", "tidb":
		sqlDB, err = sql.Open("mysql", cfg.DSN)
		if err != nil {
			panic(err)
		}

		dbDialect = dialect.MySQL
	default:
		panic(fmt.Errorf("invalid dialect: %s", cfg.Dialect))
	}

	drv := entsql.OpenDB(dbDialect, sqlDB)
	opts = append(opts, ent.Driver(drv))
	client := ent.NewClient(opts...)

	startedAt := time.Now()
	startupProgressf("database connected dialect=%s", cfg.Dialect)
	if shouldAutoMigrateSchema(cfg) {
		startupProgressf("schema migration started destructive=%t", shouldUseDestructiveSchemaMigration(cfg.Dialect))

		err = client.Schema.Create(context.Background(), schemaMigrateOptions(cfg)...)
		if err != nil {
			panic(err)
		}
		startupProgressf("schema migration finished duration=%s", time.Since(startedAt))
	} else {
		startupProgressf("schema migration skipped auto_migrate=false")
	}

	// Run data migrations using the Migrator framework
	ctx := context.Background()

	startedAt = time.Now()
	startupProgressf("data migration started")
	migrator := datamigrate.NewMigrator(client)
	if err := migrator.Run(ctx); err != nil {
		panic(err)
	}
	startupProgressf("data migration finished duration=%s", time.Since(startedAt))

	return client
}
