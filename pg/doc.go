// SPDX-License-Identifier: MIT

// Package main provides gostgrator‑pg, a PostgreSQL‑specific command‑line
// interface for the gostgrator migration library.
//
// # Install
//
//	go install github.com/bcomnes/gostgrator/pg@latest
//
// # Synopsis
//
//	gostgrator-pg [command] [arguments] [options]
//
// # Commands
//
//	migrate [target]    Apply all pending migrations up to *target* (default "max").
//	down   [steps]      Roll back the last *steps* migrations (default 1).
//	new    <desc>       Scaffold an empty migration pair labelled *desc*.
//	drop-schema         Delete the migration‑tracking table.
//	list                List available migrations and highlight the current version.
//
// # Global flags
//
//	-conn string               PostgreSQL connection URL. Overrides $DATABASE_URL and the
//	                           "conn" field in -config.
//	-config string             Optional JSON file that mirrors gostgrator.Config.
//	-migration-pattern string  Glob for locating *.sql migrations (default "migrations/*.sql").
//	-schema-table string       Table used to track migration state (default "schemaversion").
//	-mode string               Numbering mode for *new*: "int" or "timestamp" (default "int").
//	-help                      Show built‑in help.
//	-version                   Print gostgrator‑pg version.
//
// *Precedence:* -conn flag ➜ $DATABASE_URL ➜ "conn" in -config
//
// # Environment
//
//	DATABASE_URL  Connection URL used when -conn is omitted; overrides the "conn"
//	              value found in a JSON config file.
//
// Example:
//
//	postgres://user:pass@host:5432/dbname?sslmode=require
//
// # Examples
//
//	# Apply every migration in ./db/migrations
//	gostgrator-pg migrate -conn $DATABASE_URL \
//	    -migration-pattern "db/migrations/*.sql"
//
//	# Roll back the two most recent migrations
//	gostgrator-pg down 2
//
//	# Create a timestamp‑based migration called add-users-table
//	gostgrator-pg new "add-users-table" -mode timestamp
//
//	# Print migrations with the current version highlighted
//	gostgrator-pg list
//
// # Configuration file
//
// A JSON config file can replace most flags:
//
//	{
//	  "conn":             "postgres://user:pass@host:5432/db?sslmode=disable",
//	  "driver":           "pg",
//	  "schemaTable":      "public.schema_version",
//	  "migrationPattern": "sql/*.sql"
//	}
//
// Load it with:
//
//	gostgrator-pg migrate -config ./gostgrator.json
//
// # Exit status
//
// The program exits non‑zero on any error. Each command runs with a context that
// times out after ten minutes; modify the source if you need a different limit.
//
// For driver‑agnostic details see the root gostgrator package.
//
// Generated documentation; update when flags or behaviour change.
package main
