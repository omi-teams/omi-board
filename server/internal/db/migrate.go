package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned by Migrate when the database and the files disagree.
var (
	ErrUnknownMigration = errors.New("database has a migration newer than this version knows")
	ErrChecksum         = errors.New("applied migration file changed")
	ErrOutOfOrder       = errors.New("pending migration is older than an applied one")
)

// migrateLockKey is the advisory lock key shared by all runners ("omimigr").
const migrateLockKey int64 = 0x6f6d696d696772

const noTxMarker = "-- migrate:no-transaction"

var fileName = regexp.MustCompile(`^(\d{4,})_[a-z0-9_]+\.sql$`)

type migration struct {
	version  int64
	name     string
	sql      string
	checksum string
}

// Migrate applies the pending NNNN_name.sql files of fsys in order and returns
// the versions it applied. Runners are serialised by an advisory lock held on a
// dedicated connection, so concurrent callers apply each file exactly once.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) ([]int64, error) {
	files, err := loadMigrations(fsys)
	if err != nil {
		return nil, err
	}

	// Own connection: closing it always releases the session lock.
	conn, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig.Copy())
	if err != nil {
		return nil, fmt.Errorf("migrate: connect: %w", err)
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrateLockKey); err != nil {
		return nil, fmt.Errorf("migrate: lock: %w", err)
	}
	// After the lock, so it cannot cancel the wait for another runner.
	if _, err := conn.Exec(ctx, "SET lock_timeout = '10s'"); err != nil {
		return nil, fmt.Errorf("migrate: set lock_timeout: %w", err)
	}
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version bigint PRIMARY KEY,
		name text NOT NULL,
		checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, fmt.Errorf("migrate: create schema_migrations: %w", err)
	}

	applied, err := appliedChecksums(ctx, conn)
	if err != nil {
		return nil, err
	}
	known := make(map[int64]migration, len(files))
	for _, m := range files {
		known[m.version] = m
	}
	var maxApplied int64
	for v, sum := range applied {
		m, ok := known[v]
		if !ok {
			return nil, fmt.Errorf("%w: version %d", ErrUnknownMigration, v)
		}
		if m.checksum != sum {
			return nil, fmt.Errorf("%w: %s", ErrChecksum, m.name)
		}
		maxApplied = max(maxApplied, v)
	}

	var done []int64
	for _, m := range files {
		if _, ok := applied[m.version]; ok {
			continue
		}
		if m.version < maxApplied {
			return nil, fmt.Errorf("%w: %s", ErrOutOfOrder, m.name)
		}
		if err := apply(ctx, conn, m); err != nil {
			return done, fmt.Errorf("migrate: apply %s: %w", m.name, err)
		}
		done = append(done, m.version)
	}
	return done, nil
}

func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: read files: %w", err)
	}
	var files []migration
	seen := map[int64]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		g := fileName.FindStringSubmatch(e.Name())
		if g == nil {
			return nil, fmt.Errorf("migrate: bad file name %q (want NNNN_name.sql)", e.Name())
		}
		v, err := strconv.ParseInt(g[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("migrate: bad version in %q: %w", e.Name(), err)
		}
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("migrate: duplicate version %d: %s and %s", v, prev, e.Name())
		}
		seen[v] = e.Name()
		body, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", e.Name(), err)
		}
		sum := sha256.Sum256(body)
		files = append(files, migration{v, e.Name(), string(body), hex.EncodeToString(sum[:])})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].version < files[j].version })
	return files, nil
}

func appliedChecksums(ctx context.Context, conn *pgx.Conn) (map[int64]string, error) {
	rows, err := conn.Query(ctx, "SELECT version, checksum FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("migrate: read schema_migrations: %w", err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var v int64
		var sum string
		if err := rows.Scan(&v, &sum); err != nil {
			return nil, fmt.Errorf("migrate: scan schema_migrations: %w", err)
		}
		out[v] = sum
	}
	return out, rows.Err()
}

const insertApplied = "INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)"

func apply(ctx context.Context, conn *pgx.Conn, m migration) error {
	if strings.HasPrefix(m.sql, noTxMarker) {
		// One statement, run outside a transaction (e.g. CREATE INDEX CONCURRENTLY).
		// ponytail: no tx means a crash between the two statements re-runs the file; keep it idempotent.
		if _, err := conn.Exec(ctx, m.sql, pgx.QueryExecModeSimpleProtocol); err != nil {
			return err
		}
		_, err := conn.Exec(ctx, insertApplied, m.version, m.name, m.checksum)
		return err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, m.sql, pgx.QueryExecModeSimpleProtocol); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, insertApplied, m.version, m.name, m.checksum); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
