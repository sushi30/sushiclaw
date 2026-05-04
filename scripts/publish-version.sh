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

list_release_suffixes() {
	local prefix="$1"

	{
		git for-each-ref --format='%(refname:strip=2)' "refs/tags/${prefix}.*" 2>/dev/null || true
		git ls-remote --tags --refs origin "refs/tags/${prefix}.*" 2>/dev/null | awk -F/ '{print $3}' || true
	} | awk -F. -v prefix="${prefix}" '
		$1 "." $2 == prefix && $3 ~ /^[0-9]+$/ {
			if ($3 > max) max = $3
			found = 1
		}
		END { print found ? max + 1 : 0 }
	'
}

tag_exists() {
	local tag="$1"
	local status

	if git show-ref --tags --quiet --verify "refs/tags/${tag}"; then
		return 0
	fi

	git ls-remote --exit-code --tags --refs origin "refs/tags/${tag}" >/dev/null 2>&1
	status=$?

	if [[ "${status}" -eq 0 ]]; then
		return 0
	fi

	case "${status}" in
		2)
			return 1
			;;
		*)
			echo "ERROR: unable to verify whether ${tag} already exists on origin." >&2
			exit 1
			;;
	esac
}

git fetch --tags --prune-tags origin

prefix="$(date -u +%Y.%m)"
remote_head="$(git rev-parse --short=8 origin/main)"
head="$(git rev-parse --short=8 HEAD)"

if [[ "${FORCE:-}" != "1" && "${head}" != "${remote_head}" ]]; then
	echo "ERROR: local HEAD (${head}) does not match origin/main (${remote_head})." >&2
	echo "Run with FORCE=1 to publish anyway." >&2
	exit 1
fi

if [[ "${FORCE:-}" == "1" && "${head}" != "${remote_head}" ]]; then
	echo "WARNING: FORCE=1 set; publishing even though local HEAD differs from origin/main." >&2
fi

echo "Current commit:   ${head}"
echo "origin/main:      ${remote_head}"

published=false
prompted=false

while [[ "${published}" != "true" ]]; do
	next_iter="$(list_release_suffixes "${prefix}")"
	version="${prefix}.${next_iter}"

	while tag_exists "${version}"; do
		next_iter=$((next_iter + 1))
		version="${prefix}.${next_iter}"
	done

	echo "Next release tag: ${version}"

	if [[ "${dry_run}" == true ]]; then
		echo "Dry run: would create annotated tag ${version} and push it to origin."
		echo "Command: git tag -a ${version} -m \"Release ${version}\""
		echo "Command: git push origin ${version}"
		exit 0
	fi

	if [[ "${prompted}" != "true" ]]; then
		printf "Publish %s from HEAD by pushing this tag to GitHub? [y/N] " "${version}"
		read -r answer

		case "${answer}" in
			y | Y | yes | YES) ;;
			*)
				echo "Cancelled."
				exit 1
				;;
		esac

		prompted=true
	fi

	git tag -a "${version}" -m "Release ${version}"

	if push_output="$(git push origin "${version}" 2>&1)"; then
		echo "Published ${version}."
		published=true
		break
	fi

	git tag -d "${version}" >/dev/null 2>&1 || true

	case "${push_output}" in
		*"would clobber existing tag"*|*"already exists"*)
			echo "NOTICE: ${version} already exists on origin; retrying with the next release number." >&2
			git fetch --tags --prune-tags origin
			;;
		*)
			printf '%s\n' "${push_output}" >&2
			exit 1
			;;
	esac
done
