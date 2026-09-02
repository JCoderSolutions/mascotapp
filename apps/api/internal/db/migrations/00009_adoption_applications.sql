-- The adoption flow's spine: an application is a PROCESS, not a form submission.
--
-- LT-4 in the plan is explicit about why this table exists in this shape: *"la
-- adopción no es un formulario, es una máquina de estados"*. A submission is a
-- document somebody sent once (00008); an application is a case a shelter works,
-- with an owner, a queue position and a decision. Modelling it as the former is
-- how the product becomes a mailbox the shelters abandon.
--
-- What the DATABASE decides here is what a status may BE. Which transitions are
-- legal — `submitted` may become `in_review` but never `delivered` — is domain
-- logic in Go and is explicitly out of scope for this phase. A CHECK cannot see
-- the previous value without a trigger, and encoding a state machine in triggers
-- puts the rules where nobody looks for them.

-- +goose Up

-- +goose StatementBegin

CREATE TABLE adoption_applications (
    id                  uuid        NOT NULL PRIMARY KEY,
    shelter_id          uuid        NOT NULL REFERENCES shelters (id),

    -- Denormalised from `pets`, and paired with it below (D5). An application is
    -- always ABOUT an animal, so its shelter is the animal's shelter.
    pet_id              uuid        NOT NULL,

    -- The adopter. A single-column reference, because `users` is the one table
    -- in this schema that genuinely spans shelters (D6) and therefore has no
    -- composite key to point at. §4.1 gives an adopter a magic-link account with
    -- no membership anywhere, which is exactly why the `users` policy needs the
    -- second branch this migration adds at the bottom.
    applicant_user_id   uuid        NOT NULL REFERENCES users (id),

    -- §4.5's set, closed. The domain layer branches on this value, so a twelfth
    -- one reaches a switch with no arm for it.
    status              text        NOT NULL DEFAULT 'draft'
                                    CHECK (status IN ('draft', 'submitted', 'in_review',
                                                      'interview_scheduled',
                                                      'home_visit_scheduled', 'approved',
                                                      'rejected', 'withdrawn',
                                                      'contract_signed', 'delivered',
                                                      'returned')),
    status_changed_at   timestamptz NOT NULL DEFAULT now(),

    -- Who on the shelter's side owns this case. Nullable: unassigned is the
    -- normal state of a new application. See the composite key below -- this is
    -- NOT a plain reference to `users`.
    assigned_to_user_id uuid,

    -- Computed by the domain layer and only ever an ORDERING. §1.3: the AI
    -- assistant of Phase 12 may sort and explain, never reject.
    priority_score      integer,

    decision_note       text,

    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    -- D5. A plain `pet_id REFERENCES pets (id)` would resolve another shelter's
    -- animal on this tenant's behalf, because referential integrity checks
    -- ALWAYS BYPASS ROW SECURITY -- and nothing that READS can notice it.
    --
    -- RESTRICT, not CASCADE, for the reason `pet_status_history` was given it in
    -- 00006: `app_tenant` holds DELETE on `pets`, so a cascade would let a
    -- shelter erase every application it ever received by deleting the animal.
    -- An animal leaves the catalog by changing status, never by disappearing.
    CONSTRAINT adoption_applications_pet_fkey
        FOREIGN KEY (pet_id, shelter_id) REFERENCES pets (id, shelter_id)
        ON DELETE RESTRICT,

    -- "Assignment stays inside the shelter", enforced rather than remembered.
    --
    -- The obvious modelling is `assigned_to_user_id REFERENCES users (id)`, and
    -- it lets a shelter assign a case to somebody who has no relationship with
    -- it at all -- whose name then renders in that shelter's queue. No policy
    -- catches it either, because the tenant is writing a value into its OWN row
    -- and WITH CHECK only looks at `shelter_id`.
    --
    -- `memberships` already carries UNIQUE (user_id, shelter_id) for its own
    -- sake, which happens to be exactly the referenced key this needs. MATCH
    -- SIMPLE skips the check when any column of the key is NULL, so an
    -- unassigned application is unconstrained, which is what we want.
    --
    -- ON DELETE RESTRICT: revoking a membership is an UPDATE of its status, not
    -- a delete, so this fires only if somebody removes the row outright -- and
    -- then the application would silently lose its owner.
    CONSTRAINT adoption_applications_assignee_fkey
        FOREIGN KEY (assigned_to_user_id, shelter_id)
            REFERENCES memberships (user_id, shelter_id)
        ON DELETE RESTRICT,

    -- The referenced key of the composite foreign keys `application_events`,
    -- `application_notes` (00010, T-01-029) and `documents` (00011, T-01-031)
    -- will declare. It reads as redundant beside the primary key and is not.
    CONSTRAINT adoption_applications_id_shelter_key UNIQUE (id, shelter_id)
);

-- §4.6. The queue a refuge actually works from: its own applications, in one
-- status, newest first. `shelter_id` leads because under RLS the policy puts
-- that predicate on every query whether the caller wrote it or not.
CREATE INDEX adoption_applications_shelter_status_created_idx
    ON adoption_applications (shelter_id, status, created_at DESC);

ALTER TABLE adoption_applications ENABLE ROW LEVEL SECURITY;
ALTER TABLE adoption_applications FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON adoption_applications FOR ALL TO app_tenant
    USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

-- No `app_public` grant and no public policy, deliberately. An adoption
-- application is the most sensitive record in this schema after the submission
-- it carries; the public catalog shows animals.
REVOKE TRUNCATE                       ON adoption_applications FROM app_tenant, PUBLIC;
GRANT  SELECT, INSERT, UPDATE, DELETE ON adoption_applications TO   app_tenant;

-- ---------------------------------------------------------------------------
-- The applicant branch of the `users` policy (D6).
-- ---------------------------------------------------------------------------
--
-- Deferred out of 00002 for the ordinary reason that a policy cannot read a
-- table that does not exist. `member_visible_users` scopes visibility through
-- `memberships`, and an ADOPTER has no membership anywhere -- so without this
-- branch a shelter cannot read the name of the person applying to adopt from it.
--
-- A SECOND policy, not a widened first one. Permissive policies OR together, so
-- each states one reason a user is visible and neither can silently swallow the
-- other; a single policy with an OR inside it would be one edit away from
-- widening both paths at once.
--
-- KNOWN EXPOSURE, and it is the same one D6 recorded on `memberships`: when a
-- policy on table P derives visibility through bridge table B, WRITE permission
-- on B is READ permission on P. `app_tenant` holds INSERT here, so a tenant that
-- already knows a user's uuid can mint read access to that user's PII by
-- inserting an application naming them. Unlike `memberships`, there is no
-- business state to filter on -- the shelter writes the status too -- so no
-- predicate available at this layer closes it. The real fix is column-level
-- grants, which the board deferred to Phase 02/03 with finding B1 from Judgment
-- Day. TestApplicantPolicy_IsAsWideAsWritingAnApplication pins the current
-- behaviour so the day those grants land, the change is visible rather than
-- silent.
CREATE POLICY applicant_visible_users ON users FOR SELECT TO app_tenant
    USING (EXISTS (SELECT 1
                   FROM adoption_applications a
                   WHERE a.applicant_user_id = users.id
                     AND a.shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid));

-- ---------------------------------------------------------------------------
-- The key `form_submissions.application_id` has been waiting for (D5).
-- ---------------------------------------------------------------------------
--
-- 00008 left the column unkeyed because a reference cannot name a table that
-- does not exist, and left
-- TestFormSubmissions_GetsItsApplicationKeyWhenApplicationsLand asserting the
-- equivalence in BOTH directions so the deferral could not survive by being
-- forgotten. It did not: that test went red the moment this migration created
-- the table, and named exactly this.
--
-- ON DELETE CASCADE, and it is the OPPOSITE of the RESTRICT on every other
-- reference in this schema -- deliberately, because the direction is reversed.
-- Elsewhere the child is the history and the parent is the live row, so a
-- cascade would let a tenant erase the trail by deleting what it points at. Here
-- the child is the PERSONAL DATA and the parent is the case: §5.4 requires a
-- retention purge of rejected applications and a subject-deletion path, and both
-- of those are the answers going away. A RESTRICT would leave the adopter's
-- document behind after the case it belonged to was purged, which is the
-- privacy failure the retention policy exists to prevent.
--
-- What makes that safe is that the AUDIT TRAIL does not live here.
-- `application_events` (T-01-029) and `audit_log` (T-01-032) are append-only and
-- separate, and both must reference this table with RESTRICT for exactly the
-- reason this one does not.
ALTER TABLE form_submissions
    ADD CONSTRAINT form_submissions_application_fkey
        FOREIGN KEY (application_id, shelter_id)
            REFERENCES adoption_applications (id, shelter_id)
        ON DELETE CASCADE;

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

-- The policy lives ON `users` but READS `adoption_applications`, so PostgreSQL
-- records a dependency and refuses to drop the table while it exists (2BP01).
-- Dropped by name rather than with CASCADE: CASCADE would take whatever else
-- happens to depend on the table with it, silently.
ALTER TABLE form_submissions DROP CONSTRAINT form_submissions_application_fkey;

DROP POLICY applicant_visible_users ON users;

DROP TABLE adoption_applications;

-- +goose StatementEnd
