package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoleHierarchy(t *testing.T) {
	if !allows([]string{"admin"}, "viewer") {
		t.Fatal("admin must include viewer permissions")
	}
	if !allows([]string{"analyst"}, "viewer") {
		t.Fatal("analyst must include viewer permissions")
	}
	if allows([]string{"viewer"}, "analyst") {
		t.Fatal("viewer must not include analyst permissions")
	}
}

func TestNormalizeRoles(t *testing.T) {
	roles := normalizeRoles([]string{"Admin", " viewer ", "unknown", "Admin"})
	if len(roles) != 2 || roles[0] != "admin" || roles[1] != "viewer" {
		t.Fatalf("unexpected roles: %#v", roles)
	}
}

func TestDisabledAuthUsesTrustedFrontendSubject(t *testing.T) {
	middleware, err := New(context.Background(), true, "", "")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-App-User", "demo-user")
	response := httptest.NewRecorder()
	middleware.Authenticate(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		if actor := Actor(request.Context()); actor != "demo-user" {
			t.Fatalf("unexpected actor: %s", actor)
		}
	})).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}
