package rlstest_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// seedSubmission records one answer document against a published version.
func seedSubmission(
	t *testing.T, env *dbtest.Env, shelter, version uuid.UUID, answers string,
) uuid.UUID {
	t.Helper()

	id := uuid.New()
	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO form_submissions
				     (id, shelter_id, template_version_id, answers)
				 VALUES ($1, $2, $3, $4::jsonb)`,
				id, shelter, version, answers)

			return err
		})
	if err != nil {
		t.Fatalf("recording a submission against version %s: %v", version, err)
	}

	return id
}

// The spec's "Answers are searchable" scenario, and the task asks for more than
// the catalog: the index must be USED for a containment query, not merely
// present. An index nobody's plan reaches is the same as no index, except it
// also costs every write.
//
// `enable_seqscan = off` is not cheating here, and the distinction matters. The
// setting is a planner preference, not a fake: PostgreSQL will still refuse an
// index it CANNOT use for the operator in the query. So the case discriminates
// exactly the realistic bug — a btree index on `answers`, which serves no `@>`
// at all — from a GIN one, which does. On a table holding three rows the planner
// would never choose any index on cost alone, so without the setting the case
// would be about table size rather than about the index.
//
// The anti-vacuity below is what keeps that honest.
func TestFormSubmissions_AnswersAreSearchableByContainment(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	template := seedTemplate(t, env, shelter, "adoption")
	version := publishVersion(t, env, shelter, template, 1,
		definitionWith("full_name", "has_other_pets"))

	seedSubmission(t, env, shelter, version, `{"full_name":"Ana","has_other_pets":true}`)
	seedSubmission(t, env, shelter, version, `{"full_name":"Beto","has_other_pets":false}`)

	// The query actually returns the right row. A plan assertion over a query
	// that answers wrongly proves the index is fast at being wrong.
	var found int
	if err := db.WithTenant(ctx, env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`SELECT count(*) FROM form_submissions
				 WHERE answers @> '{"has_other_pets":true}'::jsonb`).Scan(&found)
		}); err != nil {
		t.Fatalf("running the containment query: %v", err)
	}
	if found != 1 {
		t.Fatalf("the containment query matched %d rows, want 1", found)
	}

	// The GIN index is named, never "some index", and the first run of this test
	// is what taught the difference. Every query under app_tenant carries the
	// policy's `shelter_id = ...` predicate, so almost any plan can reach the
	// shelter index and the string "Index Scan" shows up in almost all of them —
	// including the anti-vacuity query below, which is how it was caught.
	// Asserting the generic string would have passed with no GIN index at all.
	const gin = "form_submissions_answers_idx"

	plan := planFor(t, env,
		`SELECT id FROM form_submissions WHERE answers @> '{"has_other_pets":true}'::jsonb`)
	if !strings.Contains(plan, gin) {
		t.Errorf("the containment query does not reach %s even with sequential scans "+
			"disabled, so whatever is on `answers` cannot serve `@>`. A btree index reads "+
			"as an index in the catalog and answers no containment query at all.\nplan:\n%s",
			gin, plan)
	}

	// Anti-vacuity: the same setting does NOT drag the GIN index into a query it
	// cannot serve. Without this, `enable_seqscan = off` could be producing the
	// result above on its own and the case would assert nothing about `@>`.
	fallback := planFor(t, env,
		`SELECT id FROM form_submissions WHERE answers::text LIKE '%Ana%'`)
	if strings.Contains(fallback, gin) {
		t.Errorf("%s was planned for a LIKE over the document's text, which it cannot "+
			"serve, so the assertion above says nothing about containment.\nplan:\n%s",
			gin, fallback)
	}
}

// planFor returns the query plan with sequential scans disabled.
//
// It plans as the OWNER, and that is the fix the first run of this test forced.
// Under app_tenant every query carries the policy's `shelter_id = ...`
// predicate, and the planner satisfies it with the shelter index and then
// applies `answers @>` as a plain filter — so the plan reached an index, just
// not this one, and the GIN index was never consulted however correct it was.
// The property under test belongs to the INDEX, not to the tenant path, so the
// tenant path is taken out of the picture. This is introspection, like
// ReadTables: it asserts nothing about isolation.
//
// `SET LOCAL` keeps the preference inside this transaction, so it cannot leak
// onto a pooled connection and change how a later test plans — the failure mode
// scope_test.go exists for.
func planFor(t *testing.T, env *dbtest.Env, query string) string {
	t.Helper()

	ctx := context.Background()
	tx, err := env.OwnerPool.Begin(ctx)
	if err != nil {
		t.Fatalf("opening a transaction to explain %q: %v", query, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatalf("disabling sequential scans: %v", err)
	}

	rows, err := tx.Query(ctx, `EXPLAIN `+query)
	if err != nil {
		t.Fatalf("explaining %q: %v", query, err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scanning a plan line: %v", err)
		}
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the plan for %q: %v", query, err)
	}

	return plan.String()
}

// The DoD, asserted rather than reviewed: *"no PII in plaintext columns beyond
// what §4.4 specifies; encryption is Phase 07."*
//
// This is a contract over the COLUMN SET, and it is the one shape of test that
// catches the change nobody announces. Adding `applicant_email text` to this
// table is a two-line commit that reads as convenience and puts regulated
// personal data in a plaintext column, on a table whose whole point is that the
// sensitive half goes to `answers_encrypted` in Phase 07.
//
// It fails in BOTH directions on purpose. A missing column means §4.4 is not
// implemented; an extra one means somebody widened the record without saying so,
// and the failure names it.
func TestFormSubmissions_CarriesExactlyTheColumnsSection44Specifies(t *testing.T) {
	env := dbtest.Postgres(t)

	want := []string{
		"answers",
		"answers_encrypted",
		"application_id",
		"id",
		"ip_hash",
		"shelter_id",
		"submitted_at",
		"submitted_by_user_id",
		"template_version_id",
	}

	rows, err := env.OwnerPool.Query(context.Background(), `
		SELECT a.attname
		FROM pg_attribute a
		WHERE a.attrelid = 'public.form_submissions'::regclass
		  AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attname COLLATE "C"`)
	if err != nil {
		t.Fatalf("reading form_submissions' columns: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning a column: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading form_submissions' columns: %v", err)
	}

	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("form_submissions' columns are %v, want %v. An extra column here is "+
			"plaintext personal data on the table §5.4 says must encrypt it; a missing one "+
			"means §4.4 is not implemented",
			got, want)
	}
}

// `application_id` has no foreign key yet, because `adoption_applications` does
// not exist until T-01-027 — a reference cannot name a table that is not there.
//
// This is the guard, in both directions, so the deferral cannot survive by being
// forgotten. `media`'s public policy was rescheduled TWICE before a test like
// this caught it (T-01-019), and the cost of forgetting is the same shape here:
// a nullable uuid pointing at nothing, with no composite key, is exactly the
// cross-tenant reference D5 exists to prevent.
func TestFormSubmissions_GetsItsApplicationKeyWhenApplicationsLand(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	var applicationsExist bool
	if err := env.OwnerPool.QueryRow(ctx,
		`SELECT to_regclass('public.adoption_applications') IS NOT NULL`).
		Scan(&applicationsExist); err != nil {
		t.Fatalf("looking for adoption_applications: %v", err)
	}

	var keyed bool
	if err := env.OwnerPool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint c
			WHERE c.conrelid = 'public.form_submissions'::regclass
			  AND c.contype = 'f'
			  AND 'application_id' = ANY (
			      SELECT a.attname FROM unnest(c.conkey) AS k(attnum)
			      JOIN pg_attribute a
			        ON a.attrelid = c.conrelid AND a.attnum = k.attnum)
		)`).Scan(&keyed); err != nil {
		t.Fatalf("reading form_submissions' foreign keys: %v", err)
	}

	switch {
	case applicationsExist && !keyed:
		t.Error("`adoption_applications` exists but `form_submissions.application_id` still " +
			"has no foreign key. It must be the COMPOSITE key to (id, shelter_id) (D5): " +
			"referential checks bypass row security, so a single-column reference would let " +
			"a tenant bind its submission to another shelter's application")
	case !applicationsExist && keyed:
		t.Error("`form_submissions.application_id` carries a foreign key while " +
			"`adoption_applications` does not exist, so whatever it references, it is not " +
			"an adoption application")
	}
}

// D5 on this table's own reference, and the assertion T-01-025 shipped without.
//
// Mutation found it: `FOREIGN KEY (template_version_id) REFERENCES
// form_template_versions (id)` — the single-column form — SURVIVED the whole
// suite. Nothing that reads can notice, because referential integrity checks
// ALWAYS BYPASS ROW SECURITY: the key resolves tenant A's version on tenant B's
// behalf, and B binds its submission to a form it cannot see.
//
// That is not a filing error. A submission is the most sensitive record in this
// schema, and the version is what says WHAT WAS ASKED. A submission bound to
// another shelter's version renders A's questions against B's answers, and the
// row is invisible to A while carrying B's shelter_id.
//
// Note which cross-tenant write this is: the row carries B's OWN shelter_id, so
// the policy's WITH CHECK is satisfied and steps aside. A 42501 here is a
// FAILING result — it would mean the policy answered first and the foreign key
// was never exercised, so the key could be dropped tomorrow and this test would
// keep reporting a refusal.
func TestFormSubmissions_CannotReferenceAnotherTenantsVersion(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	tenantA, tenantB := freshTenant(t, env), freshTenant(t, env)
	versionOfA := publishVersion(t, env, tenantA,
		seedTemplate(t, env, tenantA, "adoption"), 1, definitionWith("full_name"))
	versionOfB := publishVersion(t, env, tenantB,
		seedTemplate(t, env, tenantB, "adoption"), 1, definitionWith("full_name"))

	record := func(version uuid.UUID) (string, []any) {
		return `INSERT INTO form_submissions
		            (id, shelter_id, template_version_id, answers)
		        VALUES ($1, $2, $3, '{}'::jsonb)`,
			[]any{uuid.New(), tenantB, version}
	}

	// Anti-vacuity: B can record against its OWN version. Without this, a table
	// B could not write to at all would refuse both cases below and read as
	// perfectly isolated.
	sql, args := record(versionOfB)
	if err := db.WithTenant(ctx, env.TenantPool, tenantB,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)

			return err
		}); err != nil {
		t.Fatalf("tenant B could not record a submission against its OWN version, so the "+
			"refusals below would prove nothing about isolation: %v", err)
	}

	sql, args = record(versionOfA)
	foreign := refusal(t, env, tenantB, sql, args,
		"tenant B recorded a submission against tenant A's form version")

	sql, args = record(uuid.New())
	absent := refusal(t, env, tenantB, sql, args,
		"tenant B recorded a submission against a version that does not exist")

	if foreign.Code == sqlstateInsufficientPrivilege {
		t.Fatalf("the cross-tenant submission was refused by the POLICY (%s), not by a "+
			"foreign key. The row carries B's own shelter_id, so WITH CHECK should have "+
			"passed and the composite key should have done the refusing. As written this "+
			"test cannot see the key disappear: %s",
			sqlstateInsufficientPrivilege, foreign.Message)
	}
	if foreign.Code != sqlstateForeignKeyViolation {
		t.Fatalf("naming tenant A's version was refused with %s rather than a foreign key "+
			"violation (%s), so what refused it is unproven: %s",
			foreign.Code, sqlstateForeignKeyViolation, foreign.Message)
	}

	// Rejection is not the property. INDISTINGUISHABLE rejection is: if a
	// foreign version answered differently from a nonexistent one, B could
	// enumerate tenant A's form versions one id at a time.
	if foreign.Code != absent.Code || foreign.ConstraintName != absent.ConstraintName {
		t.Errorf("form_submissions answers differently for another tenant's version (%s/%s) "+
			"than for a version that does not exist (%s/%s), so tenant B can enumerate "+
			"tenant A's published forms by reading the difference off the error",
			foreign.Code, foreign.ConstraintName, absent.Code, absent.ConstraintName)
	}
	if foreign.TableName != "form_submissions" {
		t.Errorf("the refusal was reported against %q rather than \"form_submissions\", so "+
			"the constraint that fired is not the one this case is about", foreign.TableName)
	}
}

// The end-to-end half of §4.4's historical-readability requirement, and the one
// assertion T-01-024 could not make before this table existed:
//
//	"publicar v1, enviar respuesta, publicar v2 con un campo eliminado → la
//	 respuesta de v1 sigue renderizando correctamente"
//
// `TestPublishingAVersion_AppendsAndLeavesTheOldOneByteIdentical` proves the
// versions side: v1 keeps `has_other_pets` after v2 drops it. That is necessary
// and it is not the requirement. The requirement is about a RECORDED ANSWER, and
// an answer is readable only if the path the renderer actually walks —
// submission → its version → that version's fields — still resolves every key
// the answer document carries.
//
// Rule 2 of §4.4 is what makes it possible (`field.id` is immutable for the life
// of a template), and the immutability of published versions is what makes it
// durable. Both are asserted elsewhere. What is asserted HERE is the thing they
// were for.
func TestARecordedAnswer_StillResolvesAgainstItsOwnVersionAfterTheFieldIsDropped(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	template := seedTemplate(t, env, shelter, "adoption")

	v1 := publishVersion(t, env, shelter, template, 1,
		definitionWith("full_name", "email", "has_other_pets"))
	submission := seedSubmission(t, env, shelter, v1,
		`{"full_name":"Ana","has_other_pets":true}`)

	// The edit the requirement is about: v2 no longer asks the question.
	v2 := publishVersion(t, env, shelter, template, 2, definitionWith("full_name", "email"))

	t.Run("the submission still points at the version it was filled with", func(t *testing.T) {
		var boundTo uuid.UUID
		if err := env.OwnerPool.QueryRow(ctx,
			`SELECT template_version_id FROM form_submissions WHERE id = $1`,
			submission).Scan(&boundTo); err != nil {
			t.Fatalf("reading the submission's version back: %v", err)
		}
		if boundTo != v1 {
			t.Fatalf("the submission is bound to %s rather than to v1 (%s). An answer "+
				"repointed at a later version is read against questions it was never shown",
				boundTo, v1)
		}
	})

	t.Run("every answer key resolves against that version", func(t *testing.T) {
		// The real read path, run as the TENANT, because that is who renders it.
		unresolved := unresolvedAnswerKeys(t, env, shelter, submission, v1)
		if len(unresolved) != 0 {
			t.Errorf("answer keys %v have no field in the version the submission was "+
				"recorded against, so the answer cannot be rendered. §4.4 rule 2 says "+
				"field.id is immutable for the life of a template precisely so this set "+
				"stays empty", unresolved)
		}

		// Anti-vacuity, and it is load-bearing rather than decorative: the same
		// query against v2 MUST report `has_other_pets` as unresolvable. Without
		// it a query that silently matches nothing — a wrong path into the
		// document, a typo in a key — returns zero rows above and reads as proof
		// while proving nothing.
		against2 := unresolvedAnswerKeys(t, env, shelter, submission, v2)
		if len(against2) != 1 || against2[0] != "has_other_pets" {
			t.Errorf("read against v2 the unresolvable keys are %v, want exactly "+
				"[has_other_pets]. Either v2 never dropped the field — in which case this "+
				"case is not about a removal — or the resolver query matches nothing at "+
				"all, in which case the assertion above is vacuous", against2)
		}
	})

	t.Run("the published version cannot be deleted out from under the answer",
		func(t *testing.T) {
			// What makes the readability above durable rather than lucky.
			//
			// Read the comment on the case below before trusting this one: the
			// layer that answers HERE is 00007's immutability trigger, not the
			// foreign key, because v1 is published. Mutation proved it —
			// `ON DELETE RESTRICT` changed to `ON DELETE CASCADE` left this
			// subtest green. So this asserts that a published version survives,
			// and says nothing at all about the reference.
			err := db.WithTenant(ctx, env.TenantPool, shelter,
				func(ctx context.Context, tx pgx.Tx) error {
					_, err := tx.Exec(ctx,
						`DELETE FROM form_template_versions WHERE id = $1`, v1)

					return err
				})
			if err == nil {
				t.Fatal("v1 was deleted while a submission still referenced it, so every " +
					"assertion above describes a state the shelter can erase")
			}
		})
}

// The door 00008 says `ON DELETE RESTRICT` is there to close, in its own words:
// *"A published version already cannot be deleted at all (00007's trigger); this
// closes the remaining door, which is a DRAFT version that somehow collected
// submissions."*
//
// That sentence was unasserted, and mutation is what found it: changing the
// reference to `ON DELETE CASCADE` left the whole suite green, because every
// case that deleted a version deleted a PUBLISHED one and the trigger answered
// first. The reference could have been cascading since the day it was written.
//
// A draft CAN collect submissions — nothing in this schema requires
// `published_at IS NOT NULL` to record one — and a draft is deletable by design,
// so the trigger steps aside and the foreign key is the only thing left. Under
// `CASCADE` the shelter discards a draft form and silently discards every answer
// anybody sent to it.
func TestADraftVersionCarryingAnswers_CannotBeDiscarded(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	template := seedTemplate(t, env, shelter, "adoption")

	// Anti-vacuity, and the whole reason this case can see the foreign key: an
	// EMPTY draft is deletable. Without this the refusal below would be
	// indistinguishable from "drafts are frozen too", which is the opposite of
	// what 00007 decided.
	empty := seedVersion(t, env, shelter, template, 1, false)
	if err := db.WithTenant(ctx, env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			tag, err := tx.Exec(ctx,
				`DELETE FROM form_template_versions WHERE id = $1`, empty)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				t.Errorf("deleting an empty draft affected %d rows, want 1. A draft is "+
					"editable and discardable by design (00007); if it is not, the refusal "+
					"below is about drafts rather than about the reference",
					tag.RowsAffected())
			}

			return nil
		}); err != nil {
		t.Fatalf("deleting an empty draft: %v", err)
	}

	draft := seedVersion(t, env, shelter, template, 2, false)
	seedSubmission(t, env, shelter, draft, `{"full_name":"Ana"}`)

	err := db.WithTenant(ctx, env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`DELETE FROM form_template_versions WHERE id = $1`, draft)

			return err
		})
	if err == nil {
		t.Fatal("a DRAFT version carrying a submission was deleted. The immutability " +
			"trigger does not cover a draft — by design — so `ON DELETE RESTRICT` on " +
			"form_submissions' reference is the only layer here, and the answers went " +
			"with the form")
	}
	if code := sqlstateOf(t, err, "deleting a draft version that carries a submission"); code !=
		sqlstateForeignKeyViolation {
		t.Errorf("deleting the draft was refused with %s rather than a foreign key "+
			"violation (%s), so what refused it is not the reference this case is about",
			code, sqlstateForeignKeyViolation)
	}
}

// unresolvedAnswerKeys returns the keys of the submission's answer document that
// have no field with that id in the given version's definition.
//
// It walks §4.4's shape — sections → rows → fields — rather than searching the
// document as text, because a text search would match `has_other_pets` appearing
// as a label, a logic reference or a value, and report a field that is not
// there. The renderer resolves by `field.id`; so does this.
//
// It runs as the TENANT and joins on `(id, shelter_id)`, which is the path the
// application takes, so the policy and the composite key are both in force.
func unresolvedAnswerKeys(
	t *testing.T, env *dbtest.Env, shelter, submission, version uuid.UUID,
) []string {
	t.Helper()

	var keys []string
	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `
				SELECT answer.key
				FROM form_submissions s
				JOIN form_template_versions v
				  ON v.id = $2 AND v.shelter_id = s.shelter_id
				CROSS JOIN LATERAL jsonb_object_keys(s.answers) AS answer(key)
				WHERE s.id = $1
				  AND NOT EXISTS (
				        SELECT 1
				        FROM jsonb_array_elements(v.definition -> 'sections') AS section
				        CROSS JOIN LATERAL
				             jsonb_array_elements(section -> 'rows')   AS row
				        CROSS JOIN LATERAL
				             jsonb_array_elements(row -> 'fields')     AS field
				        WHERE field ->> 'id' = answer.key
				      )
				ORDER BY answer.key COLLATE "C"`, submission, version)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var key string
				if err := rows.Scan(&key); err != nil {
					return err
				}
				keys = append(keys, key)
			}

			return rows.Err()
		})
	if err != nil {
		t.Fatalf("resolving submission %s against version %s: %v", submission, version, err)
	}

	return keys
}

// `form_submissions` carries TWO composite references, and the second one is the
// easy one to miss for exactly the reason T-01-021 found on `pet_media`: the key
// to the version is the one D5 writes out, so a single-column
// `REFERENCES adoption_applications (id)` alongside it looks finished.
//
// Mutation confirmed it at T-01-027: reducing this key survived the whole suite,
// including the enumerated child guard — which asks whether a child has A
// composite reference, and it still had the other one.
//
// It leaks the same way and about a more sensitive table. Distinguishing "that
// application belongs to another shelter" from "that application does not exist"
// hands tenant B a probe over tenant A's adoption cases.
func TestFormSubmissions_CannotBindToAnotherTenantsApplication(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	tenantA, tenantB := freshTenant(t, env), freshTenant(t, env)

	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}

	applicationOfA := seedApplication(t, env, tenantA, seedPet(t, env, tenantA), applicant)
	applicationOfB := seedApplication(t, env, tenantB, seedPet(t, env, tenantB), applicant)

	versionOfB := publishVersion(t, env, tenantB,
		seedTemplate(t, env, tenantB, "adoption"), 1, definitionWith("full_name"))

	bind := func(application uuid.UUID) (string, []any) {
		return `INSERT INTO form_submissions
		            (id, shelter_id, template_version_id, application_id, answers)
		        VALUES ($1, $2, $3, $4, '{}'::jsonb)`,
			[]any{uuid.New(), tenantB, versionOfB, application}
	}

	// Anti-vacuity: B can bind a submission to its OWN application.
	sql, args := bind(applicationOfB)
	if err := db.WithTenant(ctx, env.TenantPool, tenantB,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)

			return err
		}); err != nil {
		t.Fatalf("tenant B could not bind a submission to its OWN application, so the "+
			"refusals below would prove nothing: %v", err)
	}

	sql, args = bind(applicationOfA)
	foreign := refusal(t, env, tenantB, sql, args,
		"tenant B bound a submission to tenant A's adoption application")

	sql, args = bind(uuid.New())
	absent := refusal(t, env, tenantB, sql, args,
		"tenant B bound a submission to an application that does not exist")

	if foreign.Code != sqlstateForeignKeyViolation {
		t.Fatalf("naming tenant A's application was refused with %s rather than a foreign "+
			"key violation (%s): %s",
			foreign.Code, sqlstateForeignKeyViolation, foreign.Message)
	}
	if foreign.Code != absent.Code || foreign.ConstraintName != absent.ConstraintName {
		t.Errorf("form_submissions answers differently for another tenant's application "+
			"(%s/%s) than for one that does not exist (%s/%s), so tenant B can enumerate "+
			"tenant A's adoption cases one id at a time",
			foreign.Code, foreign.ConstraintName, absent.Code, absent.ConstraintName)
	}
}
