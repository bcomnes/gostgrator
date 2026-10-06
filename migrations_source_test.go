package gostgrator

import (
	"crypto/md5"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

//go:embed testdata/source_sqlite
var sourceFixtures embed.FS

func sourceFixtureFS(t *testing.T) fs.FS {
	t.Helper()
	root, err := fs.Sub(sourceFixtures, "testdata/source_sqlite")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestMigrationSourceDiskEmbedParity(t *testing.T) {
	disk := DiskMigrations{Pattern: "testdata/source_sqlite/*.sql"}
	embedded := FSMigrations{FS: sourceFixtureFS(t), Pattern: "*.sql"}
	configs := map[string]Config{
		"legacy":       {MigrationPattern: disk.Pattern},
		"disk value":   {Migrations: disk},
		"disk pointer": {Migrations: &disk},
		"FS value":     {Migrations: embedded},
		"FS pointer":   {Migrations: &embedded},
	}
	want, err := getMigrations(configs["legacy"])
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 4 {
		t.Fatalf("fixture migrations = %d, want 4", len(want))
	}
	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			got, err := getMigrations(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want) {
				t.Fatalf("migrations = %d, want %d", len(got), len(want))
			}
			for i, m := range got {
				w := want[i]
				if m.Version != w.Version || m.Action != w.Action || m.Name != w.Name || m.Md5 != w.Md5 || filepath.Base(m.Filename) != filepath.Base(w.Filename) {
					t.Errorf("migration metadata = %+v, want %+v (apart from source path)", m, w)
				}
				sql, err := m.getSQL()
				if err != nil {
					t.Fatal(err)
				}
				wantSQL, err := w.getSQL()
				if err != nil {
					t.Fatal(err)
				}
				if sql != wantSQL {
					t.Errorf("SQL = %q, want %q", sql, wantSQL)
				}
			}
		})
	}
}

func TestMigrationSourceValidation(t *testing.T) {
	var disk *DiskMigrations
	var virtual *FSMigrations
	var nilFS *embed.FS
	validFS := fstest.MapFS{"001.do.sql": {Data: []byte("SELECT 1;")}}
	cases := map[string]Config{
		"empty disk":           {Migrations: DiskMigrations{}},
		"empty disk pointer":   {Migrations: &DiskMigrations{}},
		"empty FS pattern":     {Migrations: FSMigrations{FS: validFS}},
		"nil FS":               {Migrations: FSMigrations{Pattern: "*.sql"}},
		"typed nil FS":         {Migrations: FSMigrations{FS: nilFS, Pattern: "*.sql"}},
		"malformed disk":       {Migrations: DiskMigrations{Pattern: "["}},
		"malformed FS":         {Migrations: FSMigrations{FS: validFS, Pattern: "["}},
		"nil FS in pointer":    {Migrations: &FSMigrations{Pattern: "*.sql"}},
		"nil disk pointer":     {Migrations: disk},
		"nil FS pointer":       {Migrations: virtual},
		"disk conflict":        {MigrationPattern: "*.sql", Migrations: DiskMigrations{Pattern: "*.sql"}},
		"FS conflict":          {MigrationPattern: "*.sql", Migrations: FSMigrations{FS: validFS, Pattern: "*.sql"}},
		"nil pointer conflict": {MigrationPattern: "*.sql", Migrations: disk},
	}
	for _, pattern := range []string{"./*.sql", "../*.sql", "/migrations/*.sql", "migrations/../*.sql", "migrations/./*.sql"} {
		cases[pattern] = Config{Migrations: FSMigrations{FS: validFS, Pattern: pattern}}
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := getMigrations(cfg); err == nil {
				t.Fatal("getMigrations accepted invalid source")
			}
			cfg.Driver = "sqlite"
			if _, err := NewGostgrator(cfg, nil); err == nil {
				t.Fatal("NewGostgrator accepted invalid source")
			}
		})
	}
}

func TestMigrationSourceMapFSDiscovery(t *testing.T) {
	files := fstest.MapFS{
		"migrations/001.do.create.items.sql":   {Data: []byte("SELECT 1;\n")},
		"migrations/001.undo.create.items.sql": {Data: []byte("SELECT 2;\n")},
		"migrations/readme.txt":                {Data: []byte("ignored")},
		"migrations/not-a-version.do.sql":      {Data: []byte("ignored")},
		"migrations/002.sql":                   {Data: []byte("ignored")},
		"elsewhere/002.do.sql":                 {Data: []byte("SELECT 3;")},
	}
	migs, err := getMigrations(Config{Migrations: FSMigrations{FS: files, Pattern: "migrations/*"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) != 2 {
		t.Fatalf("migrations = %d, want 2", len(migs))
	}
	for i, action := range []string{"do", "undo"} {
		m := migs[i]
		if m.Version != 1 || m.Action != action || m.Name != "create.items" || m.Filename != "migrations/001."+action+".create.items.sql" {
			t.Errorf("unexpected migration: %+v", m)
		}
	}
	for _, pattern := range []string{"missing/*.sql", "migrations/*.missing"} {
		migs, err := getMigrations(Config{Migrations: FSMigrations{FS: files, Pattern: pattern}})
		if err != nil || len(migs) != 0 {
			t.Errorf("no matches: migrations = %v, error = %v", migs, err)
		}
	}
	files["migrations/1.do.duplicate.sql"] = &fstest.MapFile{Data: []byte("SELECT 4;")}
	if _, err := getMigrations(Config{Migrations: FSMigrations{FS: files, Pattern: "migrations/*.sql"}}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate version/action error = %v", err)
	}
}

func TestMigrationSourceNewlinesAndChecksums(t *testing.T) {
	const raw = "SELECT 1;\r\nSELECT 2;\rSELECT 3;\n"
	for _, tc := range []struct{ newline, want string }{
		{"", raw},
		{"LF", "SELECT 1;\nSELECT 2;\nSELECT 3;\n"},
		{"CR", "SELECT 1;\rSELECT 2;\rSELECT 3;\r"},
		{"CRLF", "SELECT 1;\r\nSELECT 2;\r\nSELECT 3;\r\n"},
	} {
		t.Run("newline="+tc.newline, func(t *testing.T) {
			files := fstest.MapFS{"001.do.sql": {Data: []byte(raw)}}
			migs, err := getMigrations(Config{Migrations: FSMigrations{FS: files, Pattern: "*.sql"}, Newline: tc.newline})
			if err != nil {
				t.Fatal(err)
			}
			if len(migs) != 1 {
				t.Fatalf("migrations = %d, want 1", len(migs))
			}
			want := fmt.Sprintf("%x", md5.Sum([]byte(tc.want)))
			if migs[0].Md5 != want {
				t.Errorf("checksum = %s, want %s", migs[0].Md5, want)
			}
			if got, err := migs[0].getSQL(); err != nil || got != raw {
				t.Errorf("getSQL = %q, %v; want unmodified SQL %q", got, err, raw)
			}
		})
	}
	files := fstest.MapFS{"001.do.sql": {Data: []byte(raw)}}
	if _, err := getMigrations(Config{Migrations: FSMigrations{FS: files, Pattern: "*.sql"}, Newline: "invalid"}); err == nil {
		t.Fatal("accepted invalid newline")
	}
}

// Keep directory discovery working while making a matched file unreadable.
type sourceReadErrorFS struct{ files fstest.MapFS }

func (f sourceReadErrorFS) Open(name string) (fs.File, error) {
	if name == "001.do.sql" {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return f.files.Open(name)
}

func TestMigrationSourceReadErrors(t *testing.T) {
	files := fstest.MapFS{"001.do.sql": {Data: []byte("SELECT 1;")}}
	if _, err := getMigrations(Config{Migrations: FSMigrations{FS: sourceReadErrorFS{files}, Pattern: "*.sql"}}); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("read error = %v, want permission error", err)
	}
	for name, cfg := range map[string]Config{
		"disk": {Migrations: DiskMigrations{Pattern: "["}},
		"FS":   {Migrations: FSMigrations{FS: files, Pattern: "["}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := getMigrations(cfg); err == nil {
				t.Fatal("accepted malformed glob")
			}
		})
	}
	migs, err := getMigrations(Config{Migrations: FSMigrations{FS: files, Pattern: "*.sql"}})
	if err != nil || len(migs) != 1 {
		t.Fatalf("discovery = %v, %v", migs, err)
	}
	delete(files, "001.do.sql")
	if _, err := migs[0].getSQL(); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("getSQL after removal = %v, want not-exist error", err)
	}
}

func TestMigrationSourceComparability(t *testing.T) {
	configs := map[string]Config{
		"MapFS": {Migrations: FSMigrations{
			FS: fstest.MapFS{"001.do.sql": {Data: []byte("SELECT 1;")}}, Pattern: "*.sql",
		}},
		"embed": {Migrations: FSMigrations{FS: sourceFixtureFS(t), Pattern: "*.sql"}},
		"disk":  {Migrations: DiskMigrations{Pattern: "testdata/source_sqlite/*.sql"}},
	}
	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			migs, err := getMigrations(cfg)
			if err != nil || len(migs) == 0 {
				t.Fatalf("getMigrations = %v, %v", migs, err)
			}
			original := migs[0]
			copied := original
			t.Run("equality", func(t *testing.T) {
				if copied != original {
					t.Fatal("a copied migration must compare equal")
				}
			})
			t.Run("map key", func(t *testing.T) {
				seen := map[Migration]bool{original: true}
				if !seen[copied] {
					t.Fatal("could not look up a copied migration")
				}
			})
			want, err := original.getSQL()
			if err != nil {
				t.Fatal(err)
			}
			if got, err := copied.getSQL(); err != nil || got != want {
				t.Fatalf("copied migration SQL = %q, %v; want %q", got, err, want)
			}
		})
	}
}

func TestMigrationSourceManualMigrationUsesDisk(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "001.do.sql")
	const content = "SELECT 42;\n"
	if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	m := Migration{Filename: filename}
	if got, err := m.getSQL(); err != nil || got != content {
		t.Fatalf("getSQL = %q, %v; want %q", got, err, content)
	}
}
