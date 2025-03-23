// Package main implements a CLI for gostgrator.
// It loads configuration from a JSON file (unless disabled),
// parses command-line flags, builds a database connection,
// and runs either migrations or drops the schema table.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"           // PostgreSQL driver
	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"github.com/bcomnes/gostgrator/pkg/gostgrator"
)

// CLIConfig holds the configuration loaded from a JSON file and any extra CLI parameters.
type CLIConfig struct {
	gostgrator.Config

	// Connection parameters.
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	SSL      bool   `json:"ssl,omitempty"`
}

// version of the CLI.
const version = "1.0.0"

// usage prints a help message.
func usage() {
	helpText := `
Usage:
  gostgrator [command] [options]

Commands:
  migrate       Migrate the schema to a target version.
                If no command is provided, migrate to the latest version ("max").
                You may also specify a version number as the first argument.
  drop-schema   Drop the schema version table.

Options:
  -config string
        Path to a JSON configuration file.
  -no-config
        Disable loading configuration file.
  -driver string
        Database driver. (pg or sqlite3) (default "pg")
  -host string
        Database host. (default "localhost")
  -port int
        Database port. (default 5432)
  -database string
        Database name (or SQLite file; default depends on driver)
  -username string
        Database username.
  -password string
        Database password.
  -ssl
        Enable SSL connection (default false).
  -migration-pattern string
        Glob pattern to find migration files. (default "migrations/*.sql")
  -schema-table string
        Name of the schema table. (default "schemaversion")
  -newline string
        Newline style to use: LF, CR, or CRLF.
  -validate-checksums
        Validate checksums of applied migrations. (default true)
  -to string
        Target version to migrate to. Use "max" for the latest. (default "max")
  -help
        Show this help message.
  -version
        Print the version.
`
	fmt.Fprintln(os.Stderr, helpText)
}

func main() {
	// Define flags.
	configPath := flag.String("config", "", "Path to JSON configuration file")
	noConfig := flag.Bool("no-config", false, "Disable configuration file loading")
	driver := flag.String("driver", "pg", "Database driver: pg or sqlite3")
	host := flag.String("host", "localhost", "Database host")
	port := flag.Int("port", 5432, "Database port")
	databaseName := flag.String("database", "", "Database name (or SQLite file)")
	username := flag.String("username", "", "Database username")
	password := flag.String("password", "", "Database password")
	ssl := flag.Bool("ssl", false, "Enable SSL connection")
	migrationPattern := flag.String("migration-pattern", "migrations/*.sql", "Glob pattern for migration files")
	schemaTable := flag.String("schema-table", "schemaversion", "Name of the schema table")
	newline := flag.String("newline", "", "Newline style: LF, CR, or CRLF")
	validateChecksums := flag.Bool("validate-checksums", true, "Validate migration file checksums")
	to := flag.String("to", "max", "Target version to migrate to (number or max)")
	helpFlag := flag.Bool("help", false, "Show help")
	versionFlag := flag.Bool("version", false, "Show version")

	flag.Usage = usage
	flag.Parse()

	// Print help or version if requested.
	if *helpFlag {
		usage()
		os.Exit(0)
	}
	if *versionFlag {
		fmt.Println("gostgrator version:", version)
		os.Exit(0)
	}

	// Determine command. Use the first non-flag argument.
	args := flag.Args()
	command := "migrate"
	targetVersion := *to
	if len(args) > 0 {
		arg := strings.TrimSpace(args[0])
		if arg == "drop-schema" {
			command = "drop-schema"
		} else if arg != "migrate" {
			// If the argument is a number or "max", interpret as target version.
			if arg == "max" || isPositiveInteger(arg) {
				command = "migrate"
				targetVersion = arg
			} else {
				fmt.Fprintf(os.Stderr, "Invalid command: %s\n", arg)
				usage()
				os.Exit(1)
			}
		}
	}

	// Create a base CLIConfig from command-line flags.
	cliConfig := CLIConfig{
		Config: gostgrator.Config{
			Driver:            *driver,
			SchemaTable:       *schemaTable,
			MigrationPattern:  *migrationPattern,
			Newline:           *newline,
			ValidateChecksums: *validateChecksums,
		},
		Host:     *host,
		Port:     *port,
		Username: *username,
		Password: *password,
		SSL:      *ssl,
	}
	// Set the database name from the flag.
	cliConfig.Database = *databaseName

	// Load configuration file if not disabled.
	if !*noConfig && *configPath != "" {
		if err := loadConfig(*configPath, &cliConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config file: %v\n", err)
			os.Exit(1)
		}
	}

	// If no database name is provided, set defaults.
	if cliConfig.Database == "" {
		if strings.ToLower(cliConfig.Config.Driver) == "sqlite3" {
			cliConfig.Database = ":memory:"
		} else {
			fmt.Fprintln(os.Stderr, "Error: database name must be provided for driver", cliConfig.Config.Driver)
			os.Exit(1)
		}
	}

	// Build connection string.
	connStr := buildConnString(cliConfig)

	// Open database connection.
	db, err := sql.Open(cliConfig.Config.Driver, connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create a gostgrator instance.
	g, err := gostgrator.NewGostgrator(cliConfig.Config, db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing gostgrator: %v\n", err)
		os.Exit(1)
	}

	// Create a context with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Execute the requested command.
	switch command {
	case "migrate":
		fmt.Printf("[%s] Starting migration to version %s...\n", time.Now().Format(time.Kitchen), targetVersion)
		applied, err := g.Migrate(ctx, targetVersion)
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
		if err := dropSchema(ctx, cliConfig.Config, g); err != nil {
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

// loadConfig reads a JSON configuration file and decodes it into cfg.
func loadConfig(path string, cfg *CLIConfig) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	f, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, cfg)
}

// buildConnString builds a connection string based on the driver and CLIConfig.
func buildConnString(cfg CLIConfig) string {
	switch strings.ToLower(cfg.Config.Driver) {
	case "pg":
		// Build a PostgreSQL connection string.
		sslMode := "disable"
		if cfg.SSL {
			sslMode = "require"
		}
		// The connection string uses URL format.
		userInfo := ""
		if cfg.Username != "" {
			userInfo = cfg.Username
			if cfg.Password != "" {
				userInfo += ":" + cfg.Password
			}
			userInfo += "@"
		}
		// Example: "postgres://user:pass@host:port/dbname?sslmode=require"
		return fmt.Sprintf("postgres://%s%s:%d/%s?sslmode=%s", userInfo, cfg.Host, cfg.Port, cfg.Database, sslMode)
	case "sqlite3":
		// For SQLite, the database field is the filename.
		return cfg.Database
	default:
		return ""
	}
}

// dropSchema drops the schema version table.
func dropSchema(ctx context.Context, cfg gostgrator.Config, g *gostgrator.Gostgrator) error {
	var table string
	// For PostgreSQL, quote the table name.
	if strings.ToLower(cfg.Driver) == "pg" {
		if strings.Contains(cfg.SchemaTable, ".") {
			parts := strings.Split(cfg.SchemaTable, ".")
			table = fmt.Sprintf(`"%s"."%s"`, parts[0], parts[1])
		} else {
			table = fmt.Sprintf(`"%s"`, cfg.SchemaTable)
		}
	} else {
		// For SQLite, use as is.
		table = cfg.SchemaTable
	}
	query := fmt.Sprintf("DROP TABLE %s", table)
	_, err := g.RunQuery(ctx, query)
	return err
}

// isPositiveInteger returns true if the string represents a non-negative integer.
func isPositiveInteger(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n >= 0
}
