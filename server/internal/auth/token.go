package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
)

const tokenBytes = 32

// NewToken returns a random session token (the cookie value) and its sha256, the only form stored.
func NewToken() (token string, hash []byte, err error) {
	raw := make([]byte, tokenBytes)
	if _, err = rand.Read(raw); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken returns the sha256 of a session token.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// csrfToken derives the CSRF token for a session: HMAC-SHA256(secret, "csrf:"+session id), hex.
func csrfToken(secret []byte, sessionID int64) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("csrf:" + strconv.FormatInt(sessionID, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}
