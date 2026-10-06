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
	if [ "$provider" != "anthropic-api" ] && [ "${ALLOW_NON_API:-}" != 1 ]; then
		echo "$id was produced by '$provider', not anthropic-api; sample runs must come from delegate run" >&2
		rm -r "$out"
		exit 1
	fi
	go run ./delegation/cmd/delegate verify "$id" > /dev/null
	cp -R "delegation/runs/$id" "$out/evidence/runs/$id"
done
for rec in delegation/runs/*.exchanges.jsonl; do
	[ -e "$rec" ] || continue
	mkdir -p "$out/evidence/recordings"
	cp "$rec" "$out/evidence/recordings/"
done

git log --stat --date=iso -- delegation go.mod > "$out/evidence/git-log.txt"
cp delegation/README.md "$out/README.md"
(cd "$out/source" && go test ./delegation/... > ../evidence/go-test.txt 2>&1) || { echo "tests failed; see $out/evidence/go-test.txt" >&2; exit 1; }
(cd certification/stage5 && zip -qr edward-liang-stage5.zip edward-liang-stage5)
echo "wrote certification/stage5/edward-liang-stage5.zip"
