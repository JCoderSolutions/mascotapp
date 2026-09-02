-- Dynamic forms, part one: the template and its VERSIONS.
--
-- The whole design rests on one rule from §4.4: a published version is never
-- edited, editing publishes version + 1. Without it a submission recorded months
-- ago renders against a definition that has since changed shape, and the answers
-- stop meaning anything. That is not a validation concern -- it is the reason
-- historical data stays readable -- so it is enforced here rather than by
-- whoever remembers to check.
--
-- The immutability is CONDITIONAL, which is what makes it different from
-- `pet_status_history`. A draft version is fully editable; a published one is
-- frozen. No grant can express "these rows but not those", so the mechanism has
-- to be a trigger.

-- +goose Up

-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- form_templates -- the named form a shelter addresses by key.
-- ---------------------------------------------------------------------------

CREATE TABLE form_templates (
    id         uuid        NOT NULL PRIMARY KEY,
    shelter_id uuid        NOT NULL REFERENCES shelters (id),

    -- The stable handle the application resolves a form by: 'adoption',
    -- 'home_visit'. Scoped per shelter, never globally -- see the UNIQUE below.
    key        text        NOT NULL CHECK (length(key) BETWEEN 1 AND 64),
    name       text        NOT NULL,

    purpose    text        NOT NULL
                           CHECK (purpose IN ('adoption_application', 'home_visit',
                                              'followup', 'custom')),
    is_active  boolean     NOT NULL DEFAULT true,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- Per shelter, and this is a product requirement before it is a security
    -- one: every shelter names its adoption form 'adoption'. A global key would
    -- give the word to whoever signed up first.
    --
    -- It is also the same existence oracle `pets.microchip_id` was (T-01-019):
    -- uniqueness is checked before any policy, so a global key would let one
    -- shelter type a word and learn from the 23505 that another shelter uses it.
    CONSTRAINT form_templates_key_per_shelter UNIQUE (shelter_id, key),

    -- The referenced key of the composite foreign key below (D5). It reads as
    -- redundant beside the primary key and is not.
    CONSTRAINT form_templates_id_shelter_key UNIQUE (id, shelter_id)
);

CREATE INDEX form_templates_shelter_active_idx
    ON form_templates (shelter_id, is_active);

ALTER TABLE form_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_templates FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON form_templates FOR ALL TO app_tenant
    USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON form_templates TO app_tenant;

-- ---------------------------------------------------------------------------
-- form_template_versions -- immutable once published.
-- ---------------------------------------------------------------------------

CREATE TABLE form_template_versions (
    id           uuid        NOT NULL PRIMARY KEY,
    shelter_id   uuid        NOT NULL REFERENCES shelters (id),
    template_id  uuid        NOT NULL,

    version      integer     NOT NULL CHECK (version >= 1),
    definition   jsonb       NOT NULL,

    -- NULL means draft. Non-null means frozen, and the trigger below reads
    -- exactly this column to decide.
    published_at timestamptz,
    created_by   uuid        REFERENCES users (id),
    created_at   timestamptz NOT NULL DEFAULT now(),

    -- §4.4 writes this key as UNIQUE (template_id, version). It is scoped per
    -- shelter here instead, and the deviation is deliberate: the spec's intent
    -- is preserved exactly while a cross-tenant leak is closed.
    --
    -- A template belongs to exactly one shelter, so per-shelter uniqueness over
    -- its versions IS per-template uniqueness -- nothing about the guarantee
    -- inside a tenant changes.
    --
    -- What changes is what another tenant can learn. VERIFIED against PG 17
    -- rather than assumed: THE UNIQUE INDEX IS CHECKED BEFORE THE FOREIGN KEY
    -- (referential checks run as AFTER triggers; the index insert happens on the
    -- heap write). So with a global key, tenant B -- stamped with its OWN
    -- shelter_id, which satisfies the policy -- could name tenant A's template
    -- and read the answer off the SQLSTATE: 23505 means that version exists,
    -- 23503 means it does not. Two answers to a question B has no right to ask.
    -- Third time this phase: microchip_id (T-01-019), the cover photo
    -- (T-01-020), and now this.
    CONSTRAINT form_template_versions_number_per_template
        UNIQUE (shelter_id, template_id, version),

    -- D5. A plain `template_id REFERENCES form_templates (id)` would resolve
    -- another shelter's template on this tenant's behalf, because referential
    -- integrity checks ALWAYS BYPASS ROW SECURITY.
    --
    -- RESTRICT, not CASCADE, for the same reason `pet_status_history` uses it
    -- (T-01-020): a published version that the shelter can erase by deleting its
    -- template is not immutable, and the trigger below would never see it
    -- happen. Retiring a form is `is_active = false`, which is what the column
    -- is for.
    CONSTRAINT form_template_versions_template_fkey
        FOREIGN KEY (template_id, shelter_id) REFERENCES form_templates (id, shelter_id)
        ON DELETE RESTRICT
);

CREATE INDEX form_template_versions_template_idx
    ON form_template_versions (shelter_id, template_id, version DESC);

ALTER TABLE form_template_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_template_versions FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON form_template_versions FOR ALL TO app_tenant
    USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

-- UPDATE and DELETE ARE granted here, unlike on pet_status_history, because a
-- DRAFT version has to be editable and discardable. The grant cannot tell the
-- two kinds of row apart; the trigger can, and does.
--
-- TRUNCATE is revoked all the same: it takes published rows with it and no
-- row-level mechanism can see it coming.
REVOKE TRUNCATE                       ON form_template_versions FROM app_tenant, PUBLIC;
GRANT  SELECT, INSERT, UPDATE, DELETE ON form_template_versions TO   app_tenant;

-- The immutability itself.
--
-- It guards THREE columns and the third is the one that matters. Refusing
-- changes to `definition` and `version` is the obvious half; leaving
-- `published_at` unguarded leaves the door wide open, because a shelter would
-- clear it, edit the row freely as a draft, and publish again. Immutability
-- bypassed without ever touching a guarded column -- the same shape as the
-- cascading foreign key T-01-020 found, where the hole was never in the thing
-- being watched.
--
-- Nothing else on the row is worth updating, so the rule is simply "a published
-- row is frozen", which has no seams to get wrong.
--
-- A trigger is also the only layer that survives a role with BYPASSRLS, and on
-- Neon that is not hypothetical: `neon_superuser` carries it. The default
-- SQLSTATE of a plpgsql RAISE is P0001, deliberately distinct from the grant's
-- 42501 so a test can tell which layer fired.
CREATE FUNCTION form_template_versions_is_immutable_once_published() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    -- TRUNCATE first: it is a STATEMENT-level firing with no OLD row at all, so
    -- reading OLD.published_at below would fail rather than refuse.
    IF TG_OP = 'TRUNCATE' THEN
        RAISE EXCEPTION
            'form_template_versions cannot be truncated: it holds published versions '
            'that submissions still render against';
    END IF;

    IF OLD.published_at IS NOT NULL THEN
        RAISE EXCEPTION
            'form_template_versions % is published and immutable: % is refused. '
            'Publish a new version instead', OLD.id, TG_OP;
    END IF;

    -- The draft path, and it is NOT `RETURN NEW`. On a BEFORE DELETE trigger NEW
    -- is NULL, and a BEFORE row trigger that returns NULL CANCELS THE STATEMENT
    -- -- silently, with no error and zero rows affected. Deleting a draft would
    -- have looked like it worked and left the row in place.
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    RETURN NEW;
END;
$$;

-- BEFORE, not AFTER: an AFTER trigger fires once the row is already written, and
-- while the exception still rolls the statement back, the guard reads as
-- permission rather than prevention.
CREATE TRIGGER form_template_versions_freeze_published
    BEFORE UPDATE OR DELETE ON form_template_versions
    FOR EACH ROW EXECUTE FUNCTION form_template_versions_is_immutable_once_published();

-- Separately, because TRUNCATE removes every row without visiting any: USING and
-- WITH CHECK are never consulted and FOR EACH ROW has nothing to run against.
-- Only a statement-level trigger reaches it, and it cannot be selective -- which
-- is correct, since a truncate would take the published versions too.
CREATE TRIGGER form_template_versions_no_truncate
    BEFORE TRUNCATE ON form_template_versions
    FOR EACH STATEMENT EXECUTE FUNCTION form_template_versions_is_immutable_once_published();

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

-- Versions first: its foreign key RESTRICTs the templates it points at, so the
-- parent cannot go while a child still names it.
DROP TABLE form_template_versions;
DROP FUNCTION form_template_versions_is_immutable_once_published();
DROP TABLE form_templates;

-- +goose StatementEnd
