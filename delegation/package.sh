#!/bin/sh
# Assemble the Stage 5 certification package.
#   delegation/package.sh <sample-run-id>...
# Copies the delegation source, eval results, the named sample runs, and a git
# log export into certification/stage5/edward-liang-stage5/, then zips it.
set -eu
cd "$(git rev-parse --show-toplevel)"
[ $# -ge 1 ] || { echo "usage: $0 <sample-run-id>..." >&2; exit 2; }

out=certification/stage5/edward-liang-stage5
[ ! -e "$out" ] || { echo "$out exists; move it aside first" >&2; exit 1; }
mkdir -p "$out/source" "$out/evidence/runs" "$out/evidence/evals"

git ls-files --cached --others --exclude-standard delegation go.mod go.sum |
	while read -r f; do mkdir -p "$out/source/$(dirname "$f")"; cp "$f" "$out/source/$f"; done

for w in delegation/evals/*/results.json; do
	cp "$w" "$out/evidence/evals/$(basename "$(dirname "$w")").json"
done

for id in "$@"; do
	provider=$(sed -n 's/.*"provider": *"\([^"]*\)".*/\1/p' "delegation/runs/$id/run.json" | head -1)
	case $provider in anthropic-api*) api=1 ;; *) api= ;; esac
	if [ -z "$api" ] && [ "${ALLOW_NON_API:-}" != 1 ]; then
		echo "$id was produced by '$provider', not anthropic-api; sample runs must come from delegate run" >&2
		rm -r "$out"
		exit 1
	fi
	go run ./delegation/cmd/delegate verify "$id" > /dev/null
	cp -R "delegation/runs/$id" "$out/evidence/runs/$id"
done

# Index the sample runs so the README can describe them without run IDs.
index="$out/evidence/runs/INDEX.md"
{
	echo "| Run | Task | Provider | Workflows | System status | Review rounds | Rework launches | Human decisions | Status now |"
	echo "|---|---|---|---|---|---|---|---|---|"
	for id in "$@"; do
		d="delegation/runs/$id"
		field() { sed -n "s/.*\"$1\": *\"\([^\"]*\)\".*/\1/p" "$d/run.json" | head -1; }
		rounds=$(ls "$d/artifacts/adversarial-review" 2>/dev/null | grep -c '^round-[0-9]*\.json$' || true)
		rework=$(grep '"kind":"tool_call"' "$d/audit.jsonl" | grep -c '"prior_challenges"' || true)
		decisions=$(sed -n 's/.*"decision":"\([^"]*\)".*/\1/p' "$d/decisions.jsonl" 2>/dev/null | paste -sd, - || true)
		now=$(sed -n 's/.*"to_status":"\([^"]*\)".*/\1/p' "$d/decisions.jsonl" 2>/dev/null | tail -1)
		echo "| \`$id\` | $(field task_id) | $(field provider) | \`$(field workflows)\` | $(field status) | $rounds | $rework | ${decisions:-none} | ${now:-$(field status)} |"
	done
} > "$index"
for rec in delegation/runs/*.exchanges.jsonl; do
	[ -e "$rec" ] || continue
	mkdir -p "$out/evidence/recordings"
	cp "$rec" "$out/evidence/recordings/"
done

signers=$(for id in "$@"; do sed -n 's/.*"signer":"\([0-9A-F]*\)".*/\1/p' "delegation/runs/$id/decisions.jsonl" 2>/dev/null; done | sort -u)
[ -z "$signers" ] || gpg --armor --export $signers > "$out/evidence/reviewer-keys.asc"

git log --stat --date=iso -- delegation go.mod > "$out/evidence/git-log.txt"
cp certification/stage5/README.md "$out/README.md"
(cd "$out/source" && go test ./delegation/... > ../evidence/go-test.txt 2>&1) || { echo "tests failed; see $out/evidence/go-test.txt" >&2; exit 1; }
(cd certification/stage5 && zip -qr edward-liang-stage5.zip edward-liang-stage5)
echo "wrote certification/stage5/edward-liang-stage5.zip"
