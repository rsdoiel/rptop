#!/bin/bash

#
# Release script for rptop on GitHub using the gh cli.
#
# Usage: release.bash [--dry-run]
#   --dry-run  do everything up to the prompt (notes, checksums), say what
#              would be committed, pushed and uploaded, and change nothing.
#
# Run `make release` first: it regenerates version.go and the other generated files,
# runs `make check-release`, and builds dist/*.zip. This script pushes, makes a
# draft release on GitHub and uploads the archives; you finalize the draft, and
# GitHub makes the tag when you do.
#
DRY_RUN=0
for ARG in "$@"; do
    case "${ARG}" in
        -n|--dry-run) DRY_RUN=1 ;;
        -h|--help) sed -n '2,12p' "$0" | grep '^#'; exit 0 ;;
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

echo "tag: ${RELEASE_TAG}, notes:"
trap 'rm -f release_notes.tmp' EXIT # gone on every way out, including a failed gh call
jq -r .releaseNotes codemeta.json >release_notes.tmp
cat release_notes.tmp

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
    rm -f release_notes.tmp
    exit 1
fi
if [ "${DRY_RUN}" = "1" ]; then
    echo "dry run: would commit (if the tree is dirty) and push, then create draft ${RELEASE_TAG} and upload:"
    git status --short
    ls dist/*.zip "dist/${CHECKSUM_FILE}"
    rm -f release_notes.tmp
    exit 0
fi
# shellcheck disable=SC2162
read -r -p "Push release to GitHub with gh? (y/N) " YES_NO
if [ "${YES_NO}" = "y" ]; then
    if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
        git commit -am "prep for ${RELEASE_TAG}" || exit 1
    fi
    git push origin "$(git rev-parse --abbrev-ref HEAD)" || exit 1
    echo "Pushing release ${RELEASE_TAG} to GitHub"
    gh release create "${RELEASE_TAG}" \
        --draft \
        -F release_notes.tmp \
        --generate-notes || exit 1
    echo "Uploading distribution files and checksums"
    gh release upload "${RELEASE_TAG}" "dist/${CHECKSUM_FILE}" || exit 1
    for FILE in dist/*.zip; do
        echo "Uploading ${FILE}"
        gh release upload "${RELEASE_TAG}" "${FILE}" || exit 1
    done
    rm -f release_notes.tmp
    cat <<EOT

Now goto repo release and finalize draft.

	${REPO_URL}/releases

EOT
fi
