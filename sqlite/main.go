// Package main implements a SQLite-specific CLI for gostgrator.
// It accepts a connection URL via the -conn flag or SQLITE_URL environment variable
// (typically a file path like "./db.sqlite") along with options for migrations.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"github.com/bcomnes/gostgrator"
)

var versionString = gostgrator.Version

// usage prints the help text.
func usage() {
	header := `Usage:
  gostgrator-sqlite [command] [arguments] [options]

Commands:
  migrate [target]    Migrate the schema to a target version (default: "max").
  down [steps]        Roll back the specified number of migrations (default: 1).
  new <desc>          Create a new empty migration pair with the provided description.
  drop-schema         Drop the schema version table.
  list                List available migrations and annotate the migration matching the database version.

Options:`
	fmt.Fprintln(os.Stderr, header)
	flag.PrintDefaults()
}

func main() {
	// Define global flags.
	connStr := flag.String("conn", "", "SQLite connection URL (typically a file path, e.g., \"./db.sqlite\"). Can also be set via SQLITE_URL env var.")
	configPath := flag.String("config", "", "Path to JSON configuration file (optional)")
	migrationPattern := flag.String("migration-pattern", "migrations/*.sql", "Glob pattern for migration files")
	schemaTable := flag.String("schema-table", "schemaversion", "Name of the schema table")
	mode := flag.String("mode", "int", "Migration numbering mode (\"int\" or \"timestamp\") for new command")
	helpFlag := flag.Bool("help", false, "Show help message")
	versionFlag := flag.Bool("version", false, "Show version")

	flag.Usage = usage
	flag.Parse()

	// Safeguard: check for any flag-like arguments after positional arguments.
	for _, arg := range flag.Args() {
		if strings.HasPrefix(arg, "-") {
			fmt.Fprintln(os.Stderr, "Error: Flags must be specified before the command. Please reorder your arguments.")
			usage()
			os.Exit(1)
		}
	}

	// Process global flags.
	if *helpFlag {
		usage()
		os.Exit(0)
	}
	if *versionFlag {
		fmt.Println("gostgrator-sqlite version:", versionString)
		os.Exit(0)
	}

	// Load configuration from file if provided.
	cliConfig := gostgrator.Config{
		Driver:           "sqlite3",
		SchemaTable:      *schemaTable,
		MigrationPattern: *migrationPattern,
	}
	if *configPath != "" {
		if err := loadConfig(*configPath, &cliConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config file: %v\n", err)
			os.Exit(1)
		}
	}

	// Process positional arguments.
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: no command provided.")
		usage()
		os.Exit(1)
	}
	command := args[0]

	switch command {
	case "migrate":
		target := "max"
		if len(args) > 1 {
			target = args[1]
		}
		withDB(cliConfig, *connStr, func(g *gostgrator.Gostgrator, ctx context.Context) {
			fmt.Printf("[%s] Starting migration to version %s...\n", time.Now().Format(time.Kitchen), target)
			applied, err := g.Migrate(ctx, target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Migration error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("[%s] Applied %d migrations:\n", time.Now().Format(time.Kitchen), len(applied))
			for _, m := range applied {
				fmt.Printf("  - Version %d: %s (%s)\n", m.Version, m.Name, m.Filename)
			}
		})
	case "down":
		steps := 1
		if len(args) > 1 {
			var err error
			steps, err = strconv.Atoi(args[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Invalid rollback steps: %s\n", args[1])
				os.Exit(1)
			}
		}
		withDB(cliConfig, *connStr, func(g *gostgrator.Gostgrator, ctx context.Context) {
			fmt.Printf("[%s] Rolling back %d migration(s)...\n", time.Now().Format(time.Kitchen), steps)
			applied, err := g.Down(ctx, steps)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Rollback error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("[%s] Rolled back %d migration(s):\n", time.Now().Format(time.Kitchen), len(applied))
			for _, m := range applied {
				fmt.Printf("  - Rolled back version %d: %s (%s)\n", m.Version, m.Name, m.Filename)
			}
		})
	case "drop-schema":
		withDB(cliConfig, *connStr, func(g *gostgrator.Gostgrator, ctx context.Context) {
			fmt.Printf("[%s] Dropping schema table...\n", time.Now().Format(time.Kitchen))
			if err := dropSchema(ctx, cliConfig, g); err != nil {
				fmt.Fprintf(os.Stderr, "Error dropping schema table: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("[%s] Schema table dropped.\n", time.Now().Format(time.Kitchen))
		})
	case "new":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: a description is required for the new command.")
			usage()
			os.Exit(1)
		}
		description := args[1]
		g, err := gostgrator.NewGostgrator(cliConfig, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing gostgrator: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[%s] Creating new migration with description '%s' in %s mode...\n", time.Now().Format(time.Kitchen), description, *mode)
		if err := g.CreateMigration(description, *mode); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating new migration: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[%s] New migration created successfully.\n", time.Now().Format(time.Kitchen))
	case "list":
		withDB(cliConfig, *connStr, func(g *gostgrator.Gostgrator, ctx context.Context) {
			current, err := g.GetDatabaseVersion(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error fetching current database version: %v\n", err)
				os.Exit(1)
			}
			migs, err := g.GetMigrations()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading migrations: %v\n", err)
				os.Exit(1)
			}
			sort.Slice(migs, func(i, j int) bool { return migs[i].Version < migs[j].Version })
			fmt.Printf("Current database migration version: %d\n", current)
			fmt.Println("Available migrations:")
			for _, m := range migs {
				annot := ""
				if m.Version == current {
					annot = " <== current"
				}
				fmt.Printf("Version %d: %s (%s)%s\n", m.Version, m.Name, m.Filename, annot)
			}
		})
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		usage()
		os.Exit(1)
	}
}

func withDB(cliConfig gostgrator.Config, connStr string, f func(g *gostgrator.Gostgrator, ctx context.Context)) {
	if connStr == "" {
		connStr = os.Getenv("SQLITE_URL")
	}
	if connStr == "" {
		fmt.Fprintln(os.Stderr, "Error: connection URL must be provided via -conn flag or SQLITE_URL environment variable")
		usage()
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	g, err := gostgrator.NewGostgrator(cliConfig, db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing gostgrator: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	f(g, ctx)
}

func loadConfig(path string, cfg *gostgrator.Config) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(cfg)
}

func dropSchema(ctx context.Context, cfg gostgrator.Config, g *gostgrator.Gostgrator) error {
	query := fmt.Sprintf("DROP TABLE %s", cfg.SchemaTable)
	_, err := g.QueryContext(ctx, query)
	return err
}
