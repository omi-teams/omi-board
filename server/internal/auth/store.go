package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionTTL is the fixed lifetime of a session and of its cookie.
const SessionTTL = 14 * 24 * time.Hour

// Errors returned by Store.
var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrNoSession          = errors.New("no valid session")
	ErrEmailTaken         = errors.New("email already registered")
)

// User is the public part of a user row.
type User struct {
	ID    int64
	Email string
}

// Session is a valid, unexpired session and its user.
type Session struct {
	ID   int64
	User User
}

// Store reads and writes users and sessions.
type Store struct{ DB *pgxpool.Pool }

// CreateUser adds a user; it is only reachable from the create-user command.
func (s Store) CreateUser(ctx context.Context, email, password string) (User, error) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return User{}, errors.New("email and password are required")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	u := User{Email: email}
	err = s.DB.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`, email, hash).Scan(&u.ID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return User{}, ErrEmailTaken
	}
	return u, err
}

// Login checks the credentials and starts a session; it returns the cookie token.
// An unknown email costs the same argon2id run as a wrong password.
func (s Store) Login(ctx context.Context, email, password string) (string, Session, error) {
	var u User
	var hash string
	err := s.DB.QueryRow(ctx,
		`SELECT id, email, password_hash FROM users WHERE lower(email) = lower($1)`, strings.TrimSpace(email)).
		Scan(&u.ID, &u.Email, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		VerifyPassword(password, dummyHash())
		return "", Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return "", Session{}, err
	}
	if !VerifyPassword(password, hash) {
		return "", Session{}, ErrInvalidCredentials
	}
	token, tokenHash, err := NewToken()
	if err != nil {
		return "", Session{}, err
	}
	// Drop this user's expired sessions so the table does not grow without bound.
	if _, err := s.DB.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND expires_at <= now()`, u.ID); err != nil {
		return "", Session{}, err
	}
	sess := Session{User: u}
	err = s.DB.QueryRow(ctx,
		`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, now() + make_interval(secs => $3)) RETURNING id`,
		u.ID, tokenHash, SessionTTL.Seconds()).Scan(&sess.ID)
	if err != nil {
		return "", Session{}, err
	}
	return token, sess, nil
}

// Session returns the unexpired session for a cookie token, or ErrNoSession.
func (s Store) Session(ctx context.Context, token string) (Session, error) {
	var sess Session
	err := s.DB.QueryRow(ctx,
		`SELECT s.id, u.id, u.email FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > now()`, HashToken(token)).
		Scan(&sess.ID, &sess.User.ID, &sess.User.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNoSession
	}
	return sess, err
}

// DeleteSession revokes a session.
func (s Store) DeleteSession(ctx context.Context, id int64) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	return err
}
