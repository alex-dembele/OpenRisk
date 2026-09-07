#!/usr/bin/env bash
# PostToolUse(Edit|Write): format, then trip the tenant-isolation wire.
# Exit 2 returns the message to the agent as a correction it must act on.
set -uo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
. "$DIR/_json.sh"

INPUT=$(cat)
FILE=$(json_get "$INPUT" '.tool_input.file_path')
[ -z "$FILE" ] && exit 0
[ -f "$FILE" ] || exit 0

case "$FILE" in
  *.go)
    command -v gofmt     >/dev/null 2>&1 && gofmt -s -w "$FILE"
    command -v goimports >/dev/null 2>&1 && goimports -w "$FILE"
    # Tenant-isolation tripwire on persistence code (ABSOLUTE RULE 2).
    case "$FILE" in
      */infrastructure/*|*repository*|*repo.go)
        # A .Where() with no tenant_id is reported UNLESS the line above it is a
        # comment. That escape hatch is what the message below has always
        # promised; before it existed, a file holding one legitimately
        # tenant-agnostic query — a chained builder whose base already filters
        # by tenant, or a child table gated through its parent — could never be
        # edited again, because the wire re-tripped on lines nobody had touched.
        #
        # A comment is a deliberate act and it shows up in review, which is the
        # point: the wire's job is to make an unfiltered query impossible to add
        # by accident, not to make it impossible to explain.
        UNJUSTIFIED=$(awk '
          /\.Where\(/ && !/tenant_id/ {
            if (prev !~ /^[[:space:]]*\/\//) printf "%d:%s\n", FNR, $0
          }
          /[^[:space:]]/ { prev = $0 }
        ' "$FILE")

        if [ -n "$UNJUSTIFIED" ]; then
          {
            echo "TENANT TRIPWIRE — $FILE"
            echo "A .Where() clause with no tenant_id, and no comment explaining it:"
            printf '%s\n' "$UNJUSTIFIED" | head -5
            echo ""
            echo "ABSOLUTE RULE 2: every DB query filters by tenant_id."
            echo "If this table has no tenant column, gate it through the parent"
            echo "entity with an ownsX helper and say so explicitly in your report."
            echo "If this query is genuinely tenant-agnostic (migrations, health"
            echo "checks), or is chained onto a base query that already filters by"
            echo "tenant, add a one-line comment saying why on the line above."
          } >&2
          exit 2
        fi ;;
    esac ;;
  *.ts|*.tsx)
    if [ -f package.json ] && command -v npx >/dev/null 2>&1; then
      npx --no-install prettier --write "$FILE" >/dev/null 2>&1 || true
    fi ;;
esac
exit 0
