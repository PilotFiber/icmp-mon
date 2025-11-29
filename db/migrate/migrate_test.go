package migrate

import (
	"testing"
)

func TestParseMigrationFilename(t *testing.T) {
	tests := []struct {
		filename    string
		wantVersion int
		wantName    string
		wantErr     bool
	}{
		{"001_initial_schema.sql", 1, "initial_schema", false},
		{"021_agent_status_function.sql", 21, "agent_status_function", false},
		{"100_future_migration.sql", 100, "future_migration", false},
		{"001_name_with_underscores.sql", 1, "name_with_underscores", false},
		{"invalid.sql", 0, "", true},
		{"abc_name.sql", 0, "", true},
		{"001.sql", 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			version, name, err := parseMigrationFilename(tt.filename)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %s, got nil", tt.filename)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error for %s: %v", tt.filename, err)
				return
			}

			if version != tt.wantVersion {
				t.Errorf("version: got %d, want %d", version, tt.wantVersion)
			}
			if name != tt.wantName {
				t.Errorf("name: got %s, want %s", name, tt.wantName)
			}
		})
	}
}

func TestGetAvailableMigrations(t *testing.T) {
	migrations, err := getAvailableMigrations()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(migrations) == 0 {
		t.Fatal("expected at least one migration, got none")
	}

	// Verify they're sorted by version
	for i := 1; i < len(migrations); i++ {
		if migrations[i].version <= migrations[i-1].version {
			t.Errorf("migrations not sorted: %d comes after %d",
				migrations[i].version, migrations[i-1].version)
		}
	}

	// Verify first migration is 001
	if migrations[0].version != 1 {
		t.Errorf("first migration version: got %d, want 1", migrations[0].version)
	}

	// Verify migrations have SQL content
	for _, m := range migrations {
		if m.sql == "" {
			t.Errorf("migration %d (%s) has empty SQL", m.version, m.name)
		}
	}
}

func TestMigrationFilesAreEmbedded(t *testing.T) {
	// Verify that the embed directive is working
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("failed to read embedded migrations: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("no migration files embedded")
	}

	// Count SQL files
	sqlCount := 0
	for _, entry := range entries {
		if !entry.IsDir() && len(entry.Name()) > 4 {
			sqlCount++
		}
	}

	if sqlCount == 0 {
		t.Fatal("no SQL files found in embedded migrations")
	}

	t.Logf("found %d embedded migration files", sqlCount)
}

func TestMigration021Exists(t *testing.T) {
	// Specifically verify our new migration is included
	migrations, err := getAvailableMigrations()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, m := range migrations {
		if m.version == 21 && m.name == "agent_status_function" {
			found = true
			// Verify it contains the function definition
			if !contains(m.sql, "CREATE OR REPLACE FUNCTION get_agent_status") {
				t.Error("migration 021 doesn't contain get_agent_status function")
			}
			break
		}
	}

	if !found {
		t.Error("migration 021_agent_status_function.sql not found")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
