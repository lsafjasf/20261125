#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

echo "== unit tests =="
go test ./... -count=1
go vet ./...

WORK="$(mktemp -d)"
trap 'chmod -R u+w "$WORK" 2>/dev/null || true' EXIT
echo "== fixture at $WORK =="
go run ./cmd/adlabel sample -out "$WORK/demo" >/dev/null
LIB="$WORK/demo"

snapshot_hashes() {
  (cd "$1" && find media -type f -print0 | sort -z | xargs -0 sha256sum)
}
snapshot_sidecars() {
  (cd "$1" && find metadata -type f -print0 | sort -z | xargs -0 sha256sum)
}

echo "== chain is read-only =="
BEFORE_SID="$(snapshot_sidecars "$LIB")"
go run ./cmd/adlabel chain -library "$LIB" -corrections "$LIB/batches/good.json" >/dev/null
test "$BEFORE_SID" = "$(snapshot_sidecars "$LIB")"

echo "== good batch applies and media hashes stay stable =="
MED_BEFORE="$(snapshot_hashes "$LIB")"
go run ./cmd/adlabel repair -library "$LIB" -corrections "$LIB/batches/good.json" \
  -report-json "$WORK/report.json" >/dev/null
test "$MED_BEFORE" = "$(snapshot_hashes "$LIB")"
grep -q '"status": "applied"' "$WORK/report.json"
grep -q '"hash_stable": true' "$WORK/report.json"

echo "== only allowed fields changed; protected fields preserved =="
python3 - "$LIB" <<'PY'
import json, os, sys
root = sys.argv[1]
for mid in ("ad-001", "ad-002", "ad-003", "ad-006"):
    doc = json.load(open(os.path.join(root, "metadata", mid + ".json")))
    assert doc["labels"]["language"]["value"].isalpha(), mid
    assert doc["labels"]["duration"]["value"].endswith("s"), mid
doc = json.load(open(os.path.join(root, "metadata/ad-001.json")))
assert doc["campaign_meta"]["campaign"] == "winter-launch"
assert doc["media_id"] == "ad-001"
print("protected fields intact")
PY

echo "== same-name multilingual variants disambiguated by id =="
python3 - "$LIB" <<'PY'
import json, os, sys
root = sys.argv[1]
ja = json.load(open(os.path.join(root, "metadata/ad-003.json")))
en = json.load(open(os.path.join(root, "metadata/ad-002.json")))
assert ja["labels"]["language"]["value"] == "ja"
assert en["labels"]["language"]["value"] == "en"
print("same-name variants isolated")
PY

echo "== bad batch rolls back and exits non-zero =="
BAD="$(mktemp -d -p "$WORK")"
cp -r "$LIB/." "$BAD/"
SID_BEFORE="$(snapshot_sidecars "$BAD")"
set +e
go run ./cmd/adlabel repair -library "$BAD" -corrections "$BAD/batches/bad.json" \
  -all-or-nothing >/dev/null
CODE=$?
set -e
test "$CODE" -eq 1
test "$SID_BEFORE" = "$(snapshot_sidecars "$BAD")"

echo "== read-only sidecar is never modified =="
RO="$(mktemp -d -p "$WORK")"
cp -r "$LIB/." "$RO/"
chmod 444 "$RO/metadata/ad-004.json"
RO_BEFORE="$(sha256sum "$RO/metadata/ad-004.json")"
set +e
go run ./cmd/adlabel repair -library "$RO" -corrections "$RO/batches/bad.json" >/dev/null
RO_CODE=$?
set -e
test "$RO_CODE" -eq 1
test "$RO_BEFORE" = "$(sha256sum "$RO/metadata/ad-004.json")"

echo "== malformed group rejected before any write =="
set +e
go run ./cmd/adlabel repair -library "$LIB" \
  -corrections "$LIB/batches/malformed-group.json" >/dev/null
MAL_CODE=$?
set -e
test "$MAL_CODE" -eq 1

echo
echo "ALL ACCEPTANCE CHECKS PASSED"
