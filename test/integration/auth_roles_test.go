//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/kodiahbertrand/pillow/internal/auth"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
)

func TestIntegration_UserRoles(t *testing.T) {
	ctx := context.Background()
	db, cleanup := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanup()

	user := &pgmodels.UserModel{Email: "roles@example.com", DisplayName: "Roles User"}
	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	repo := auth.NewRepository(db, nil)

	if err := repo.AssignRole(ctx, user.ID, "user"); err != nil {
		t.Fatalf("assign user role: %v", err)
	}
	if err := repo.AssignRole(ctx, user.ID, "admin"); err != nil {
		t.Fatalf("assign admin role: %v", err)
	}
	if err := repo.AssignRole(ctx, user.ID, "admin"); err != nil {
		t.Fatalf("re-assigning the same role must be a no-op, got: %v", err)
	}

	roles, err := repo.ListRoles(ctx, user.ID)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("roles = %v, want exactly [user admin]", roles)
	}
	if roles[0] != "user" || roles[1] != "admin" {
		t.Errorf("roles = %v, want [user admin] in insertion order", roles)
	}

	if err := repo.RemoveRole(ctx, user.ID, "admin"); err != nil {
		t.Fatalf("remove role: %v", err)
	}
	roles, err = repo.ListRoles(ctx, user.ID)
	if err != nil {
		t.Fatalf("list roles after remove: %v", err)
	}
	if len(roles) != 1 || roles[0] != "user" {
		t.Errorf("roles after revoke = %v, want [user]", roles)
	}
}
