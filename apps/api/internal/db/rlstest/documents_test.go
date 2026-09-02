package rlstest_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// insertDocument writes one document as the given tenant. `application` may be
// uuid.Nil, which the caller means as "no application".
func insertDocument(
	env *dbtest.Env, shelter, media, application uuid.UUID, kind string,
) error {
	var applicationID any
	if application != uuid.Nil {
		applicationID = application
	}

	return db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO documents (id, shelter_id, media_id, application_id, type)
				 VALUES ($1, $2, $3, $4, $5)`,
				uuid.New(), shelter, media, applicationID, kind)

			return err
		})
}

// The spec's *"A document without an application is valid"*, and the mechanism
// behind it is worth naming because it is easy to get wrong twice.
//
// `application_id` is nullable AND part of a composite foreign key. PostgreSQL's
// default MATCH SIMPLE skips the whole check when ANY column of the key is NULL,
// which is exactly what makes a standalone document possible without weakening
// the key for the rows that do carry one. `MATCH FULL` would refuse the
// standalone case outright — same columns, same tables, opposite behaviour.
//
// The standalone case is not a curiosity either: §4.5 lists `receipt` and
// `health_certificate`, and a shelter files both against an animal long before
// anybody applies to adopt it.
func TestDocuments_MayStandAloneWithoutAnApplication(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	media := seedMedia(t, env, shelter)

	if err := insertDocument(env, shelter, media, uuid.Nil, "health_certificate"); err != nil {
		t.Fatalf("a document with no application was refused: %v. §4.5 makes application_id "+
			"nullable because a shelter files a health certificate against an animal long "+
			"before anybody applies for it", err)
	}

	// Anti-vacuity: one WITH an application also works. Without this, a column
	// that refused every non-null value would pass the case above and read as
	// "standalone documents are supported".
	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}
	application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)

	if err := insertDocument(env, shelter, media, application, "adoption_contract"); err != nil {
		t.Fatalf("a document attached to its own shelter's application was refused: %v. The "+
			"standalone case above then proves nothing about the key", err)
	}
}

// The spec's *"A document cannot point at another tenant's media"*, with the
// oracle-closing half the spec does not spell out.
//
// A shelter's media keys are not public, so distinguishing "that file belongs to
// someone else" from "that file does not exist" hands tenant B a probe over
// tenant A's storage — the same leak T-01-021 closed on `pet_media`.
func TestDocuments_CannotPointAtAnotherTenantsMedia(t *testing.T) {
	env := dbtest.Postgres(t)

	tenantA, tenantB := freshTenant(t, env), freshTenant(t, env)
	mediaOfA := seedMedia(t, env, tenantA)

	file := func(media uuid.UUID) (string, []any) {
		return `INSERT INTO documents (id, shelter_id, media_id, type)
		        VALUES ($1, $2, $3, 'custom')`,
			[]any{uuid.New(), tenantB, media}
	}

	// Anti-vacuity: B can file its own media.
	if err := insertDocument(env, tenantB, seedMedia(t, env, tenantB), uuid.Nil, "custom"); err != nil {
		t.Fatalf("tenant B could not file its OWN media, so the refusals below prove "+
			"nothing: %v", err)
	}

	sql, args := file(mediaOfA)
	foreign := refusal(t, env, tenantB, sql, args,
		"tenant B filed a document pointing at tenant A's media")

	sql, args = file(uuid.New())
	absent := refusal(t, env, tenantB, sql, args,
		"tenant B filed a document pointing at media that does not exist")

	if foreign.Code == sqlstateInsufficientPrivilege {
		t.Fatalf("the cross-tenant document was refused by the POLICY (%s), not by a foreign "+
			"key. The row carries B's own shelter_id, so WITH CHECK should have passed: %s",
			sqlstateInsufficientPrivilege, foreign.Message)
	}
	if foreign.Code != sqlstateForeignKeyViolation {
		t.Fatalf("naming tenant A's media was refused with %s rather than a foreign key "+
			"violation (%s): %s", foreign.Code, sqlstateForeignKeyViolation, foreign.Message)
	}
	if foreign.Code != absent.Code || foreign.ConstraintName != absent.ConstraintName {
		t.Errorf("documents answers differently for another tenant's media (%s/%s) than for "+
			"media that does not exist (%s/%s), so tenant B can probe tenant A's storage one "+
			"id at a time",
			foreign.Code, foreign.ConstraintName, absent.Code, absent.ConstraintName)
	}
}

// `documents` carries TWO composite references, and this is the second one —
// the shape that has now been missed twice in this schema (`pet_media`'s media
// key in T-01-020, `form_submissions`' application key in T-01-027). The key
// that D5 writes out gets written; the other one looks finished beside it.
//
// A document bound to another shelter's application is worse than a filing
// error: an adoption contract is the record of who adopted which animal from
// whom.
func TestDocuments_CannotBindToAnotherTenantsApplication(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	tenantA, tenantB := freshTenant(t, env), freshTenant(t, env)

	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}
	applicationOfA := seedApplication(t, env, tenantA, seedPet(t, env, tenantA), applicant)
	applicationOfB := seedApplication(t, env, tenantB, seedPet(t, env, tenantB), applicant)
	mediaOfB := seedMedia(t, env, tenantB)

	bind := func(application uuid.UUID) (string, []any) {
		return `INSERT INTO documents (id, shelter_id, media_id, application_id, type)
		        VALUES ($1, $2, $3, $4, 'adoption_contract')`,
			[]any{uuid.New(), tenantB, mediaOfB, application}
	}

	// Anti-vacuity: B can bind a document to its OWN application.
	if err := insertDocument(env, tenantB, mediaOfB, applicationOfB, "adoption_contract"); err != nil {
		t.Fatalf("tenant B could not bind a document to its OWN application, so the "+
			"refusals below prove nothing: %v", err)
	}

	sql, args := bind(applicationOfA)
	foreign := refusal(t, env, tenantB, sql, args,
		"tenant B bound a document to tenant A's adoption application")

	sql, args = bind(uuid.New())
	absent := refusal(t, env, tenantB, sql, args,
		"tenant B bound a document to an application that does not exist")

	if foreign.Code != sqlstateForeignKeyViolation {
		t.Fatalf("naming tenant A's application was refused with %s rather than a foreign "+
			"key violation (%s): %s",
			foreign.Code, sqlstateForeignKeyViolation, foreign.Message)
	}
	if foreign.Code != absent.Code || foreign.ConstraintName != absent.ConstraintName {
		t.Errorf("documents answers differently for another tenant's application (%s/%s) "+
			"than for one that does not exist (%s/%s), so tenant B can enumerate tenant A's "+
			"adoption cases one id at a time",
			foreign.Code, foreign.ConstraintName, absent.Code, absent.ConstraintName)
	}
}

// The spec's four types, closed — and closed for the reason that decides every
// closed union in this schema: the domain BRANCHES on it. Phase 09 generates a
// PDF from a template chosen by this value, so a fifth type reaches a switch
// with no arm for it and either produces the wrong document or none.
func TestDocuments_TypeIsAClosedUnion(t *testing.T) {
	env := dbtest.Postgres(t)

	shelter := freshTenant(t, env)
	media := seedMedia(t, env, shelter)

	for _, kind := range []string{
		"adoption_contract", "receipt", "health_certificate", "custom",
	} {
		if err := insertDocument(env, shelter, media, uuid.Nil, kind); err != nil {
			t.Errorf("type %q is in §4.5's set and the database refused it: %v", kind, err)
		}
	}

	if err := insertDocument(env, shelter, media, uuid.Nil, "invoice"); err == nil {
		t.Error("`invoice` was accepted as a document type. The set is closed because " +
			"Phase 09 picks a PDF template from this value: a fifth type reaches a switch " +
			"with no arm for it")
	}
}

// The evidence rule, applied to the third and last child of an application.
//
// An adoption contract is the record of who took which animal home. If deleting
// the application took its documents, a retention purge would erase the contract
// while leaving the animal marked `adopted` — the shelter would have no record of
// who has it.
//
// Same RESTRICT as `application_events` and `application_notes` (T-01-029), and
// the same reason: 00009 let `form_submissions` CASCADE because there the child
// is the PERSONAL DATA, and that is only safe while the EVIDENCE does not.
func TestDocuments_CannotBeErasedByDeletingTheirApplication(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}

	deleteApplication := func(application uuid.UUID) error {
		return db.WithTenant(ctx, env.TenantPool, shelter,
			func(ctx context.Context, tx pgx.Tx) error {
				tag, err := tx.Exec(ctx,
					`DELETE FROM adoption_applications WHERE id = $1`, application)
				if err != nil {
					return err
				}
				if tag.RowsAffected() != 1 {
					t.Errorf("deleting the application affected %d rows rather than 1",
						tag.RowsAffected())
				}

				return nil
			})
	}

	// Anti-vacuity: an application with no documents deletes cleanly.
	bare := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)
	if err := deleteApplication(bare); err != nil {
		t.Fatalf("an application with no documents could not be deleted, so the refusal "+
			"below says nothing about documents: %v", err)
	}

	application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)
	if err := insertDocument(env, shelter, seedMedia(t, env, shelter), application,
		"adoption_contract"); err != nil {
		t.Fatalf("filing the contract: %v", err)
	}

	err := deleteApplication(application)
	if err == nil {
		t.Fatal("the application was deleted and took its adoption contract with it. That " +
			"is the record of who took the animal home; a retention purge would erase it " +
			"while leaving the pet marked `adopted`, and the shelter would have no record " +
			"of who has it")
	}
	if code := sqlstateOf(t, err, "deleting an application that has documents"); code !=
		sqlstateForeignKeyViolation {
		t.Errorf("the delete was refused with %s rather than a foreign key violation (%s), "+
			"so ON DELETE RESTRICT is not what refused it", code, sqlstateForeignKeyViolation)
	}
}

// A document with no file is not a document, and mutation is what showed the
// column was unguarded: dropping `NOT NULL` from `media_id` broke no test.
//
// It costs more than an empty row. `media_id` is half of the composite key to
// `media`, and MATCH SIMPLE — the same rule that makes the standalone-application
// case above work — skips the ENTIRE check when any column of the key is NULL. So
// a nullable `media_id` does not merely allow a document with no file: it turns
// D5's tenant check off for that row.
//
// And downstream, `media` is where the storage key lives (§5.5). A null here
// reaches Phase 04's signed-URL path and Phase 09's PDF filer with nothing to
// dereference.
func TestDocuments_RequireTheirFile(t *testing.T) {
	env := dbtest.Postgres(t)

	shelter := freshTenant(t, env)

	// Anti-vacuity: the same row WITH a file is accepted, so a refusal below is
	// about `media_id` and not about anything else on the row.
	if err := insertDocument(env, shelter, seedMedia(t, env, shelter), uuid.Nil,
		"receipt"); err != nil {
		t.Fatalf("a document with a file was refused, so the refusal below says nothing "+
			"about media_id: %v", err)
	}

	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO documents (id, shelter_id, media_id, type)
				 VALUES ($1, $2, NULL, 'receipt')`,
				uuid.New(), shelter)

			return err
		})
	if err == nil {
		t.Fatal("a document was filed with no media_id. It is a row that renders as a " +
			"broken link — and because media_id is half of the composite key to media, " +
			"MATCH SIMPLE skips D5's tenant check entirely for it")
	}
	if code := sqlstateOf(t, err, "filing a document with no file"); code !=
		sqlstateNotNullViolation {
		t.Errorf("the insert was refused with %s rather than a not-null violation (%s), so "+
			"NOT NULL is not what refused it", code, sqlstateNotNullViolation)
	}
}
