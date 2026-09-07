-- #532 — audit_logs gains the tenant column it never had.
--
-- THE DEFECT. GET /api/v1/audit-logs returned the authentication and
-- authorization log of EVERY tenant in the deployment to any organisation
-- administrator. Not a lost predicate — there was nothing to lose: the table had
-- no tenant column, so the query filtered on the timestamp alone. Its two
-- siblings were the same shape (user_id alone, action alone). The adminRole
-- guard in front of them asks whether the caller administers their OWN
-- organisation, which every customer's administrator does; it gated the caller,
-- never the rows.
--
-- Same failure as the retired GET /timeline/recent (docs/JOURNAL.md item 36),
-- and for the same underlying reason: a journal table with no tenant column.
--
-- ON THE COLUMN ALREADY EXISTING. A pre-AutoMigrate legacy migration
-- (migrations/_archive/root-legacy-preautomigrate/0014_add_tenant_scoping.sql)
-- already added `tenant_id` to this table, with an FK to the old `tenants`
-- table. The Go model never declared it, so on those databases the column has
-- sat NULL and unread ever since, and on databases built by AutoMigrate it does
-- not exist at all. Both are handled: everything below is IF NOT EXISTS.
--
-- ON THE ID SPACE. tenant_id holds an organizations.id — the value the
-- `tenant_id` JWT claim carries (cmd/server/main.go resolveSession derives it
-- from users.default_org_id). It is NOT users.tenant_id, which points at the
-- separate legacy `tenants` table. No foreign key is declared, deliberately:
-- the legacy migration's FK pointed at `tenants`, which is the wrong table, and
-- the sibling auth_audit_logs.tenant_id carries no FK either. Adding one here
-- would have to choose a parent table on a schema where both exist, and would
-- make an audit row deletable by a cascade — an audit trail must outlive the
-- thing it audits.

BEGIN;

ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS tenant_id UUID;

-- Every read is `WHERE tenant_id = ? AND <something>`, so the tenant column
-- leads the composite indexes as well as carrying its own.
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_id
    ON audit_logs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_timestamp
    ON audit_logs (tenant_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_user
    ON audit_logs (tenant_id, user_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_action
    ON audit_logs (tenant_id, action, timestamp DESC);

COMMENT ON COLUMN audit_logs.tenant_id IS
    'Organisation (organizations.id) the audited action was performed in, taken '
    'from the acting session. NULL means the event could not be attributed to an '
    'organisation (a pre-authentication event where no user resolved). Reads '
    'filter on tenant_id = ?, which never matches NULL, so an unattributed row is '
    'invisible to every tenant rather than visible to all of them. See #532.';

-- NO BACKFILL, and that is a decision rather than an omission.
--
-- Historical rows carry only the ACTING user's id. The organisation an action
-- was performed in is the session's, not whatever that user's default
-- organisation happens to be today — a person can change organisation, and an
-- administrator can belong to several. Deriving one from the other would produce
-- an attribution that looks authoritative and is a guess, in the one table whose
-- entire value is that it is not guessed. A wrong attribution here is worse than
-- no attribution: it would show one organisation an action taken in another.
--
-- So pre-#532 rows stay NULL and are therefore invisible to every tenant in the
-- UI. They remain on disk, in order, readable by an operator with database
-- access, for as long as retention keeps them. Operators who need that history
-- attributed must do it from evidence they hold outside this table.
--
-- Count what this affects before deploying:
--   SELECT count(*) FROM audit_logs WHERE tenant_id IS NULL;

COMMIT;
