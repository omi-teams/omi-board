package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schemaPool returns a pool whose search_path is a fresh schema, dropped on cleanup.
func schemaPool(t *testing.T, ctx context.Context, schema string) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}
	admin, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func count(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMigrateFreshAndRerun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := schemaPool(t, ctx, "mig_fresh")
	fsys := fstest.MapFS{
		"0001_a.sql": {Data: []byte("CREATE TABLE a (id int);")},
		"0002_b.sql": {Data: []byte("CREATE TABLE b (id int);")},
	}
	got, err := Migrate(ctx, pool, fsys)
	if err != nil || len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("first run = %v, %v", got, err)
	}
	if n := count(t, ctx, pool, "schema_migrations"); n != 2 {
		t.Fatalf("recorded %d, want 2", n)
	}
	got, err = Migrate(ctx, pool, fsys)
	if err != nil || len(got) != 0 {
		t.Fatalf("second run = %v, %v; want no-op", got, err)
	}
}

func TestMigrateConcurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p1 := schemaPool(t, ctx, "mig_conc")
	cfg := p1.Config()
	p2, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()
	fsys := fstest.MapFS{
		"0001_probe.sql":  {Data: []byte("CREATE TABLE probe (id int); SELECT pg_sleep(0.3);")},
		"0002_insert.sql": {Data: []byte("INSERT INTO probe VALUES (1);")},
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	applied := make([]int, 2)
	for i, p := range []*pgxpool.Pool{p1, p2} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var got []int64
			got, errs[i] = Migrate(ctx, p, fsys)
			applied[i] = len(got)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if applied[0]+applied[1] != 2 {
		t.Fatalf("applied %v, want 2 in total", applied)
	}
	if n := count(t, ctx, p1, "probe"); n != 1 {
		t.Fatalf("probe rows = %d, want 1", n)
	}
	if n := count(t, ctx, p1, "schema_migrations"); n != 2 {
		t.Fatalf("recorded %d, want 2", n)
	}
}

func TestMigrateRefusals(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := schemaPool(t, ctx, "mig_refuse")
	fsys := fstest.MapFS{"0002_a.sql": {Data: []byte("CREATE TABLE a (id int);")}}
	if _, err := Migrate(ctx, pool, fsys); err != nil {
		t.Fatal(err)
	}

	// Changed file.
	changed := fstest.MapFS{"0002_a.sql": {Data: []byte("CREATE TABLE a (id bigint);")}}
	if _, err := Migrate(ctx, pool, changed); !errors.Is(err, ErrChecksum) {
		t.Fatalf("changed file: err = %v, want ErrChecksum", err)
	}

	// Pending version below the highest applied one.
	older := fstest.MapFS{
		"0001_x.sql": {Data: []byte("SELECT 1;")},
		"0002_a.sql": fsys["0002_a.sql"],
	}
	if _, err := Migrate(ctx, pool, older); !errors.Is(err, ErrOutOfOrder) {
		t.Fatalf("older pending: err = %v, want ErrOutOfOrder", err)
	}

	// Database newer than the files.
	if _, err := pool.Exec(ctx, "INSERT INTO schema_migrations (version, name, checksum) VALUES (9999, '9999_future.sql', 'x')"); err != nil {
		t.Fatal(err)
	}
	if _, err := Migrate(ctx, pool, fsys); !errors.Is(err, ErrUnknownMigration) {
		t.Fatalf("newer db: err = %v, want ErrUnknownMigration", err)
	}
}

func TestMigrateBadFiles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := schemaPool(t, ctx, "mig_bad")
	for name, fsys := range map[string]fstest.MapFS{
		"bad name":  {"one.sql": {Data: []byte("SELECT 1;")}},
		"duplicate": {"0001_a.sql": {Data: []byte("SELECT 1;")}, "00001_b.sql": {Data: []byte("SELECT 1;")}},
	} {
		if _, err := Migrate(ctx, pool, fsys); err == nil {
			t.Fatalf("%s: want error", name)
		}
	}
}

func TestMigrateNoTransaction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := schemaPool(t, ctx, "mig_notx")
	fsys := fstest.MapFS{
		"0001_t.sql":   {Data: []byte("CREATE TABLE t (id int);")},
		"0002_idx.sql": {Data: []byte(fmt.Sprintf("%s\nCREATE INDEX CONCURRENTLY t_id_idx ON t (id);", noTxMarker))},
	}
	if got, err := Migrate(ctx, pool, fsys); err != nil || len(got) != 2 {
		t.Fatalf("run = %v, %v", got, err)
	}
}
