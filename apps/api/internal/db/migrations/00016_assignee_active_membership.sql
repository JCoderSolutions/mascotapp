-- The database backstop for "a case can only be assigned to an ACTIVE member"
-- (P2-D7), closing the last carried-forward item of Phase 01.
--
-- Two layers of this schema disagreed by construction, and the disagreement was
-- pinned by a characterization test until today:
--
--     assignable = the membership row EXISTS
--     readable   = the membership row exists AND is ACTIVE
--
-- `adoption_applications` reaches `memberships` through the composite foreign
-- key `(assigned_to_user_id, shelter_id)`, which can check that the pair exists
-- and cannot check `status`. That is PostgreSQL, not an oversight: **a foreign
-- key must reference a NON-PARTIAL unique constraint**, verified on 17, so
-- `UNIQUE (user_id, shelter_id) WHERE status = 'active'` cannot be the
-- referenced key at all. Meanwhile `member_visible_users` DOES filter on
-- `m.status = 'active'` — it was given that filter in T-01-016, after Judgment
-- Day found that any membership row granted read access to a user's PII.
--
-- So a case could be assigned to a revoked or never-accepted member: it sat in a
-- queue owned by somebody whose name that shelter could no longer render. Not a
-- tenant leak — everyone involved belongs to this shelter — which is why the
-- database's answer was incomplete rather than wrong.
--
-- WHY A TRIGGER AND NOT A POLICY. ADR-0010's argument, unchanged: a trigger is
-- NOT bypassed by a `BYPASSRLS` role, and a policy is. The subquery below runs
-- as the invoking role, so under `app_tenant` it is filtered by `memberships`'
-- own tenant policy — the correct scope, not a limitation.
--
-- WHY `BEFORE` AND NOT A DEFERRABLE `CONSTRAINT TRIGGER`. Deferring matters only
-- if an invitation and an assignment land in one transaction; nothing plans
-- that, and a plain BEFORE trigger fails at the offending statement rather than
-- at COMMIT, which is much easier to attribute.
--
-- ERRCODE 23514 is chosen deliberately, not defaulted. A plpgsql RAISE defaults
-- to P0001, which this schema already uses for the append-only triggers; a
-- distinct code is what lets a test tell WHICH layer refused a statement. 23514
-- is `check_violation`, and that is what this is.
--
-- `TestAssignment_DoesNotYetRequireAnActiveMembership` pinned the absence of this
-- rule and is DELETED in the same commit, replaced by its inverse
-- `TestAssignment_RequiresAnActiveMembership`. Landing them apart would leave
-- the suite red between two commits with no code change to explain why.

-- +goose Up

-- +goose StatementBegin

CREATE FUNCTION assignee_must_be_active_member() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM memberships m
                   WHERE m.user_id = NEW.assigned_to_user_id
                     AND m.shelter_id = NEW.shelter_id
                     AND m.status = 'active') THEN
        RAISE EXCEPTION 'assignee % is not an active member of shelter %',
            NEW.assigned_to_user_id, NEW.shelter_id USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$;

-- `UPDATE OF assigned_to_user_id` narrows the trigger to the statements that
-- can actually break the rule, and the `WHEN` clause narrows it again.
--
-- The WHEN guard is load-bearing, not an optimisation, and mutation testing
-- showed it is worse than the obvious reading. `assigned_to_user_id = NULL`
-- matches no membership row, so without the guard the function raises on every
-- NULL assignee — which is not just "unassignment breaks". EVERY APPLICATION IS
-- CREATED UNASSIGNED, so without the guard no adoption application can be
-- created at all. Dropping `WHEN` kills four of this rule's five test cases,
-- most of them in their own setup.
--
-- INSERT is covered as well as UPDATE because an application can be created
-- ALREADY assigned — a trigger on UPDATE alone would leave the same hole
-- reachable through a different statement, and the two paths go through
-- different application code.
CREATE TRIGGER adoption_applications_assignee_active
    BEFORE INSERT OR UPDATE OF assigned_to_user_id ON adoption_applications
    FOR EACH ROW WHEN (NEW.assigned_to_user_id IS NOT NULL)
    EXECUTE FUNCTION assignee_must_be_active_member();

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

DROP TRIGGER adoption_applications_assignee_active ON adoption_applications;
DROP FUNCTION assignee_must_be_active_member();

-- +goose StatementEnd
