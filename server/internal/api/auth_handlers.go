package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/omi-teams/omi-board/server/internal/auth"
)

// loginFailed is the one message for every login failure, so it does not reveal which emails exist.
const loginFailed = "Invalid email or password"

func problem(status int, title, code string) Problem {
	return Problem{Type: "about:blank", Title: title, Status: status, Code: code}
}

// ErrorHandlers answers malformed requests and handler errors with Problem bodies
// that never include the underlying error text.
func ErrorHandlers() StrictHTTPServerOptions {
	write := func(w http.ResponseWriter, p Problem) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(p.Status)
		_ = json.NewEncoder(w).Encode(p)
	}
	return StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			write(w, problem(http.StatusBadRequest, "Bad request", "bad_request"))
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			slog.ErrorContext(r.Context(), "request handler failed")
			write(w, problem(http.StatusInternalServerError, "Internal error", "internal_error"))
		},
	}
}

// sessionCookie builds the session cookie; maxAge < 0 clears it.
func (s Server) sessionCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: auth.CookieName, Value: value, Path: "/", Domain: s.CookieDomain, MaxAge: maxAge,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	}
}

type loginOK struct {
	body   PostV1AuthLogin200JSONResponse
	cookie *http.Cookie
}

func (r loginOK) VisitPostV1AuthLoginResponse(w http.ResponseWriter) error {
	http.SetCookie(w, r.cookie)
	return r.body.VisitPostV1AuthLoginResponse(w)
}

type logoutOK struct{ cookie *http.Cookie }

func (r logoutOK) VisitPostV1AuthLogoutResponse(w http.ResponseWriter) error {
	http.SetCookie(w, r.cookie)
	return PostV1AuthLogout204Response{}.VisitPostV1AuthLogoutResponse(w)
}

func (s Server) session(sess auth.Session) Session {
	return Session{User: User{Id: int(sess.User.ID), Email: sess.User.Email}, CsrfToken: s.Auth.CSRFToken(sess.ID)}
}

func (s Server) PostV1AuthLogin(ctx context.Context, req PostV1AuthLoginRequestObject) (PostV1AuthLoginResponseObject, error) {
	var body LoginRequest
	if req.Body != nil {
		body = *req.Body
	}
	token, sess, err := s.Auth.Store.Login(ctx, body.Email, body.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		return PostV1AuthLogin401ApplicationProblemPlusJSONResponse(problem(401, loginFailed, "invalid_credentials")), nil
	}
	if err != nil {
		return nil, err
	}
	return loginOK{PostV1AuthLogin200JSONResponse(s.session(sess)), s.sessionCookie(token, int(auth.SessionTTL/time.Second))}, nil
}

func (s Server) PostV1AuthLogout(ctx context.Context, _ PostV1AuthLogoutRequestObject) (PostV1AuthLogoutResponseObject, error) {
	sess, ok := auth.FromContext(ctx)
	if !ok {
		return PostV1AuthLogout401ApplicationProblemPlusJSONResponse(problem(401, "Not logged in", "unauthenticated")), nil
	}
	if err := s.Auth.Store.DeleteSession(ctx, sess.ID); err != nil {
		return nil, err
	}
	return logoutOK{s.sessionCookie("", -1)}, nil
}

func (s Server) GetV1AuthSession(ctx context.Context, _ GetV1AuthSessionRequestObject) (GetV1AuthSessionResponseObject, error) {
	sess, ok := auth.FromContext(ctx)
	if !ok {
		return GetV1AuthSession401ApplicationProblemPlusJSONResponse(problem(401, "Not logged in", "unauthenticated")), nil
	}
	return GetV1AuthSession200JSONResponse(s.session(sess)), nil
}
