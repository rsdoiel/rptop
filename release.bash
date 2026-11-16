#!/bin/bash

#
# Release script for rptop on GitHub using the gh cli.
#
# Usage: release.bash [--dry-run]
#   --dry-run  do everything up to the prompt (notes, checksums), say what
#              would be uploaded, and change nothing.
#
# The order of a release: edit codemeta.json, `make generate`, commit and push,
# `make check-release`, `make release` (builds dist/*.zip at that commit),
# `./release.bash --dry-run`, `./release.bash`. This script commits nothing and
# pushes nothing: it requires a clean tree whose HEAD is on origin, makes a draft
# release on GitHub for exactly that commit, and uploads the archives. You finalize the
# draft, and GitHub makes the tag, on that commit, when you do.
#
DRY_RUN=0
for ARG in "$@"; do
    case "${ARG}" in
        -n|--dry-run) DRY_RUN=1 ;;
        -h|--help) sed -n '2,13p' "$0" | grep '^#'; exit 0 ;;
        *) echo "usage: release.bash [--dry-run]" >&2; exit 2 ;;
    esac
done

REPO_ID="${PWD##*/}"
GROUP_ID="$(git config --get remote.origin.url | sed -E 's#.*github\.com[:/]([^/]+)/.*#\1#')"
REPO_URL="https://github.com/${GROUP_ID}/${REPO_ID}"
echo "REPO_URL -> ${REPO_URL}"

#
# The tag comes from codemeta.json
#
RELEASE_TAG="v$(jq -r .version codemeta.json)"
if ! printf '%s' "${RELEASE_TAG}" | grep -qE '^v[0-9a-zA-Z._-]+$'; then
    echo "error: version contains unexpected characters: ${RELEASE_TAG}"
    exit 1
fi
if ! ls dist/*.zip >/dev/null 2>&1; then
    echo "error: no dist/*.zip; run make release first"
    exit 1
fi

#
# The archives must come from the commit the tag will land on: a clean tree, and HEAD
# is the head of the branch on origin (GitHub makes the tag at that head).
#
BRANCH="$(git rev-parse --abbrev-ref HEAD)"
HEAD_HASH="$(git rev-parse HEAD)"
if [ -n "$(git status --porcelain)" ]; then
    echo "error: uncommitted changes; run make generate, commit and push, then make release again"
    git status --short
    exit 1
fi
ORIGIN_HASH="$(git ls-remote origin "refs/heads/${BRANCH}" | cut -f1)"
if [ "${ORIGIN_HASH}" != "${HEAD_HASH}" ]; then
    echo "error: HEAD ${HEAD_HASH:0:7} is not origin/${BRANCH} (${ORIGIN_HASH:0:7}); push first, then make release again"
    exit 1
fi
echo "building from ${BRANCH} at ${HEAD_HASH:0:7}: clean, and on origin"

NOTES="dist/release_notes.tmp"
echo "tag: ${RELEASE_TAG}, notes:"
trap 'rm -f "${NOTES}"' EXIT # gone on every way out, including a failed gh call
jq -r .releaseNotes codemeta.json >"${NOTES}"
cat "${NOTES}"

#
# Generate checksums for the distribution zip files
#
CHECKSUM_FILE="${REPO_ID}-${RELEASE_TAG}-checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
    sha256sum dist/*.zip | sed 's|dist/||' > "dist/${CHECKSUM_FILE}"
else
    shasum -a 256 dist/*.zip | sed 's|dist/||' > "dist/${CHECKSUM_FILE}"
fi
echo "Checksums written to dist/${CHECKSUM_FILE}"

# A draft that already exists for the tag stops the run so a second one is never created.
if gh release view "${RELEASE_TAG}" >/dev/null 2>&1; then
    echo "error: a release for ${RELEASE_TAG} already exists on GitHub; delete the draft first"
    exit 1
fi
if [ "${DRY_RUN}" = "1" ]; then
    echo "dry run: would create draft ${RELEASE_TAG} for commit ${HEAD_HASH:0:7} and upload:"
    ls dist/*.zip "dist/${CHECKSUM_FILE}"
    exit 0
fi
# shellcheck disable=SC2162
read -r -p "Create draft ${RELEASE_TAG} for commit ${HEAD_HASH:0:7} on GitHub with gh? (y/N) " YES_NO
if [ "${YES_NO}" = "y" ]; then
    echo "Creating draft release ${RELEASE_TAG}"
    gh release create "${RELEASE_TAG}" \
        --draft \
        --target "${HEAD_HASH}" \
        -F "${NOTES}" \
        --generate-notes || exit 1
    echo "Uploading distribution files and checksums"
    gh release upload "${RELEASE_TAG}" "dist/${CHECKSUM_FILE}" || exit 1
    for FILE in dist/*.zip; do
        echo "Uploading ${FILE}"
        gh release upload "${RELEASE_TAG}" "${FILE}" || exit 1
    done
    cat <<EOT

Now goto repo release and finalize draft. Push nothing until you have.

	${REPO_URL}/releases

EOT
fi
