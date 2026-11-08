# Migrations

Numbered SQL files, applied in order when the server starts and by the
`migrate` subcommand. Applied versions are recorded in `schema_migrations`
(version, name, checksum, applied_at).

## Running

- The server runs all pending migrations after connecting and before it
  listens. Any error stops the start.
- `board migrate` is the server binary's subcommand: `/server migrate` in the
  image, or `go run ./cmd/server migrate` from `server/`. It reads only
  `DATABASE_URL`. A second run is a no-op.
- A Postgres advisory lock (held on its own connection) serialises runners, so
  replicas starting together apply each migration exactly once.

## Files

- Name: `NNNN_short_name.sql` (four or more digits, unique, ascending).
- Forward-only, one change per file. Never edit an applied file: the checksum
  is verified and a changed file stops the start. Fix mistakes with a new file.
- Each file runs in one transaction. A file whose first line is
  `-- migrate:no-transaction` runs outside one and must hold a single
  statement (for example `CREATE INDEX CONCURRENTLY`).
- A pending version lower than the highest applied one is refused (out of
  order). The server also refuses to start if the database holds a version it
  has no file for.

## Expand / contract

During a deploy the old and the new server version run side by side against
the same database, so every migration must work for both:

1. **Expand** (release N): add what the new code needs without breaking the old
   code: new tables, new nullable columns, new indexes (`CONCURRENTLY`).
   Add columns nullable, backfill in batches, then add the constraint with
   `NOT VALID` followed by `VALIDATE CONSTRAINT`. Set a `lock_timeout`
   for statements that need strong locks.
2. **Deploy** the new code, which uses the new schema. Wait until no old
   replica runs.
3. **Contract** (release N+1): drop the columns, tables or constraints the old
   code used, in a later migration.

Never rename or drop in the same release that stops using it.

## Rollback

Migrations are not reverted. An old replica restarted after a newer migration
ran refuses to start (unknown migration). Roll back by redeploying the newer
binary or shipping a new forward migration, not by reverting the schema.
