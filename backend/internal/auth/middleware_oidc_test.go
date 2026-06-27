package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
)

func TestOIDCJWTAndRBACMiddleware(t *testing.T) {
	const (
		clientID = "analyst-test-client"
		keyID    = "test-key"
	)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	var issuer string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeJSON(t, w, map[string]any{
				"issuer":                                issuer,
				"jwks_uri":                              issuer + "/keys",
				"authorization_endpoint":                issuer + "/authorize",
				"token_endpoint":                        issuer + "/token",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			writeJSON(t, w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
				Key:       &key.PublicKey,
				KeyID:     keyID,
				Algorithm: string(jose.RS256),
				Use:       "sig",
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(provider.Close)
	issuer = provider.URL

	middleware, err := New(context.Background(), false, issuer, clientID)
	if err != nil {
		t.Fatal(err)
	}
	protected := middleware.Authenticate(http.HandlerFunc(middleware.Require("analyst", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("principal missing from request context")
		}
		if principal.Subject != "subject-1" {
			t.Fatalf("unexpected subject: %s", principal.Subject)
		}
		if principal.Email != "analyst@example.test" {
			t.Fatalf("unexpected email: %s", principal.Email)
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	assertStatus(t, protected, "", http.StatusUnauthorized)
	assertStatus(t, protected, signedIDToken(t, key, keyID, issuer, clientID, []string{"viewer"}, ""), http.StatusForbidden)
	assertStatus(t, protected, expiredIDToken(t, key, keyID, issuer, clientID, []string{"analyst"}, ""), http.StatusUnauthorized)
	assertStatus(t, protected, signedIDToken(t, key, keyID, issuer, clientID, nil, "analyst"), http.StatusNoContent)
	assertStatus(t, protected, signedIDToken(t, key, keyID, issuer, clientID, []string{"admin"}, ""), http.StatusNoContent)
}

func signedIDToken(
	t *testing.T,
	key *rsa.PrivateKey,
	keyID string,
	issuer string,
	clientID string,
	roles []string,
	role string,
) string {
	t.Helper()
	return signedIDTokenWithExpiry(t, key, keyID, issuer, clientID, roles, role, time.Now().Add(5*time.Minute))
}

func expiredIDToken(
	t *testing.T,
	key *rsa.PrivateKey,
	keyID string,
	issuer string,
	clientID string,
	roles []string,
	role string,
) string {
	t.Helper()
	return signedIDTokenWithExpiry(t, key, keyID, issuer, clientID, roles, role, time.Now().Add(-5*time.Minute))
}

func signedIDTokenWithExpiry(
	t *testing.T,
	key *rsa.PrivateKey,
	keyID string,
	issuer string,
	clientID string,
	roles []string,
	role string,
	expiry time.Time,
) string {
	t.Helper()
	options := (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", keyID)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, options)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	claims := josejwt.Claims{
		Issuer:   issuer,
		Subject:  "subject-1",
		Audience: josejwt.Audience{clientID},
		Expiry:   josejwt.NewNumericDate(expiry),
		IssuedAt: josejwt.NewNumericDate(now.Add(-time.Minute)),
	}
	customClaims := struct {
		Email string   `json:"email"`
		Roles []string `json:"roles,omitempty"`
		Role  string   `json:"role,omitempty"`
	}{
		Email: "analyst@example.test",
		Roles: roles,
		Role:  role,
	}
	raw, err := josejwt.Signed(signer).Claims(claims).Claims(customClaims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertStatus(t *testing.T, handler http.Handler, token string, expected int) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != expected {
		t.Fatalf("expected HTTP %d, got %d: %s", expected, response.Code, response.Body.String())
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
