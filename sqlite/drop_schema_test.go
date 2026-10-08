package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/bcomnes/gostgrator/v2"
)

type dropSchemaClient struct {
	gostgrator.Client
	query string
	args  []any
	err   error
}

func (c *dropSchemaClient) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	c.query, c.args = query, args
	return nil, c.err
}

func TestDropSchemaQuotesIdentifiers(t *testing.T) {
	sentinel := errors.New("driver error")
	for _, driverErr := range []error{nil, sentinel} {
		client := &dropSchemaClient{err: driverErr}
		cfg := gostgrator.Config{SchemaTable: `schema '"; --.versions '"; --`}
		g, err := gostgrator.NewGostgrator(cfg, client)
		if err != nil {
			t.Fatal(err)
		}
		if err := dropSchema(context.Background(), cfg, g); !errors.Is(err, driverErr) {
			t.Fatalf("drop error = %v, want %v", err, driverErr)
		}
		want := `DROP TABLE "schema '""; --.versions '""; --"`
		if client.query != want || len(client.args) != 0 {
			t.Fatalf("drop query = %q, args = %v; want %q without args", client.query, client.args, want)
		}
	}
}
