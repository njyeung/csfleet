package api

import (
	"context"
	"csfleet/orchestrator/database"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "csfleet_session"
	tokenTTL      = 24 * time.Hour
	bcryptCost    = 12
	minPassLen    = 8
)

type ctxKey int

const userCtxKey ctxKey = 0

// HashPassword bcrypt-hashes a plaintext password. Used by the admin reconcile in
// main.go and the user-management handlers.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	return string(b), err
}

// signToken issues an HS256 JWT for username, valid for tokenTTL.
func (s *Server) signToken(username string) (string, time.Time, error) {
	exp := time.Now().Add(tokenTTL)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   username,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(exp),
	})
	signed, err := tok.SignedString([]byte(s.cfg.JWTSecret))
	return signed, exp, err
}

// parseToken verifies signature + expiry and returns the subject (username).
func (s *Server) parseToken(raw string) (string, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil {
		return "", err
	}
	return claims.Subject, nil
}

// requireAuth gates a handler on a valid session cookie, stashing the username in
// the request context.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		username, err := s.parseToken(c.Value)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, username)
		next(w, r.WithContext(ctx))
	}
}

// currentUser pulls the authenticated username stashed by requireAuth.
func currentUser(r *http.Request) string {
	u, _ := r.Context().Value(userCtxKey).(string)
	return u
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		Expires:  exp,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// POST /api/auth/login {username, password}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}

	username := strings.ToLower(strings.TrimSpace(body.Username))

	// The seed admin is checked in memory; everyone else against the DB.
	hash := ""
	if username == s.cfg.AdminUser {
		hash = s.cfg.AdminPassHash
	} else if h, err := s.store.UserHash(username); err == nil {
		hash = h
	}
	if hash == "" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)) != nil {
		// Generic failure: never reveal whether the username exists.
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, exp, err := s.signToken(username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	s.setSessionCookie(w, r, token, exp)
	writeJSON(w, http.StatusOK, map[string]string{"username": username})
}

// POST /api/auth/logout
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/auth/me
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"username": currentUser(r)})
}

// --- User management (/api/users) ---

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The seed admin isn't in the DB; surface it first, flagged non-editable.
	out := make([]userResponse, 0, len(users)+1)
	out = append(out, userResponse{Username: s.cfg.AdminUser, Seed: true})
	for _, u := range users {
		out = append(out, userResponse{Username: u.Username, CreatedAt: u.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	username := strings.ToLower(strings.TrimSpace(body.Username))
	if username == "" {
		writeErr(w, http.StatusBadRequest, "username is required")
		return
	}
	if username == s.cfg.AdminUser {
		writeErr(w, http.StatusConflict, "username is reserved")
		return
	}
	if len(body.Password) < minPassLen {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := HashPassword(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	if err := s.store.CreateUser(username, hash); err != nil {
		if errors.Is(err, database.ErrUserExists) {
			writeErr(w, http.StatusConflict, "user already exists")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	username := strings.ToLower(r.PathValue("username"))
	if username == s.cfg.AdminUser {
		writeErr(w, http.StatusForbidden, "cannot delete the seed admin account")
		return
	}
	if err := s.store.DeleteUser(username); err != nil {
		dbErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setUserPassword(w http.ResponseWriter, r *http.Request) {
	username := strings.ToLower(r.PathValue("username"))
	if username == s.cfg.AdminUser {
		writeErr(w, http.StatusForbidden, "seed admin password is managed via .env")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if len(body.Password) < minPassLen {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := HashPassword(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	if err := s.store.SetUserPassword(username, hash); err != nil {
		dbErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
