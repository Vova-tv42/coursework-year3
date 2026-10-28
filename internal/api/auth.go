package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	roleSupervisor  = "supervisor"
	roleCoordinator = "coordinator"
	roleRepoManager = "repo_manager"

	sessionUserIDKey     = "userID"
	sessionLifetime      = 7 * 24 * time.Hour
	minPasswordBytes     = 8
	maxPasswordBytes     = 72
	maxDisplayNameLength = 100
	maxEmailLength       = 254
	uniqueViolationCode  = "23505"

	userColumns = "id, email, display_name, role, stream, password_hash"
)

var (
	roles   = []string{roleSupervisor, roleCoordinator, roleRepoManager}
	streams = []string{"bachelor", "master"}
)

type user struct {
	ID           int64   `json:"id"`
	Email        string  `json:"email"`
	DisplayName  string  `json:"displayName"`
	Role         string  `json:"role"`
	Stream       *string `json:"stream,omitempty"`
	PasswordHash string  `json:"-"`
}

type registerRequest struct {
	Email       string  `json:"email"`
	Password    string  `json:"password"`
	DisplayName string  `json:"displayName"`
	Role        string  `json:"role"`
	Stream      *string `json:"stream"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type currentUserKey struct{}

func newSessionManager(pool *pgxpool.Pool, cookieSecure bool) *scs.SessionManager {
	sessions := scs.New()
	sessions.Store = pgxstore.New(pool)
	sessions.Lifetime = sessionLifetime
	sessions.Cookie.Secure = cookieSecure
	return sessions
}

func (s *server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := s.sessions.GetInt64(r.Context(), sessionUserIDKey)
		if userID == 0 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "sign in required")
			return
		}

		signedInUser, err := scanUser(s.pool.QueryRow(r.Context(), "SELECT "+userColumns+" FROM users WHERE id = $1", userID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "sign in required")
			return
		}
		if err != nil {
			writeInternalError(w, r, err)
			return
		}

		ctx := context.WithValue(r.Context(), currentUserKey{}, signedInUser)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func currentUser(r *http.Request) user {
	return r.Context().Value(currentUserKey{}).(user)
}

func scanUser(row pgx.Row) (user, error) {
	var scanned user
	err := row.Scan(&scanned.ID, &scanned.Email, &scanned.DisplayName, &scanned.Role, &scanned.Stream, &scanned.PasswordHash)
	return scanned, err
}

func (s *server) register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if !readJSON(w, r, &request) {
		return
	}
	request.Email = normalizeEmail(request.Email)
	if err := validateRegistration(request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", err.Error())
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	newUser, err := scanUser(s.pool.QueryRow(r.Context(), `
		INSERT INTO users (email, password_hash, display_name, role, stream)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+userColumns,
		request.Email, string(passwordHash), request.DisplayName, request.Role, request.Stream))
	var postgresError *pgconn.PgError
	isEmailTaken := errors.As(err, &postgresError) && postgresError.Code == uniqueViolationCode
	if isEmailTaken {
		writeError(w, http.StatusConflict, "email_taken", "this email is already registered")
		return
	}
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	if err := s.signIn(r.Context(), newUser.ID); err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, newUser)
}

func validateRegistration(request registerRequest) error {
	isCoordinator := request.Role == roleCoordinator
	hasStream := request.Stream != nil
	passwordBytes := len(request.Password)
	displayNameLength := utf8.RuneCountInString(request.DisplayName)

	switch {
	case !isValidEmail(request.Email):
		return errors.New("email must be a valid address like name@example.com")

	case passwordBytes < minPasswordBytes || passwordBytes > maxPasswordBytes:
		return fmt.Errorf("password must be %d-%d bytes long", minPasswordBytes, maxPasswordBytes)

	case displayNameLength == 0 || displayNameLength > maxDisplayNameLength:
		return fmt.Errorf("display name must be 1-%d characters", maxDisplayNameLength)

	case !slices.Contains(roles, request.Role):
		return errors.New("role must be supervisor, coordinator or repo_manager")

	case isCoordinator && !hasStream:
		return errors.New("a coordinator must choose a stream")

	case !isCoordinator && hasStream:
		return errors.New("only a coordinator has a stream")

	case hasStream && !slices.Contains(streams, *request.Stream):
		return errors.New("stream must be bachelor or master")
	}

	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isValidEmail(email string) bool {
	address, err := mail.ParseAddress(email)
	isPlainAddress := err == nil && address.Address == email
	return isPlainAddress && len(email) <= maxEmailLength
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if !readJSON(w, r, &request) {
		return
	}

	foundUser, err := scanUser(s.pool.QueryRow(r.Context(), "SELECT "+userColumns+" FROM users WHERE email = $1", normalizeEmail(request.Email)))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "wrong email or password")
		return
	}
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	isWrongPassword := bcrypt.CompareHashAndPassword([]byte(foundUser.PasswordHash), []byte(request.Password)) != nil
	if isWrongPassword {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "wrong email or password")
		return
	}

	if err := s.signIn(r.Context(), foundUser.ID); err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, foundUser)
}

func (s *server) signIn(ctx context.Context, userID int64) error {
	if err := s.sessions.RenewToken(ctx); err != nil {
		return fmt.Errorf("renew session token: %w", err)
	}

	s.sessions.Put(ctx, sessionUserIDKey, userID)
	return nil
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.sessions.Destroy(r.Context()); err != nil {
		writeInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *server) getMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}
