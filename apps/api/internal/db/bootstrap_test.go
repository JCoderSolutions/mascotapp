package db_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
)

func TestValidateRoleCredentials(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		role     string
		password string
		wantErr  error
	}{
		{name: "the tenant role", role: "app_tenant", password: "s3cret", wantErr: nil},
		{name: "the public role", role: "app_public", password: "s3cret", wantErr: nil},

		// A bootstrap willing to name any role is one that eventually gets
		// pointed at the superuser.
		{name: "postgres", role: "postgres", password: "s3cret", wantErr: db.ErrInvalidRoleName},
		{name: "the owner", role: "mascotapp", password: "s3cret", wantErr: db.ErrInvalidRoleName},
		{name: "empty", role: "", password: "s3cret", wantErr: db.ErrInvalidRoleName},
		{
			name:     "an injection attempt",
			role:     `app_x"; DROP ROLE postgres; --`,
			password: "s3cret",
			wantErr:  db.ErrInvalidRoleName,
		},
		{
			name:     "uppercase, which would fold and surprise later",
			role:     "APP_TENANT",
			password: "s3cret",
			wantErr:  db.ErrInvalidRoleName,
		},

		// PostgreSQL accepts an empty password and leaves the role usable
		// without a credential, which is worse than failing.
		{name: "no password", role: "app_tenant", password: "", wantErr: db.ErrEmptyPassword},
		{name: "blank password", role: "app_tenant", password: "   ", wantErr: db.ErrEmptyPassword},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := db.ValidateRoleCredentials(tc.role, tc.password)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("rejected a valid pair: %v", err)
				}

				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// The password must not reach the error message. Errors travel to logs, and
// logs travel further than anyone expects.
func TestValidateRoleCredentials_DoesNotLeakThePasswordIntoTheError(t *testing.T) {
	t.Parallel()

	const password = "correct-horse-battery-staple"

	err := db.ValidateRoleCredentials("postgres", password)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("the error carries the password: %v", err)
	}
}
