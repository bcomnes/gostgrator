package gostgrator

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestCreateMigrationDiskSource(t *testing.T) {
	for _, variant := range []string{"legacy", "value", "pointer"} {
		for _, mode := range []string{"int", "timestamp"} {
			t.Run(variant+"/"+mode, func(t *testing.T) {
				dir := t.TempDir()
				const existing = "SELECT 1;\n"
				filename := filepath.Join(dir, "007.do.existing.sql")
				if err := os.WriteFile(filename, []byte(existing), 0600); err != nil {
					t.Fatal(err)
				}
				source := DiskMigrations{Pattern: filepath.Join(dir, "*.sql")}
				var cfg Config
				switch variant {
				case "legacy":
					cfg.MigrationPattern = source.Pattern
				case "value":
					cfg.Migrations = source
				case "pointer":
					cfg.Migrations = &source
				}
				before := time.Now().Unix()
				if err := CreateMigration(cfg, "Add Source Items", mode); err != nil {
					t.Fatal(err)
				}
				after := time.Now().Unix()
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 3 {
					t.Fatalf("directory entries = %v, %v; want 3", entries, err)
				}
				prefix := "008"
				if mode == "timestamp" {
					for _, entry := range entries {
						if strings.HasSuffix(entry.Name(), ".do.add-source-items.sql") {
							prefix = strings.Split(entry.Name(), ".")[0]
						}
					}
					stamp, err := strconv.ParseInt(prefix, 10, 64)
					if err != nil || stamp < before || stamp > after {
						t.Fatalf("timestamp = %q, %v; want [%d, %d]", prefix, err, before, after)
					}
				}
				for action, want := range map[string]string{
					"do":   "-- Write your migration SQL here\n",
					"undo": "-- Write your rollback SQL here\n",
				} {
					data, err := os.ReadFile(filepath.Join(dir, prefix+"."+action+".add-source-items.sql"))
					if err != nil || string(data) != want {
						t.Errorf("%s template = %q, %v; want %q", action, data, err, want)
					}
				}
				if data, err := os.ReadFile(filename); err != nil || string(data) != existing {
					t.Errorf("existing migration changed: %q, %v", data, err)
				}
			})
		}
	}
}

func TestCreateMigrationRejectsSource(t *testing.T) {
	var disk *DiskMigrations
	var virtual *FSMigrations
	files := fstest.MapFS{}
	for name, source := range map[string]MigrationSource{
		"FS value":            FSMigrations{FS: files, Pattern: "*.sql"},
		"FS pointer":          &FSMigrations{FS: files, Pattern: "*.sql"},
		"nil FS":              FSMigrations{Pattern: "*.sql"},
		"nil disk pointer":    disk,
		"nil FS pointer":      virtual,
		"empty disk":          DiskMigrations{},
		"empty disk pointer":  &DiskMigrations{},
		"malformed disk glob": DiskMigrations{Pattern: "["},
	} {
		t.Run(name, func(t *testing.T) {
			// Isolate accidental fallback writes to the current directory.
			dir := t.TempDir()
			t.Chdir(dir)
			if err := CreateMigration(Config{Migrations: source}, "Must Not Exist", "int"); err == nil {
				t.Fatal("CreateMigration accepted unsupported or invalid source")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("rejected source wrote files: %v, %v", entries, err)
			}
			if len(files) != 0 {
				t.Fatalf("rejected source modified FS: %v", files)
			}
		})
	}
}

func TestCreateMigrationRejectsConflictingSources(t *testing.T) {
	for _, kind := range []string{"disk", "FS"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			pattern := filepath.Join(dir, "*.sql")
			var source MigrationSource = DiskMigrations{Pattern: pattern}
			if kind == "FS" {
				source = FSMigrations{FS: fstest.MapFS{}, Pattern: "*.sql"}
			}
			if err := CreateMigration(Config{MigrationPattern: pattern, Migrations: source}, "Conflict", "int"); err == nil {
				t.Fatal("CreateMigration accepted both source settings")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("conflicting sources wrote files: %v, %v", entries, err)
			}
		})
	}
}
