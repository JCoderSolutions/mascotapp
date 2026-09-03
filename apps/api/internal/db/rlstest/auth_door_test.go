package rlstest_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// Phase 02's auth door (P2-D1...D3, migration 00013): app_auth's own role,
// pool and GUC over refresh_tokens, users and memberships. Every case here
// runs through db.WithAuthUser or db.WithAuthLookup, never through the owner
// or app_tenant -- the same discipline the tenant A/B suite holds itself to
// in isolation_test.go.
//
// TestRefreshTokens_RefuseAppTenantEntirely (scope_test.go) already proves
// this table's app_tenant half of the spec scenario "refresh_tokens is
// reachable only through the auth role"; app_public's half and the positive
// app_auth path -- read, write, and A/B isolation keyed on user_id -- are
// proved here.

// seedRefreshToken creates one session row as the owner, for fixtures a case
// does not assert about directly. Idempotent: the container is shared and any
// case may be re-run alone.
func seedRefreshToken(
	ctx context.Context, env *dbtest.Env, id, user uuid.UUID, tokenHash []byte,
) error {
	_, err := env.OwnerPool.Exec(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, family_id, expires_at)
		 VALUES ($1, $2, $3, $4, now() + interval '30 days')
		 ON CONFLICT (id) DO NOTHING`,
		id, user, tokenHash, uuid.New())
	if err != nil {
		return fmt.Errorf("seeding a refresh token for %s: %w", user, err)
	}

	return nil
}

// The positive path: app_auth performs exactly what a rotation does -- an
// insert, a lookup by token_hash, and the follow-up write -- all inside one
// user's scope, and all succeed. This is the door P2-D2 describes.
func TestAuthPool_RotatesRefreshTokensThroughTheDoor(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	user := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, user); err != nil {
		t.Fatalf("seeding the user: %v", err)
	}

	id := uuid.New()
	tokenHash := []byte("hash-" + uuid.NewString())

	err := db.WithAuthUser(ctx, env.AuthPool, user,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO refresh_tokens (id, user_id, token_hash, family_id, expires_at)
				 VALUES ($1, $2, $3, $4, now() + interval '30 days')`,
				id, user, tokenHash, uuid.New())

			return err
		})
	if err != nil {
		t.Fatalf("app_auth could not insert a refresh token in its own scope: %v", err)
	}

	err = db.WithAuthUser(ctx, env.AuthPool, user,
		func(ctx context.Context, tx pgx.Tx) error {
			var found uuid.UUID

			return tx.QueryRow(ctx,
				`SELECT id FROM refresh_tokens WHERE token_hash = $1`, tokenHash).Scan(&found)
		})
	if err != nil {
		t.Fatalf("app_auth could not look up its own row by token_hash: %v", err)
	}

	// The follow-up write a rotation makes: mark the presented token rotated.
	err = db.WithAuthUser(ctx, env.AuthPool, user,
		func(ctx context.Context, tx pgx.Tx) error {
			tag, err := tx.Exec(ctx,
				`UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1`, id)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return fmt.Errorf("the rotation write affected %d rows, want 1", tag.RowsAffected())
			}

			return nil
		})
	if err != nil {
		t.Fatalf("app_auth could not rotate its own refresh token: %v", err)
	}
}

// app_public's half of the spec scenario. app_tenant's half is
// TestRefreshTokens_RefuseAppTenantEntirely in scope_test.go; refresh_tokens
// carries no grant for either role.
func TestRefreshTokens_RefuseAppPublicEntirely(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		sql  string
		args []any
	}{
		{"reading", `SELECT count(*) FROM refresh_tokens`, nil},
		{
			"writing",
			`INSERT INTO refresh_tokens (id, user_id, token_hash, family_id, expires_at)
			 VALUES ($1, $2, $3, $4, now())`,
			[]any{uuid.New(), uuid.New(), []byte("hash"), uuid.New()},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.PublicPool.Exec(ctx, tc.sql, tc.args...)

			requireInsufficientPrivilege(t, err,
				"app_public reached refresh_tokens. The door is app_auth only (P2-D1); "+
					"app_public gets no grant on it at all")
		})
	}
}

// The A/B shape keyed on user_id, mirroring the tenant suite's own completion
// rule but for the auth door: a user scoped to A must be blind to B's
// sessions, and the forged write must be refused by WITH CHECK rather than
// merely filtered.
func TestAuthPool_UserAIsBlindToUserBsRefreshTokens(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	userA, userB := uuid.New(), uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, userA); err != nil {
		t.Fatalf("seeding user A: %v", err)
	}
	if err := seedMemberUser(ctx, env.OwnerPool, userB); err != nil {
		t.Fatalf("seeding user B: %v", err)
	}

	bsSession := uuid.New()
	if err := seedRefreshToken(ctx, env, bsSession, userB, []byte("hash-"+uuid.NewString())); err != nil {
		t.Fatalf("seeding B's session: %v", err)
	}

	// A cannot READ B's row. The query itself must succeed -- the policy
	// filters, it does not refuse the statement -- and separately the row
	// must not be visible.
	var visible int
	err := db.WithAuthUser(ctx, env.AuthPool, userA,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`SELECT count(*) FROM refresh_tokens WHERE id = $1`, bsSession).Scan(&visible)
		})
	if err != nil {
		t.Fatalf("reading under A's scope: %v", err)
	}
	if visible != 0 {
		t.Errorf("user A can see %d of user B's refresh_tokens rows", visible)
	}

	// A's write against B's row reaches zero rows -- the SELECT-then-filter
	// shape, not a refusal, because the statement is qualified.
	err = db.WithAuthUser(ctx, env.AuthPool, userA,
		func(ctx context.Context, tx pgx.Tx) error {
			tag, err := tx.Exec(ctx,
				`UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1`, bsSession)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 0 {
				return fmt.Errorf("user A's UPDATE touched %d of user B's rows", tag.RowsAffected())
			}

			return nil
		})
	if err != nil {
		t.Fatalf("A's qualified write against B's row: %v", err)
	}

	// A forged insert -- a row stamped with B's user_id while scoped to A --
	// is refused by WITH CHECK.
	err = db.WithAuthUser(ctx, env.AuthPool, userA,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO refresh_tokens (id, user_id, token_hash, family_id, expires_at)
				 VALUES ($1, $2, $3, $4, now())`,
				uuid.New(), userB, []byte("forged-"+uuid.NewString()), uuid.New())

			return err
		})
	requireInsufficientPrivilege(t, err,
		"user A inserted a refresh_tokens row stamped with user B's id. The policy's "+
			"WITH CHECK is missing or permissive")

	// B still sees its own row -- a policy that hid it from everyone would
	// pass every assertion above and still be wrong.
	err = db.WithAuthUser(ctx, env.AuthPool, userB,
		func(ctx context.Context, tx pgx.Tx) error {
			var found int

			if err := tx.QueryRow(ctx,
				`SELECT count(*) FROM refresh_tokens WHERE id = $1`, bsSession).Scan(&found); err != nil {
				return err
			}
			if found != 1 {
				return fmt.Errorf("user B can no longer see its own session (found %d)", found)
			}

			return nil
		})
	if err != nil {
		t.Fatalf("B re-reading its own row: %v", err)
	}
}

// The read policy on users is USING (true), narrowed by column grant instead
// of a row predicate (P2-D3): app_auth can read the credential columns and
// cannot read full_name or phone at all.
func TestAuthPool_ReadsCredentialColumnsOnUsersButNotPII(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	id := uuid.New()
	email := "credential-check." + uuid.NewString() + "@example.org"
	if _, err := env.OwnerPool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, full_name, phone)
		 VALUES ($1, $2, 'argon2id$stub', 'A Name', '+54 11 5555 5555')`,
		id, email); err != nil {
		t.Fatalf("seeding the user: %v", err)
	}

	err := db.WithAuthLookup(ctx, env.AuthPool,
		func(ctx context.Context, tx pgx.Tx) error {
			var gotID uuid.UUID
			var gotEmail string
			return tx.QueryRow(ctx,
				`SELECT id, email, password_hash, status, totp_secret_enc, email_verified_at,
				        created_at
				 FROM users WHERE id = $1`, id).
				Scan(&gotID, &gotEmail, new(*string), new(string), new([]byte),
					new(*time.Time), new(time.Time))
		})
	if err != nil {
		t.Fatalf("app_auth could not read the credential columns it was granted: %v", err)
	}

	for _, column := range []string{"full_name", "phone"} {
		t.Run(column, func(t *testing.T) {
			err := db.WithAuthLookup(ctx, env.AuthPool,
				func(ctx context.Context, tx pgx.Tx) error {
					var v *string

					return tx.QueryRow(ctx,
						fmt.Sprintf(`SELECT %s FROM users WHERE id = $1`, column), id).Scan(&v)
				})
			requireInsufficientPrivilege(t, err,
				fmt.Sprintf("app_auth read users.%s. It carries no SELECT grant on that "+
					"column: the read policy is USING (true), and the PII columns are "+
					"narrowed out at the grant, not the row (P2-D3)", column))
		})
	}
}

// Registration writes full_name at INSERT time and never reads it back
// through the auth door -- the deliberate asymmetry P2-D3 states: app_auth
// can WRITE the column and cannot SELECT it, ever, for any row.
func TestAuthPool_RegistersAUserButNeverReadsFullNameBack(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	newUser := uuid.New()
	email := "register." + uuid.NewString() + "@example.org"

	err := db.WithAuthUser(ctx, env.AuthPool, newUser,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO users (id, email, password_hash, full_name, phone)
				 VALUES ($1, $2, 'argon2id$stub', 'Registered Person', '+54 11 4444 4444')`,
				newUser, email)

			return err
		})
	if err != nil {
		t.Fatalf("app_auth could not register a new user: %v", err)
	}

	// The write landed -- provable through a column app_auth CAN read.
	err = db.WithAuthLookup(ctx, env.AuthPool,
		func(ctx context.Context, tx pgx.Tx) error {
			var gotEmail string

			if err := tx.QueryRow(ctx,
				`SELECT email FROM users WHERE id = $1`, newUser).Scan(&gotEmail); err != nil {
				return err
			}
			if gotEmail != email {
				return fmt.Errorf("email = %q, want %q", gotEmail, email)
			}

			return nil
		})
	if err != nil {
		t.Fatalf("app_auth could not read back the row it just registered: %v", err)
	}

	err = db.WithAuthLookup(ctx, env.AuthPool,
		func(ctx context.Context, tx pgx.Tx) error {
			var fullName *string

			return tx.QueryRow(ctx,
				`SELECT full_name FROM users WHERE id = $1`, newUser).Scan(&fullName)
		})
	requireInsufficientPrivilege(t, err,
		"app_auth read full_name back for a row it just wrote. It can write the column "+
			"at registration and must never read it back -- that is the deliberate "+
			"asymmetry P2-D3 states, and the reason full_name and phone are the PII "+
			"columns left out of the SELECT grant")
}

// memberships (P2-D3): app_auth's read is both column- and row-scoped to the
// caller's own rows, so login can list a user's shelters before a shelter_id
// claim exists without ever seeing another user's membership.
func TestAuthPool_ReadsOnlyItsOwnMemberships(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := uuid.New()
	if err := seedShelters(ctx, env.OwnerPool, shelter); err != nil {
		t.Fatalf("seeding the shelter: %v", err)
	}

	userA, userB := uuid.New(), uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, userA); err != nil {
		t.Fatalf("seeding user A: %v", err)
	}
	if err := seedMemberUser(ctx, env.OwnerPool, userB); err != nil {
		t.Fatalf("seeding user B: %v", err)
	}
	if err := seedMembership(ctx, env.OwnerPool, userA, shelter); err != nil {
		t.Fatalf("seeding A's membership: %v", err)
	}
	if err := seedMembership(ctx, env.OwnerPool, userB, shelter); err != nil {
		t.Fatalf("seeding B's membership: %v", err)
	}

	err := db.WithAuthUser(ctx, env.AuthPool, userA,
		func(ctx context.Context, tx pgx.Tx) error {
			var own, other int
			if err := tx.QueryRow(ctx,
				`SELECT count(*) FROM memberships WHERE user_id = $1`, userA).Scan(&own); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx,
				`SELECT count(*) FROM memberships WHERE user_id = $1`, userB).Scan(&other); err != nil {
				return err
			}
			if own != 1 {
				return fmt.Errorf("user A cannot see its own membership (found %d)", own)
			}
			if other != 0 {
				return fmt.Errorf("user A can see %d of user B's memberships", other)
			}

			return nil
		})
	if err != nil {
		t.Fatalf("app_auth's own-membership scope: %v", err)
	}
}
