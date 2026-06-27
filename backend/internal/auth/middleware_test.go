package auth

import "testing"

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
