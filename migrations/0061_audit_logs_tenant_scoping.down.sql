-- Reverses 0061. Dropping the column re-opens the cross-tenant read in #532, so
-- this exists to make the migration reversible in a deployment sense, not
-- because rolling back is safe. Roll the APPLICATION back with it.

BEGIN;

DROP INDEX IF EXISTS idx_audit_logs_tenant_action;
DROP INDEX IF EXISTS idx_audit_logs_tenant_user;
DROP INDEX IF EXISTS idx_audit_logs_tenant_timestamp;
DROP INDEX IF EXISTS idx_audit_logs_tenant_id;

-- The column is NOT dropped. It carries the only tenant attribution that exists
-- for every row written since 0061, and dropping it destroys that
-- irrecoverably — a down migration must not be the thing that loses audit data.
-- An operator who genuinely wants the column gone can drop it by hand, having
-- decided to.

COMMIT;
