package auth

import (
	"context"
	"net/http"

	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

// ctxKey is an unexported type so our context key can't collide with keys from
// other packages (a context.WithValue best practice).
type ctxKey int

const userIDKey ctxKey = iota

// UserID returns the authenticated user id that RequireAuth placed in ctx.
func UserID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	return v, ok
}

// RequireAuth verifies the jp_session cookie and injects the user id into the
// request context for downstream handlers, or responds 401.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName)
		if err != nil {
			web.Error(w, http.StatusUnauthorized, web.CodeUnauthorized, "unauthorized")
			return
		}
		uid, err := parseToken(h.svc.jwtSecret, c.Value)
		if err != nil {
			web.Error(w, http.StatusUnauthorized, web.CodeUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
