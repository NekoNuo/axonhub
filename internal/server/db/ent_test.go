package db

import "testing"

func boolPtr(v bool) *bool {
	return &v
}

func TestShouldUseDestructiveSchemaMigration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dialect string
		want    bool
	}{
		{name: "sqlite disables destructive migration", dialect: "sqlite3", want: false},
		{name: "sqlite alias disables destructive migration", dialect: "sqlite", want: false},
		{name: "postgres keeps destructive migration", dialect: "postgres", want: true},
		{name: "mysql keeps destructive migration", dialect: "mysql", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := shouldUseDestructiveSchemaMigration(tc.dialect)
			if got != tc.want {
				t.Fatalf("shouldUseDestructiveSchemaMigration(%q) = %v, want %v", tc.dialect, got, tc.want)
			}
		})
	}
}

func TestShouldAutoMigrateSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{name: "default enables auto migrate", cfg: Config{}, want: true},
		{name: "explicit enable keeps auto migrate", cfg: Config{AutoMigrate: boolPtr(true)}, want: true},
		{name: "explicit disable skips auto migrate", cfg: Config{AutoMigrate: boolPtr(false)}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := shouldAutoMigrateSchema(tc.cfg)
			if got != tc.want {
				t.Fatalf("shouldAutoMigrateSchema(%+v) = %v, want %v", tc.cfg, got, tc.want)
			}
		})
	}
}
