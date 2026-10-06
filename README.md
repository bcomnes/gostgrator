![gostgrator: SQL migrations for Go.](assets/gostgrator-opengraph-migratory-goversion-style-v3.webp)

# gostgrator
[![Actions Status][action-img]][action-url]
[![SocketDev][socket-image]][socket-url]
[![PkgGoDev][pkg-go-dev-img]][pkg-go-dev-url]

[action-img]: https://github.com/bcomnes/gostgrator/actions/workflows/test.yml/badge.svg
[action-url]: https://github.com/bcomnes/gostgrator/actions/workflows/test.yml
[pkg-go-dev-img]: https://pkg.go.dev/badge/github.com/bcomnes/gostgrator
[pkg-go-dev-url]: https://pkg.go.dev/github.com/bcomnes/gostgrator
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
go get -tool github.com/bcomnes/gostgrator/pg
go tool github.com/bcomnes/gostgrator/pg -help
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
go get -tool github.com/bcomnes/gostgrator/sqlite
go tool github.com/bcomnes/gostgrator/sqlite -help
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
go tool github.com/bcomnes/gostgrator/pg migrate

# rollback the last two migrations
go tool github.com/bcomnes/gostgrator/pg down 2

# create a timestamp‑based pair
go tool github.com/bcomnes/gostgrator/pg -mode timestamp new "add-users-table"

# list all migrations and mark current
gostgrator-pg list
```


## Library usage

Full API docs live on [PkgGoDev][pkg-go-dev-url].

### Database connections and SQL dialects

To run migrations from your Go application, pass a configuration and an initialized [`*sql.DB`](https://pkg.go.dev/database/sql#DB) to `gostgrator.NewGostgrator`.
You can reuse your application's existing database connection; your application remains responsible for opening and closing it.

`Config.Driver` selects Gostgrator's SQL dialect, not the Go driver used to open the connection:

| Database | `Config.Driver` | Example driver name passed to `sql.Open` |
| --- | --- | --- |
| PostgreSQL | `"pg"` | `"pgx"` |
| SQLite | `"sqlite"` (or the legacy alias `"sqlite3"`) | `"sqlite"` with modernc SQLite |

Gostgrator uses the dialect to inspect, create, and update its migration tracking table (`schemaversion` by default, configurable through `Config.SchemaTable`).
It executes the SQL in your migration files as supplied; it does not translate that SQL between databases.
Write migrations for the database you are using.

Set `Config.Driver` explicitly to match the database behind `db`.
Go's `database/sql` API does not expose a standard SQL dialect identifier, and Gostgrator does not infer one from the connection.
An empty or unsupported value is rejected; a supported value is not checked against the actual database when constructing the instance.

#### Other drivers and custom dialects

You can supply a `*sql.DB` opened by another compatible PostgreSQL or SQLite driver while keeping `Config.Driver` set to `"pg"` or `"sqlite"`.
The driver's registered name does not need to match this setting, but it must support the SQL and execution behavior your migrations require.


To use a custom dialect or wrap database operations, implement the exported [`Client`](https://pkg.go.dev/github.com/bcomnes/gostgrator#Client) interface and pass it to `NewGostgratorWithClient`.
The client handles SQL execution and migration tracking-table operations; Gostgrator still handles loading migrations, ordering, checksum validation, and orchestration.
You can also wrap an existing built-in client rather than implementing every operation yourself.

Given an initialized `customClient` implementing `gostgrator.Client`, construct the migrator as follows:

```go
g, err := gostgrator.NewGostgratorWithClient(gostgrator.Config{
    Migrations: gostgrator.DiskMigrations{
        Pattern: "migrations/*.sql",
    },
}, customClient)
if err != nil {
    return err
}
_, err = g.Migrate(ctx, "max")
```

This constructor also supports `FSMigrations` and the legacy `MigrationPattern`.
It uses the same migration configuration validation and defaults as `NewGostgrator`, but it does not select or configure a database client.
`Config.Driver`, `Config.Conn`, and `Config.SchemaTable` do not configure or override the supplied client: configure its database and tracking table yourself before passing it in.
No supported `Config.Driver` value is required.
Nil clients, including typed nil implementations, are rejected.

Your application owns the client's database resources and remains responsible for closing them.
The `Client` interface uses `*sql.Rows` and `sql.Result`, so it remains tied to `database/sql` rather than supporting arbitrary database APIs.
A custom dialect must implement the required bookkeeping operations, and your migration SQL must still be compatible with its database.


### Choose where migrations are loaded from

Set `Config.Migrations` to one of two options:

- **`DiskMigrations`** reads SQL files from disk when migrations run.
  Use this when you deploy a migrations directory alongside your application.
- **`FSMigrations`** reads SQL files from a Go filesystem supplied in its `FS` field.
  Use this with `embed.FS` to package migrations inside your executable, without needing a migrations directory at runtime.
  See [Embed migrations in a binary](#embed-migrations-in-a-binary) below.

Both options have a `Pattern` field that selects which files to load.
For example, `migrations/*.sql` selects SQL files directly inside the `migrations` directory.
Name the files using the [migration naming convention](#migrations), such as `001.do.create-users.sql` and `001.undo.create-users.sql`.

To load migrations from disk, use the following configuration.
This example assumes `db` is an initialized SQLite `*sql.DB` and `ctx` is a `context.Context`:

```go
cfg := gostgrator.Config{
    Driver: "sqlite",
    Migrations: gostgrator.DiskMigrations{
        Pattern: "migrations/*.sql",
    },
}
g, err := gostgrator.NewGostgrator(cfg, db)
if err != nil {
    return err
}
_, err = g.Migrate(ctx, "max")
```

`Migrate(ctx, "max")` applies pending migrations up to the highest available version.
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

#### Existing configurations and CLI usage

If you already use `Config.MigrationPattern`, you can keep using it to load migrations from disk.
Set either `Migrations` or `MigrationPattern`, not both.
The CLI continues to select disk files with `-migration-pattern`; `Config.Migrations` is for applications using the Go library.

### Embed migrations in a binary

Given a `migrations` directory beside this Go source file, embed the SQL files and pass the filesystem to `FSMigrations`:

```go
package database

import (
    "context"
    "database/sql"
    "embed"

    "github.com/bcomnes/gostgrator"
    _ "modernc.org/sqlite"
)

//go:embed migrations
var migrations embed.FS

func Migrate(ctx context.Context, db *sql.DB) error {
    g, err := gostgrator.NewGostgrator(gostgrator.Config{
        Driver: "sqlite",
        Migrations: gostgrator.FSMigrations{
            FS:      migrations,
            Pattern: "migrations/*.sql",
        },
        ValidateChecksums: true,
    }, db)
    if err != nil {
        return err
    }
    _, err = g.Migrate(ctx, "max")
    return err
}
```

The example expects a SQLite connection opened with `sql.Open("sqlite", ...)`.
Embedded SQL is built into the binary and does not require migration files in the runtime working directory.
Changing embedded migrations requires rebuilding the binary.

The directory directive embeds eligible files recursively, but `migrations/*.sql` only selects migrations directly inside that directory.
Directory embedding excludes dotfiles and underscore-prefixed files by default; use `//go:embed all:migrations` if those are needed.
Embedding paths are relative to the package containing the directive and cannot escape it using `..`.

Use `g.Migrate(ctx, "0")` to apply the undo migrations back to version zero, or `g.Down(ctx, 1)` to roll back one version step.
File naming, migration ordering, duplicate version/action detection, and checksum validation work the same way for disk and filesystem sources.
`Config.Newline` normalizes content for checksum calculation without rewriting the SQL executed against the database.

Migrations returned by `GetMigrations` retain their source for later `RunMigrations` calls, even when passed to a different runner.
A manually constructed `Migration` without a retained source reads its `Filename` from disk.

### Create migration files

`CreateMigration` supports explicit `DiskMigrations` as well as the legacy `MigrationPattern`:

```go
err := gostgrator.CreateMigration(gostgrator.Config{
    Migrations: gostgrator.DiskMigrations{Pattern: "migrations/*.sql"},
}, "Add users", "int")
```

The migration directory must already exist.
Use `"timestamp"` instead of `"int"` for Unix timestamp numbering.

`CreateMigration` rejects `FSMigrations`, including filesystems backed by disk, because the `fs.FS` interface is read-only.
Create files through a disk source during development, then rebuild the binary to update embedded migrations.

---

## Why another migrator?

* **CLI ‑ first** – instant productivity; no boilerplate code required.
* **Typed Go API** – embed migrations programmatically when you need to.
* **Checksum validation** – MD5 guardrails ensure applied migrations never drift.
* **Up ⬆ / Down ⬇ parity** – every migration pair keeps rollbacks honest.
* **Zero dependencies** – a single static binary per database driver.

## License

MIT © Bret Comnes 2025

## Mascot

![gostgrator migratory gopher](assets/gostgrator-gopher-migratory-1.webp)
