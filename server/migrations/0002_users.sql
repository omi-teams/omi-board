-- Users log in with email and password; rows are added only by the create-user command.
CREATE TABLE users (
	id bigint GENERATED ALWAYS AS IDENTITY,
	email text NOT NULL,
	password_hash text NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT users_pkey PRIMARY KEY (id)
);

CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));
