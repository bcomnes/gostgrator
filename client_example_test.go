package gostgrator_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"testing/fstest"

	"github.com/bcomnes/gostgrator/v2"
	_ "modernc.org/sqlite"
)

type loggingClient struct {
	gostgrator.Client
	logger *log.Logger
}

var _ gostgrator.Client = (*loggingClient)(nil)

func (c *loggingClient) ExecContext(ctx context.Context, script string, args ...any) (sql.Result, error) {
	result, err := c.Client.ExecContext(ctx, script, args...)
	c.logger.Printf("SQL execution succeeded: %t", err == nil)
	return result, err
}

func ExampleNewGostgrator_customClient() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	client := &loggingClient{
		Client: gostgrator.NewSqlite3Client(gostgrator.Config{
			SchemaTable: "app_migrations",
		}, db),
		logger: log.New(os.Stdout, "", 0),
	}
	// Use an in-memory filesystem to make the example self-contained.
	// DiskMigrations or an embed.FS can be used with the same client.
	migrations := fstest.MapFS{
		"001.do.create-items.sql": {Data: []byte("CREATE TABLE items (id INTEGER PRIMARY KEY);")},
	}
	g, err := gostgrator.NewGostgrator(gostgrator.Config{
		Migrations: gostgrator.FSMigrations{FS: migrations, Pattern: "*.sql"},
	}, client)
	if err != nil {
		panic(err)
	}
	applied, err := g.Migrate(context.Background(), "max")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Applied %d migration\n", len(applied))

	// Output:
	// SQL execution succeeded: true
	// SQL execution succeeded: true
	// Applied 1 migration
}
