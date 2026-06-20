// Package auth implements registration, login, sessions (JWT in an httpOnly
// cookie), and route protection. See docs/API.md §2,§4,§5.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

// Sentinel errors the handler maps onto HTTP responses.
var (
	ErrLoginTaken         = errors.New("auth: login already taken")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
)

// Session is the result of a successful register/login.
type Session struct {
	User    db.User
	Token   string
	Expires time.Time
}

// Service holds the auth business logic. It depends on the generated queries
// and the signing secret, not on HTTP.
type Service struct {
	q         *db.Queries
	jwtSecret string
	dummyHash string // a real bcrypt hash, compared against on unknown-login (anti-enumeration)
	now       func() time.Time
}

// NewService builds the auth service. The dummy hash is computed once so a login
// attempt for an unknown user still spends bcrypt time (see Login).
func NewService(q *db.Queries, jwtSecret string) (*Service, error) {
	// Fail fast if bcrypt can't run at boot — a server that can't hash cannot
	// serve auth, and an empty dummyHash would silently break the timing defense.
	dummy, err := hashPassword("not-a-real-password")
	if err != nil {
		return nil, fmt.Errorf("auth: precompute dummy hash: %w", err)
	}
	return &Service{q: q, jwtSecret: jwtSecret, dummyHash: dummy, now: time.Now}, nil
}

// Register creates a user and returns a session. A duplicate login surfaces as
// ErrLoginTaken (the one place the API reveals existence — the user picks their
// own identifier, docs/API.md §5).
func (s *Service) Register(ctx context.Context, login, password string, displayName *string) (Session, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return Session{}, err
	}
	u, err := s.q.CreateUser(ctx, db.CreateUserParams{
		Login:        login,
		PasswordHash: hash,
		DisplayName:  displayName,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Session{}, ErrLoginTaken
		}
		return Session{}, err
	}
	return s.session(u)
}

// Login verifies credentials and returns a session. Unknown login and wrong
// password both return ErrInvalidCredentials after a bcrypt comparison, so the
// two cases are indistinguishable in result and timing (docs/API.md §5).
func (s *Service) Login(ctx context.Context, login, password string) (Session, error) {
	u, err := s.q.GetUserByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_ = comparePassword(s.dummyHash, password) // spend bcrypt time even on unknown login
			return Session{}, ErrInvalidCredentials
		}
		return Session{}, err
	}
	if err := comparePassword(u.PasswordHash, password); err != nil {
		return Session{}, ErrInvalidCredentials
	}
	return s.session(u)
}

// User fetches a user by id (used by the /me probe).
func (s *Service) User(ctx context.Context, id string) (db.User, error) {
	return s.q.GetUserByID(ctx, id)
}

func (s *Service) session(u db.User) (Session, error) {
	token, exp, err := issueToken(s.jwtSecret, u.ID, s.now())
	if err != nil {
		return Session{}, err
	}
	return Session{User: u, Token: token, Expires: exp}, nil
}

// isUniqueViolation reports whether err is a Postgres unique-constraint error (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
