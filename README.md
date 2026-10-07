![gostgrator: SQL migrations for Go.](assets/gostgrator-opengraph-migratory-goversion-style-v3.webp)

# gostgrator
[![Actions Status][action-img]][action-url]
[![SocketDev][socket-image]][socket-url]
[![PkgGoDev][pkg-go-dev-img]][pkg-go-dev-url]

[action-img]: https://github.com/bcomnes/gostgrator/actions/workflows/test.yml/badge.svg
[action-url]: https://github.com/bcomnes/gostgrator/actions/workflows/test.yml
[pkg-go-dev-img]: https://pkg.go.dev/badge/github.com/bcomnes/gostgrator/v2
[pkg-go-dev-url]: https://pkg.go.dev/github.com/bcomnes/gostgrator/v2
[socket-image]: https://socket.dev/api/badge/go/package/github.com/bcomnes/gostgrator?version=v1.0.2
[socket-url]: https://socket.dev/go/package/github.com/bcomnes/gostgrator?version=v1.0.2

**gostgrator**: A low dependency, go stdlib port of [postgrator](https://github.com/rickbergfalk/postgrator) supporting postgres and sqlite.

## Install with Homebrew

Install both database-specific commands from the [`bcomnes/tap`](https://github.com/bcomnes/homebrew-tap) tap:

```console
brew install bcomnes/tap/gostgrator
gostgrator-pg -help
gostgrator-sqlite -help
```

This adds the tap automatically.
Update both commands later with `brew upgrade gostgrator`.

## Migrations

Migrations can live in any folder in your project. The default is `./migrations`.
Migration files are named `001.do.some-optional-description.sql` and `001.undo.some-optional-description.sql` and come in up and down pairs.
The files should contain SQL appropriate for the database you are running them

```console
./migrations
├── 001.do.sql
├── 001.undo.sql
├── 002.do.some-description.sql
├── 002.undo.some-description.sql
├── 003.do.sql
├── 003.undo.sql
├── 004.do.sql
├── 004.undo.sql
├── 005.do.sql
├── 005.undo.sql
├── 006.do.sql
└── 006.undo.sql
```

### Migration Transactions

gostgrator (like postgrator), applies no special or magic transaction around your migrations, other than running multiple statements from a file in one execution which postgres will treat as a transaction. If you need stricter behavior than this, or are migrating databases that don't have this behavior, wrap your migrations in explicite BEGIN/END blocks.

## gostgrator CLI

gostgrator is intended to be installed and versioned as a [go tool](https://go.dev/doc/go1.24#go-command).

Each supported database has it's own CLI you can install.

### gostgrator/pg

The `gostgrator/pg` cli provides migration support for [Postgres](https://www.postgresql.org).

```console
go get -tool github.com/bcomnes/gostgrator/v2/pg
go tool github.com/bcomnes/gostgrator/v2/pg -help
Usage:
  gostgrator-pg [command] [arguments] [options]

Commands:
  migrate [target]    Migrate the schema to a target version (default: "max").
  down [steps]        Roll back the specified number of migrations (default: 1).
  new <desc>          Create a new empty migration pair with the provided description.
  drop-schema         Drop the schema version table.
  list                List available migrations and annotate the migration matching the database version.

Options:
  -config string
    	Path to JSON configuration file (optional)
  -conn string
    	PostgreSQL connection URL. Can be set with DATABASE_URL env var.
  -help
    	Show help message
  -migration-pattern string
    	Glob pattern for migration files when running up or down migrations (default "migrations/*.sql")
  -mode string
    	Migration numbering mode ("int" or "timestamp") when creating new migrations (default "int")
  -schema-table string
    	Name of the schema table migration state is stored in (default "schemaversion")
  -version
    	Show version
```

### gostgrator/sqlite

```console
go get -tool github.com/bcomnes/gostgrator/v2/sqlite
go tool github.com/bcomnes/gostgrator/v2/sqlite -help
Usage:
  gostgrator-sqlite [command] [arguments] [options]

Commands:
  migrate [target]    Migrate the schema to a target version (default: "max").
  down [steps]        Roll back the specified number of migrations (default: 1).
  new <desc>          Create a new empty migration pair with the provided description.
  drop-schema         Drop the schema version table.
  list                List available migrations and annotate the migration matching the database version.

Options:
  -config string
    	Path to JSON configuration file (optional)
  -conn string
    	SQLite connection URL (typically a file path, e.g., "./db.sqlite"). Can also be set via SQLITE_URL env var.
  -help
    	Show help message
  -migration-pattern string
    	Glob pattern for migration files (default "migrations/*.sql")
  -mode string
    	Migration numbering mode ("int" or "timestamp") for new command (default "int")
  -schema-table string
    	Name of the schema table (default "schemaversion")
  -version
    	Show version
```

## Quick tour

```console
# migrate to latest in ./migrations using DATABASE_URL
go tool github.com/bcomnes/gostgrator/v2/pg migrate

# rollback the last two migrations
go tool github.com/bcomnes/gostgrator/v2/pg down 2

# create a timestamp‑based pair
go tool github.com/bcomnes/gostgrator/v2/pg -mode timestamp new "add-users-table"

# list all migrations and mark current
gostgrator-pg list
```


## Library usage

Full API docs live on [PkgGoDev][pkg-go-dev-url].

### Database connections and SQL dialects

Use your existing [`*sql.DB`](https://pkg.go.dev/database/sql#DB) with the matching client:

| Database | Client constructor | Example driver name passed to `sql.Open` |
| --- | --- | --- |
| PostgreSQL | `NewPostgresClient` | `"pgx"` |
| SQLite | `NewSqlite3Client` | `"sqlite"` with modernc SQLite |

```go
client := gostgrator.NewPostgresClient(gostgrator.Config{
    SchemaTable: "app_migrations", // Optional; defaults to "schemaversion".
}, db)

g, err := gostgrator.NewGostgrator(gostgrator.Config{
    Migrations: gostgrator.DiskMigrations{Pattern: "migrations/*.sql"},
}, client)
if err != nil {
    return err
}
_, err = g.Migrate(ctx, "max")
```

The example assumes a PostgreSQL `db` and a `context.Context` named `ctx`.
Your application owns and closes the database connection.

The client chooses the bookkeeping dialect; migration SQL runs unchanged.
Set `SchemaTable` on the client and `Migrations` on the migrator.

### Choose where migrations are loaded from

Set `Config.Migrations` to either:

- **`DiskMigrations{Pattern: "migrations/*.sql"}`** for SQL files deployed alongside your application.
- **`FSMigrations{FS: migrations, Pattern: "migrations/*.sql"}`** for a supplied Go filesystem, including [embedded migrations](#embed-migrations-in-a-binary).

The pattern selects files; their [names](#migrations) determine version and action:

```text
migrations/
├── 001.do.create-users.sql
├── 001.undo.create-users.sql
├── 002.do.add-email.sql
└── 002.undo.add-email.sql
```

`Migrate(ctx, "max")` applies pending migrations up to the highest available version.
Use `Migrate(ctx, "0")` to undo all migrations or `Down(ctx, 1)` to roll back one version step.
For disk migrations, relative paths are resolved from the application's working directory; absolute paths are also supported.

#### Filesystem paths and configuration

`FSMigrations` accepts an [`fs.FS`](https://pkg.go.dev/io/fs#FS), Go's standard read-only filesystem interface.
Implementations include [`embed.FS`](https://pkg.go.dev/embed#FS) for files compiled into your binary, [`os.DirFS`](https://pkg.go.dev/os#DirFS) for a directory on disk, and [`fstest.MapFS`](https://pkg.go.dev/testing/fstest#MapFS) for in-memory test files.

Unlike disk paths, `FSMigrations.Pattern` is relative to the supplied filesystem's root and uses forward slashes, with no leading slash or `.` or `..` path components.
Use `migrations/*.sql`, not `./migrations/*.sql`.
To make the migrations directory itself the root, use [`fs.Sub`](https://pkg.go.dev/io/fs#Sub) and then select files with `*.sql`.

Patterns follow [`filepath.Glob`](https://pkg.go.dev/path/filepath#Glob) for `DiskMigrations` and [`fs.Glob`](https://pkg.go.dev/io/fs#Glob) for `FSMigrations`.
Neither treats `**` as a recursive wildcard.
Both options require a nonempty pattern, and `FSMigrations` also requires a nonnil filesystem.
`Config.Migrations` accepts either an option value or a nonnil pointer to one; its interface type is `MigrationSource`.

Both sources use the same naming, ordering, duplicate detection, and checksum validation.
`Config.Newline` normalizes checksums without changing the SQL executed.
Migrations returned by `GetMigrations` retain their source for `RunMigrations`, even with a different runner; manually constructed migrations read `Filename` from disk.


### Embed migrations in a binary

Keep the SQL files beside the Go file that embeds them:

```text
myapp/
├── go.mod
├── cmd/
│   └── myapp/
│       └── main.go
└── database/
    ├── migrations.go
    └── migrations/
        ├── 001.do.create-users.sql
        ├── 001.undo.create-users.sql
        ├── 002.do.add-email.sql
        └── 002.undo.add-email.sql
```

`database/migrations.go` — embed paths are relative to this file's directory:

```go
package database

import (
    "context"
    "database/sql"
    "embed"

    "github.com/bcomnes/gostgrator/v2"
    _ "modernc.org/sqlite"
)

//go:embed migrations
var migrations embed.FS

func Migrate(ctx context.Context, db *sql.DB) error {
    client := gostgrator.NewSqlite3Client(gostgrator.Config{}, db)
    g, err := gostgrator.NewGostgrator(gostgrator.Config{
        Migrations: gostgrator.FSMigrations{
            FS:      migrations,
            Pattern: "migrations/*.sql",
        },
        ValidateChecksums: true,
    }, client)
    if err != nil {
        return err
    }
    _, err = g.Migrate(ctx, "max")
    return err
}
```

Pass a SQLite `*sql.DB` opened with `sql.Open("sqlite", ...)`.
Deploy the binary without the SQL directory; rebuild it when migrations change.

`//go:embed migrations` includes eligible files recursively; `migrations/*.sql` selects only the directory's immediate SQL files.
See Go's [`embed` documentation](https://pkg.go.dev/embed#hdr-Directives) for path restrictions and including dotfiles with `all:`.

### Create migration files

Create an empty `migrations/` directory, then generate a do/undo pair:

```go
err := gostgrator.CreateMigration(gostgrator.Config{
    Migrations: gostgrator.DiskMigrations{Pattern: "migrations/*.sql"},
}, "Add users", "int")
```

```text
migrations/
├── 001.do.add-users.sql
└── 001.undo.add-users.sql
```

Subsequent calls increment the version; use `"timestamp"` for Unix timestamp numbering.
Use `DiskMigrations` for file creation; `FSMigrations` is read-only.
For embedded migrations, edit these files and rebuild.

### Advanced: custom clients and dialects

Use a custom [`Client`](https://pkg.go.dev/github.com/bcomnes/gostgrator/v2#Client) only when you need an escape hatch for another dialect or custom database behavior.
Switching to a compatible PostgreSQL or SQLite driver does not require one.

This wrapper adds execution logging to the built-in SQLite client:

```go
package database

import (
    "context"
    "database/sql"
    "log"

    "github.com/bcomnes/gostgrator/v2"
)

type loggingClient struct {
    gostgrator.Client
    logger *log.Logger
}

var _ gostgrator.Client = (*loggingClient)(nil)

func (c *loggingClient) ExecContext(ctx context.Context, script string) (sql.Result, error) {
    result, err := c.Client.ExecContext(ctx, script)
    c.logger.Printf("SQL execution succeeded: %t", err == nil)
    return result, err
}

func Migrate(ctx context.Context, db *sql.DB, logger *log.Logger) error {
    client := &loggingClient{
        Client: gostgrator.NewSqlite3Client(gostgrator.Config{
            SchemaTable: "app_migrations",
        }, db),
        logger: logger,
    }
    g, err := gostgrator.NewGostgrator(gostgrator.Config{
        Migrations: gostgrator.DiskMigrations{
            Pattern: "migrations/*.sql",
        },
    }, client)
    if err != nil {
        return err
    }
    _, err = g.Migrate(ctx, "max")
    return err
}
```

Pass a SQLite `*sql.DB` and a nonnil logger such as `log.Default()`.
The wrapper logs migration and bookkeeping executions, not SQL text, queries, or calls made internally by the wrapped client's `EnsureTable`.

For a new dialect, implement all [`Client`](https://pkg.go.dev/github.com/bcomnes/gostgrator/v2#Client) methods: SQL execution, tracking-table management, version/checksum queries, and action persistence.
The interface uses `*sql.Rows` and `sql.Result`, so implementations remain tied to `database/sql`.

Configure and manage the client's resources yourself; `NewGostgrator` does not override them with `Config.Driver`, `Conn`, or `SchemaTable`.
Nil clients, including typed nil implementations, are rejected.

---

## Why another migrator?

* **CLI ‑ first** – instant productivity; no boilerplate code required.
* **Typed Go API** – run migrations directly from your Go application.
* **Embedded migrations** – ship SQL files inside your binary with `go:embed`; no runtime migrations directory needed.
* **Custom adapter clients** – implement the [`Client`](https://pkg.go.dev/github.com/bcomnes/gostgrator/v2#Client) interface to support another SQL dialect or wrap database operations.
* **Checksum validation** – MD5 guardrails ensure applied migrations never drift.
* **Up ⬆ / Down ⬇ parity** – every migration pair keeps rollbacks honest.
* **Zero dependencies** – a single static binary per database driver.

## License

MIT © Bret Comnes 2025

## Mascot

![gostgrator migratory gopher](assets/gostgrator-gopher-migratory-1.webp)
