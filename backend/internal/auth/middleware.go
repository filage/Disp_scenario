package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Principal struct {
	Subject string
	Email   string
	Roles   []string
}

type contextKey struct{}

type Middleware struct {
	disabled bool
	verifier *oidc.IDTokenVerifier
}

func New(ctx context.Context, disabled bool, issuer, clientID string) (*Middleware, error) {
	if disabled {
		return &Middleware{disabled: true}, nil
	}
	if issuer == "" || clientID == "" {
		return nil, fmt.Errorf("OIDC_ISSUER and OIDC_CLIENT_ID are required when auth is enabled")
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	return &Middleware{verifier: provider.Verifier(&oidc.Config{ClientID: clientID})}, nil
}

func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.disabled {
			subject := strings.TrimSpace(r.Header.Get("X-App-User"))
			if subject == "" || len(subject) > 200 || strings.ContainsAny(subject, "\r\n") {
				subject = "local-user"
			}
			principal := Principal{
				Subject: subject, Email: "local@development",
				Roles: []string{"admin", "analyst", "viewer"},
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, principal)))
			return
		}
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
			writeUnauthorized(w, "bearer token is required")
			return
		}
		rawToken := strings.TrimSpace(header[len("Bearer "):])
		token, err := m.verifier.Verify(r.Context(), rawToken)
		if err != nil {
			writeUnauthorized(w, "token verification failed")
			return
		}
		var claims struct {
			Email string   `json:"email"`
			Roles []string `json:"roles"`
			Role  string   `json:"role"`
		}
		if err := token.Claims(&claims); err != nil {
			writeUnauthorized(w, "token claims are invalid")
			return
		}
		roles := claims.Roles
		if claims.Role != "" {
			roles = append(roles, claims.Role)
		}
		principal := Principal{Subject: token.Subject, Email: claims.Email, Roles: normalizeRoles(roles)}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, principal)))
	})
}

func (m *Middleware) Require(role string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok || !allows(principal.Roles, role) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"403","message":"insufficient role"}}`))
			return
		}
		handler(w, r)
	}
}

func (m *Middleware) Authorize(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok || !allows(principal.Roles, role) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"code":"403","message":"insufficient role"}}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}

func Actor(ctx context.Context) string {
	principal, ok := PrincipalFromContext(ctx)
	if !ok || principal.Subject == "" {
		return "unknown"
	}
	return principal.Subject
}

func allows(roles []string, required string) bool {
	level := map[string]int{"viewer": 1, "analyst": 2, "admin": 3}
	for _, role := range roles {
		if level[role] >= level[required] {
			return true
		}
	}
	return false
}

func normalizeRoles(roles []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if _, ok := map[string]bool{"viewer": true, "analyst": true, "admin": true}[role]; ok && !seen[role] {
			seen[role] = true
			result = append(result, role)
		}
	}
	return result
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, `{"error":{"code":"401","message":%q}}`, message)
}
