#!/usr/bin/env bash
set -euo pipefail

dry_run=false

case "${1:-}" in
	--dry-run)
		dry_run=true
		;;
	"")
		;;
	*)
		echo "Usage: $0 [--dry-run]" >&2
		exit 2
		;;
esac

git fetch --tags --prune-tags origin

prefix="$(date -u +%Y.%m)"
next_iter="$(
	git ls-remote --tags --refs origin "refs/tags/${prefix}.*" |
		awk -F/ '{print $3}' |
		awk -F. -v prefix="${prefix}" '$1 "." $2 == prefix && $3 ~ /^[0-9]+$/ { if ($3 > max) max = $3; found = 1 } END { print found ? max + 1 : 0 }'
)"
version="${prefix}.${next_iter}"
remote_head="$(git rev-parse --short=8 origin/main)"
head="$(git rev-parse --short=8 HEAD)"

echo "Next release tag: ${version}"
echo "Current commit:   ${head}"
echo "origin/main:      ${remote_head}"

if [[ "${dry_run}" == true ]]; then
	echo "Dry run: would create annotated tag ${version} and push it to origin."
	echo "Command: git tag -a ${version} -m \"Release ${version}\""
	echo "Command: git push origin ${version}"
	exit 0
fi

printf "Publish %s from HEAD by pushing this tag to GitHub? [y/N] " "${version}"
read -r answer

case "${answer}" in
	y | Y | yes | YES) ;;
	*)
		echo "Cancelled."
		exit 1
		;;
esac

git tag -a "${version}" -m "Release ${version}"
git push origin "${version}"
echo "Published ${version}."
