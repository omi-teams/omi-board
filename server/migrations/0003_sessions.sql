-- Server-side sessions; token_hash is the sha256 of the cookie value, logout deletes the row.
CREATE TABLE sessions (
	id bigint GENERATED ALWAYS AS IDENTITY,
	user_id bigint NOT NULL,
	token_hash bytea NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now(),
	expires_at timestamptz NOT NULL,
	CONSTRAINT sessions_pkey PRIMARY KEY (id),
	CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
	CONSTRAINT sessions_token_hash_key UNIQUE (token_hash)
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
