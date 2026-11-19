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
# An interrupted run is resumed by running the script again (workspace DR-0013).
# If the draft for the tag exists, targets this commit and is still a draft,
# nothing is created again: each file in dist/ is skipped when the draft already
# holds it (same SHA-256), uploaded when missing, and uploaded again with --clobber
# when the draft holds a different or unfinished copy. A published release, or a
# draft for another commit, stops the script. Over SSH, run it under tmux or nohup
# so a dropped connection does not interrupt the upload.
#
DRY_RUN=0
for ARG in "$@"; do
    case "${ARG}" in
        -n|--dry-run) DRY_RUN=1 ;;
        -h|--help) sed -n '2,/^DRY_RUN/p' "$0" | grep '^#'; exit 0 ;;
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

# digest prints the SHA-256 of a file.
digest() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    else
        shasum -a 256 "$1" | cut -d' ' -f1
    fi
}

# A release that already exists is resumed only if it is a draft for this commit
# (DR-0013); anything else stops the run, so a second release is never created
# and a published one is never touched.
RESUME=0
VIEW=""
if VIEW="$(gh release view "${RELEASE_TAG}" --json isDraft,targetCommitish,assets 2>/dev/null)"; then
    if [ "$(printf '%s' "${VIEW}" | jq -r .isDraft)" != "true" ]; then
        echo "error: ${RELEASE_TAG} is not a draft on GitHub; it is published and is not touched"
        exit 1
    fi
    DRAFT_TARGET="$(printf '%s' "${VIEW}" | jq -r .targetCommitish)"
    if [ "${DRAFT_TARGET}" != "${HEAD_HASH}" ]; then
        echo "error: the draft for ${RELEASE_TAG} targets ${DRAFT_TARGET}, not HEAD ${HEAD_HASH}; delete the draft first"
        exit 1
    fi
    RESUME=1
fi

# plan_for FILE prints what to do with one file: upload, replace or skip.
plan_for() {
    if [ "${RESUME}" != "1" ]; then
        echo upload
        return
    fi
    printf '%s' "${VIEW}" | jq -r --arg n "$(basename "$1")" --arg d "sha256:$(digest "$1")" --argjson s "$(wc -c <"$1" | tr -d ' ')" '
        ([.assets[]? | select(.name == $n)] | .[0]) as $a
        | if $a == null then "upload"
          elif $a.state != "uploaded" then "replace"
          elif ($a.digest // "") != "" then (if $a.digest == $d then "skip" else "replace" end)
          elif $a.size == $s then "skip"
          else "replace" end'
}

# The checksums file first, then the zips.
PLAN=()
TODO_COUNT=0
FILE_COUNT=0
for FILE in "dist/${CHECKSUM_FILE}" dist/*.zip; do
    ACTION="$(plan_for "${FILE}")"
    PLAN+=("${ACTION} ${FILE}")
    FILE_COUNT=$((FILE_COUNT + 1))
    [ "${ACTION}" = "skip" ] || TODO_COUNT=$((TODO_COUNT + 1))
done

if [ "${RESUME}" = "1" ]; then
    echo "Resume: the draft ${RELEASE_TAG} exists and targets ${HEAD_HASH}; ${TODO_COUNT} of ${FILE_COUNT} files to upload"
else
    echo "New release: draft ${RELEASE_TAG} at ${HEAD_HASH}; ${FILE_COUNT} files to upload"
fi
for LINE in "${PLAN[@]}"; do
    echo "  ${LINE%% *}: ${LINE#* }"
done

if [ "${DRY_RUN}" = "1" ]; then
    echo "dry run: nothing changed"
    exit 0
fi
if [ "${RESUME}" = "1" ] && [ "${TODO_COUNT}" = "0" ]; then
    echo "Nothing to upload: the draft holds every file. Finalize it at ${REPO_URL}/releases"
    exit 0
fi
if [ "${RESUME}" = "1" ]; then
    PROMPT="Resume draft ${RELEASE_TAG} for commit ${HEAD_HASH:0:7} on GitHub with gh? (y/N) "
else
    PROMPT="Create draft ${RELEASE_TAG} for commit ${HEAD_HASH:0:7} on GitHub with gh? (y/N) "
fi
# shellcheck disable=SC2162
read -r -p "${PROMPT}" YES_NO
if [ "${YES_NO}" = "y" ]; then
    if [ "${RESUME}" != "1" ]; then
        echo "Creating draft release ${RELEASE_TAG}"
        gh release create "${RELEASE_TAG}" \
            --draft \
            --target "${HEAD_HASH}" \
            -F "${NOTES}" \
            --generate-notes || exit 1
    fi
    echo "Uploading distribution files and checksums"
    for LINE in "${PLAN[@]}"; do
        ACTION="${LINE%% *}"
        FILE="${LINE#* }"
        case "${ACTION}" in
            skip)
                echo "Skipped (already uploaded): ${FILE}"
                continue
                ;;
            replace) CLOBBER="--clobber" ;;
            *) CLOBBER="" ;;
        esac
        echo "Uploading ${FILE}"
        # shellcheck disable=SC2086
        if ! gh release upload "${RELEASE_TAG}" "${FILE}" ${CLOBBER}; then
            echo "error: upload of ${FILE} failed; run release.bash again to resume"
            exit 1
        fi
    done
    cat <<EOT

Now goto repo release and finalize draft. Push nothing until you have.

	${REPO_URL}/releases

EOT
fi
