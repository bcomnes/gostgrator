// SPDX-License-Identifier: MIT

// Package gostgrator provides database-agnostic schema migration utilities
// for Go applications.  It loads *.sql* migration files, tracks execution
// state in a table you choose, and moves the database forward or backward
// to any version you specify.
//
// A thin driver layer (currently PostgreSQL and SQLite) supplies SQL
// dialect differences.  Companion CLI tools live under sub-packages
// *pg* and *sqlite*; the core logic is here.
//
// # Install
//
//	go get github.com/bcomnes/gostgrator@latest
//
// # Quick start
//
//	import (
//	    "context"
//	    "database/sql"
//
//	    _ "github.com/jackc/pgx/v5/stdlib" // or sqlite3
//	    "github.com/bcomnes/gostgrator"
//	)
//
//	func main() {
//	    db, _ := sql.Open("pgx", os.Getenv("DATABASE_URL"))
//	    cfg := gostgrator.Config{
//	        Driver:           "pg",
//	        MigrationPattern: "migrations/*.sql",
//	    }
//
//	    g, _ := gostgrator.NewGostgrator(cfg, db)
//	    g.Migrate(context.Background(), "max")
//	}
//
// # Configuration
//
// Use Config to tweak behaviour:
//
//   - Driver            — database driver name ("pg", "sqlite3")
//   - SchemaTable       — table that stores migration state (default "schemaversion")
//   - MigrationPattern  — glob for locating migration files
//   - Newline           — line-ending style when scaffolding new migrations
//   - ValidateChecksums — compare MD5 hashes before running *up* migrations
//
// You can merge Config with your own JSON/YAML file or set it inline.
//
// # Migration files
//
// A migration *pair* is two files with the same version and name:
//
//	001.do.create_users.sql   // apply
//	001.undo.create_users.sql // roll back
//
// Versions may be plain integers (*001*, *002*, …) or timestamps if you
// prefer.  The CLI’s *new* command scaffolds these files for you.
//
// # Programmatic API
//
//	NewGostgrator(cfg, db)        → *Gostgrator
//	(*Gostgrator).Migrate(ctx, v) → []Migration, error
//	(*Gostgrator).Down(ctx, n)    → []Migration, error
//	(*Gostgrator).GetMigrations() → []Migration, error
//	(*Gostgrator).GetDatabaseVersion(ctx) → int, error
//
// All operations are context-aware; cancel the context to abort long runs.
//
// # CLI helpers
//
// If you prefer shell commands, install driver-specific binaries:
//
//	go get -tool github.com/bcomnes/gostgrator/pg@latest      # PostgreSQL
//	go get -tool github.com/bcomnes/gostgrator/sqlite@latest  # SQLite
//
// See each sub-package’s doc for flags and usage.
//
// # Exit codes
//
// The library returns errors; the CLIs exit with non-zero status on any
// failure.  ValidateErrors include version and MD5 info for easy triage.
//
// # Versioning
//
// A semantic version string is exposed as:
//
//	var Version = "vX.Y.Z"
//
// Embed it in your own commands to surface gostgrator’s build version.
//
// Generated documentation; update whenever public API or CLI flags change.
package gostgrator
