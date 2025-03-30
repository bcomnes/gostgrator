// Package main implements a PostgreSQL-specific CLI for gostgrator.
// It accepts a connection URL (e.g., "postgres://user:pass@host:port/dbname?sslmode=require")
// along with a few options for migrations.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/bcomnes/gostgrator/pkg/gostgrator"
)

const version = "1.0.0"

func usage() {
	helpText := `
Usage:
  gostgrator-pg [command] [options]

Commands:
  migrate       Migrate the schema to a target version.
                Optionally specify a target version (number or "max", default "max").
  drop-schema   Drop the schema version table.

Options:
  -conn string
        PostgreSQL connection URL (e.g., "postgres://user:pass@host:port/dbname?sslmode=require")
  -config string
        Path to JSON configuration file (optional)
  -migration-pattern string
        Glob pattern for migration files (default "migrations/*.sql")
  -schema-table string
        Name of the schema table (default "schemaversion")
  -to string
        Target version to migrate to (default "max")
  -help
        Show help message.
  -version
        Show version.
`
	fmt.Fprintln(os.Stderr, helpText)
}

func main() {
	// Define flags.
	connStr := flag.String("conn", "", "PostgreSQL connection URL")
	configPath := flag.String("config", "", "Path to JSON configuration file (optional)")
	migrationPattern := flag.String("migration-pattern", "migrations/*.sql", "Glob pattern for migration files")
	schemaTable := flag.String("schema-table", "schemaversion", "Name of the schema table")
	target := flag.String("to", "max", "Target version to migrate to")
	helpFlag := flag.Bool("help", false, "Show help message")
	versionFlag := flag.Bool("version", false, "Show version")

	flag.Usage = usage
	flag.Parse()

	if *helpFlag {
		usage()
		os.Exit(0)
	}
	if *versionFlag {
		fmt.Println("gostgrator-pg version:", version)
		os.Exit(0)
	}

	if *connStr == "" {
		fmt.Fprintln(os.Stderr, "Error: connection URL (-conn) is required")
		usage()
		os.Exit(1)
	}

	// Create a default configuration.
	cliConfig := gostgrator.Config{
		Driver:           "pg",
		SchemaTable:      *schemaTable,
		MigrationPattern: *migrationPattern,
	}

	// Optionally load configuration from a file.
	if *configPath != "" {
		if err := loadConfig(*configPath, &cliConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config file: %v\n", err)
			os.Exit(1)
		}
	}

	// Open the database connection.
	db, err := sql.Open("pg", *connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create a gostgrator instance.
	g, err := gostgrator.NewGostgrator(cliConfig, db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing gostgrator: %v\n", err)
		os.Exit(1)
	}

	// Create a context with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Determine command.
	args := flag.Args()
	command := "migrate"
	if len(args) > 0 {
		if args[0] == "drop-schema" {
			command = "drop-schema"
		} else if args[0] != "migrate" {
			// If the argument is a target version (e.g., a number or "max").
			command = "migrate"
			*target = args[0]
		}
	}

	// Execute command.
	switch command {
	case "migrate":
		fmt.Printf("[%s] Starting migration to version %s...\n", time.Now().Format(time.Kitchen), *target)
		applied, err := g.Migrate(ctx, *target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Migration error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[%s] Applied %d migrations:\n", time.Now().Format(time.Kitchen), len(applied))
		for _, m := range applied {
			fmt.Printf("  - Version %d: %s (%s)\n", m.Version, m.Name, m.Filename)
		}
	case "drop-schema":
		fmt.Printf("[%s] Dropping schema table...\n", time.Now().Format(time.Kitchen))
		if err := dropSchema(ctx, cliConfig, g); err != nil {
			fmt.Fprintf(os.Stderr, "Error dropping schema table: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[%s] Schema table dropped.\n", time.Now().Format(time.Kitchen))
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		usage()
		os.Exit(1)
	}
}

// loadConfig loads a JSON configuration file into cfg.
func loadConfig(path string, cfg *gostgrator.Config) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(cfg)
}

// dropSchema drops the schema version table.
func dropSchema(ctx context.Context, cfg gostgrator.Config, g *gostgrator.Gostgrator) error {
	var table string
	if strings.Contains(cfg.SchemaTable, ".") {
		parts := strings.Split(cfg.SchemaTable, ".")
		table = fmt.Sprintf(`"%s"."%s"`, parts[0], parts[1])
	} else {
		table = fmt.Sprintf(`"%s"`, cfg.SchemaTable)
	}
	query := fmt.Sprintf("DROP TABLE %s", table)
	_, err := g.RunQuery(ctx, query)
	return err
}
