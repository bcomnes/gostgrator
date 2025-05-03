// SPDX-License-Identifier: MIT

// Package main provides gostgrator-sqlite, a SQLite-specific command-line
// interface for the gostgrator migration library.
//
// # Install
//
//	go get -tool github.com/bcomnes/gostgrator/sqlite@latest
//
// # Synopsis
//
//	gostgrator-sqlite [command] [arguments] [options]
//
// # Commands
//
//	migrate [target]    Apply all pending migrations up to *target* (default "max").
//	down   [steps]      Roll back the last *steps* migrations (default 1).
//	new    <desc>       Scaffold an empty migration pair labelled *desc*.
//	drop-schema         Delete the migration-tracking table.
//	list                List available migrations and highlight the current version.
//
// # Global flags
//
//	-conn string               SQLite connection string (file path). Falls back to $SQLITE_URL.
//	-config string             Optional JSON file that mirrors gostgrator.Config.
//	-migration-pattern string  Glob for locating *.sql migrations (default "migrations/*.sql").
//	-schema-table string       Table used to track migration state (default "schemaversion").
//	-mode string               Numbering mode for *new*: "int" or "timestamp" (default "int").
//	-help                      Show built-in help.
//	-version                   Print gostgrator-sqlite version.
//
// # Environment
//
//	SQLITE_URL  Connection string used when -conn is omitted.
//
// Example:
//
//	./data/dev.sqlite
//
// # Examples
//
//	# Apply every migration in ./sql
//	gostgrator-sqlite migrate -conn ./data/dev.sqlite \
//	    -migration-pattern "sql/*.sql"
//
//	# Roll back the two most recent migrations
//	gostgrator-sqlite down 2
//
//	# Create a timestamp-based migration called create-users
//	gostgrator-sqlite new "create-users" -mode timestamp
//
//	# Print migrations with the current version highlighted
//	gostgrator-sqlite list
//
// # Configuration file
//
// A JSON config file can replace most flags:
//
//	{
//	  "driver":           "sqlite3",
//	  "schemaTable":      "schema_version",
//	  "migrationPattern": "sql/*.sql"
//	}
//
// Load it with:
//
//	gostgrator-sqlite migrate -config ./gostgrator.json
//
// # Exit status
//
// The program exits non-zero on any error. Each command respects context
// cancellation and times out after ten minutes.
//
// For driver-agnostic details see the root gostgrator package.
//
// Generated documentation; update when flags or behaviour change.
package main
