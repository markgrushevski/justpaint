package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

const (
	sessionCookieName = "jp_session"
	maxAuthBodyBytes  = 64 << 10 // 64 KiB — auth bodies are tiny
)

// Handler is the HTTP layer for auth.
type Handler struct {
	svc          *Service
	cookieSecure bool
	logger       *slog.Logger
}

// NewHandler builds the auth HTTP handler.
func NewHandler(svc *Service, cookieSecure bool, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, cookieSecure: cookieSecure, logger: logger}
}

// Routes registers the auth routes on the mux. Protected routes are wrapped in
// RequireAuth.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/register", h.Register)
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.Handle("POST /api/auth/logout", h.RequireAuth(http.HandlerFunc(h.Logout)))
	mux.Handle("GET /api/auth/me", h.RequireAuth(http.HandlerFunc(h.Me)))
}

// --- DTOs ---

type registerRequest struct {
	Login       string  `json:"login"`
	Password    string  `json:"password"`
	DisplayName *string `json:"displayName"`
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type userEnvelope struct {
	User userDTO `json:"user"`
}

type userDTO struct {
	ID          string    `json:"id"`
	Login       string    `json:"login"`
	DisplayName *string   `json:"displayName"`
	Rating      int32     `json:"rating"`
	CreatedAt   time.Time `json:"createdAt"`
}

// userResponse maps a db.User to the public envelope — note password_hash is
// never part of the DTO, so it can't leak.
func userResponse(u db.User) userEnvelope {
	return userEnvelope{User: userDTO{
		ID:          u.ID,
		Login:       u.Login,
		DisplayName: u.DisplayName,
		Rating:      u.Rating,
		CreatedAt:   u.CreatedAt,
	}}
}

// --- handlers ---

// Register: POST /api/auth/register (auth: none).
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := web.DecodeJSON(w, r, &req, maxAuthBodyBytes); err != nil {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "invalid request body")
		return
	}
	login := strings.TrimSpace(req.Login)
	displayName, dnOK := normalizeDisplayName(req.DisplayName)
	if !validLogin(login) || !validPassword(req.Password) || !dnOK {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed,
			"login (3-254 chars), password (8-256 chars), or displayName (1-64 chars) is invalid")
		return
	}

	sess, err := h.svc.Register(r.Context(), login, req.Password, displayName)
	switch {
	case errors.Is(err, ErrLoginTaken):
		web.Error(w, http.StatusConflict, web.CodeConflict, "login already taken")
		return
	case err != nil:
		h.logger.Error("register failed", "err", err)
		web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		return
	}

	h.setSession(w, sess.Token, sess.Expires)
	web.JSON(w, http.StatusCreated, userResponse(sess.User))
}

// Login: POST /api/auth/login (auth: none).
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := web.DecodeJSON(w, r, &req, maxAuthBodyBytes); err != nil {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "invalid request body")
		return
	}
	login := strings.TrimSpace(req.Login)
	if login == "" || req.Password == "" {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "login and password are required")
		return
	}

	sess, err := h.svc.Login(r.Context(), login, req.Password)
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		// Generic for both unknown login and wrong password (anti-enumeration).
		web.Error(w, http.StatusUnauthorized, web.CodeInvalidCredentials, "invalid credentials")
		return
	case err != nil:
		h.logger.Error("login failed", "err", err)
		web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		return
	}

	h.setSession(w, sess.Token, sess.Expires)
	web.JSON(w, http.StatusOK, userResponse(sess.User))
}

// Logout: POST /api/auth/logout (auth: required).
func (h *Handler) Logout(w http.ResponseWriter, _ *http.Request) {
	h.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

// Me: GET /api/auth/me (auth: required).
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserID(r.Context())
	if !ok {
		web.Error(w, http.StatusUnauthorized, web.CodeUnauthorized, "unauthorized")
		return
	}
	u, err := h.svc.User(r.Context(), uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			web.Error(w, http.StatusUnauthorized, web.CodeUnauthorized, "unauthorized")
			return
		}
		h.logger.Error("me failed", "err", err)
		web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		return
	}
	web.JSON(w, http.StatusOK, userResponse(u))
}

// --- session cookie ---

func (h *Handler) setSession(w http.ResponseWriter, token string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  exp,
		MaxAge:   int(time.Until(exp).Seconds()),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// --- validation ---

func validLogin(s string) bool {
	n := utf8.RuneCountInString(s)
	return n >= 3 && n <= 254
}

func validPassword(s string) bool {
	n := utf8.RuneCountInString(s)
	return n >= 8 && n <= 256
}

// normalizeDisplayName trims and validates the optional display name.
// Absent or empty-after-trim ⇒ (nil, true). A value must be ≤64 chars.
func normalizeDisplayName(dn *string) (*string, bool) {
	if dn == nil {
		return nil, true
	}
	t := strings.TrimSpace(*dn)
	if t == "" {
		return nil, true
	}
	if utf8.RuneCountInString(t) > 64 {
		return nil, false
	}
	return &t, true
}
