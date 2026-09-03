#!/usr/bin/env bash
# CI guard: fail if the ecosystem's shared kernel (eagle) is pinned
# to more than one version across the sibling Go modules.
#
# The expected version and the list of modules that must track it are DATA, read
# from ecosystem.yaml — not hardcoded here. Bump the manifest,
# not this script. (Parsed with grep/sed/awk on purpose: no yq dependency.)
#
# Skew across a breaking kernel change is otherwise invisible: release-please
# never rewrites a downstream go.mod, and a go.work workspace can mask the pins
# entirely. This guard turns that divergence into a red build.
#
# Exit codes: 0 = all agree (or siblings not checked out), 1 = skew / misconfig.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ECO_DIR="$(cd "${ROOT_DIR}/.." && pwd)"
SELF_REPO="$(basename "${ROOT_DIR}")"
MANIFEST="${ROOT_DIR}/ecosystem.yaml"

# Colour only on an interactive terminal; CI logs and NO_COLOR stay plain.
if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_OFF=$'\033[0m'
else
  C_RED=''; C_GREEN=''; C_YELLOW=''; C_OFF=''
fi

if [[ ! -f "${MANIFEST}" ]]; then
  echo "ERROR: manifest not found: ${MANIFEST}" >&2
  exit 1
fi

# --- manifest parsing -------------------------------------------------------

yaml_scalar() {
  # yaml_scalar <key> — first "key: value" at any indentation, comments stripped.
  sed -n "s/^[[:space:]]*$1:[[:space:]]*\([^#]*\).*/\1/p" "${MANIFEST}" |
    head -1 | sed 's/[[:space:]]*$//'
}

yaml_list() {
  # yaml_list <top-level key> — the "- item" entries under that key.
  awk -v key="$1" '
    $0 ~ "^" key ":[[:space:]]*(#.*)?$" { inblock = 1; next }
    inblock && /^[^[:space:]#]/         { inblock = 0 }
    inblock && /^[[:space:]]*-[[:space:]]*/ {
      line = $0
      sub(/^[[:space:]]*-[[:space:]]*/, "", line)
      sub(/[[:space:]]*#.*$/, "", line)
      gsub(/[[:space:]]/, "", line)
      if (line != "") print line
    }
  ' "${MANIFEST}"
}

MODULE_PATH="github.com/GrayCodeAI/eagle"
WANT_VERSION="$(yaml_scalar eagle_version)"
SOURCE_REPO="eagle"
MODULES=()
while IFS= read -r _m; do
  [[ -n "${_m}" ]] && MODULES+=("${_m}")
done < <("${ROOT_DIR}/scripts/ecosystem-manifest.sh" list eagle-consumers)

if [[ -z "${MODULE_PATH}" || -z "${WANT_VERSION}" || ${#MODULES[@]} -eq 0 ]]; then
  echo "ERROR: ${MANIFEST} is missing contracts.module, contracts.version or modules[]" >&2
  exit 1
fi

# --- go.mod inspection ------------------------------------------------------

repo_dir() {
  # The repo holding this script may be checked out under any directory name.
  if [[ "$1" == "${SELF_REPO}" || "$1" == "graycode-cli" && ! -d "${ECO_DIR}/graycode-cli" ]]; then
    echo "${ROOT_DIR}"
  else
    echo "${ECO_DIR}/$1"
  fi
}

mod_version() {
  # Print the version this go.mod requires for MODULE_PATH ("" if none).
  # Skips replace/exclude directives, single-line and block form alike.
  awk -v mod="${MODULE_PATH}" '
    /^(replace|exclude)[[:space:]]*\($/ { skip = 1; next }
    skip && /^\)/                       { skip = 0; next }
    skip                                { next }
    $1 == "replace" || $1 == "exclude"  { next }
    {
      for (i = 1; i <= NF; i++) {
        if ($i == mod && (i + 1) <= NF && $(i + 1) ~ /^v[0-9]/) { print $(i + 1); exit }
      }
    }
  ' "$1"
}

# --- checks -----------------------------------------------------------------

echo "shared kernel: ${MODULE_PATH}"
echo "declared version (ecosystem.yaml): ${WANT_VERSION}"
echo

checked=0
mismatched=()
missing_require=()
undeclared=()
versions_seen=""

printf '%-24s %-12s %s\n' MODULE VERSION STATUS
for name in "${MODULES[@]}"; do
  dir="$(repo_dir "${name}")"
  if [[ ! -f "${dir}/go.mod" ]]; then
    printf '%-24s %-12s %s\n' "${name}" - NOT_CHECKED_OUT
    continue
  fi

  checked=$((checked + 1))
  got="$(mod_version "${dir}/go.mod")"

  if [[ -z "${got}" ]]; then
    printf '%-24s %-12s %s%s%s\n' "${name}" - "${C_RED}" MISSING_REQUIRE "${C_OFF}"
    missing_require+=("${name}")
    continue
  fi

  versions_seen+="${got}"$'\n'
  if [[ "${got}" == "${WANT_VERSION}" ]]; then
    printf '%-24s %-12s %s%s%s\n' "${name}" "${got}" "${C_GREEN}" OK "${C_OFF}"
  else
    printf '%-24s %-12s %s%s%s\n' "${name}" "${got}" "${C_RED}" MISMATCH "${C_OFF}"
    mismatched+=("${name}=${got}")
  fi
done

# A sibling that requires the kernel but is not declared is skew waiting to happen.
while IFS= read -r gomod; do
  [[ -f "${gomod}" ]] || continue
  name="$(basename "$(dirname "${gomod}")")"
  [[ "${name}" == "${SOURCE_REPO}" ]] && continue
  for declared in "${MODULES[@]}"; do
    [[ "${name}" == "${declared}" ]] && continue 2
  done
  [[ -n "$(mod_version "${gomod}")" ]] || continue
  printf '%-24s %-12s %s%s%s\n' "${name}" "$(mod_version "${gomod}")" "${C_RED}" UNDECLARED "${C_OFF}"
  undeclared+=("${name}")
done < <(find "${ECO_DIR}" -mindepth 2 -maxdepth 2 -name go.mod 2>/dev/null | sort)

echo

if ((checked == 0)); then
  echo "${C_YELLOW}NOTICE${C_OFF}: no sibling modules are checked out next to ${ROOT_DIR} — nothing to compare."
  echo "contracts parity guard skipped (this is expected in a single-repo CI checkout)."
  exit 0
fi

# Advisory only: VERSION declares the intended source release; a matching remote
# tag is checked separately by release-parity.
src_dir="$(repo_dir "${SOURCE_REPO}")"
if [[ -f "${src_dir}/VERSION" ]]; then
  source_version="v$(tr -d '[:space:]' < "${src_dir}/VERSION" | sed 's/^v//')"
  if [[ "${source_version}" != "${WANT_VERSION}" ]]; then
    echo "${C_YELLOW}NOTICE${C_OFF}: ${SOURCE_REPO} source declares ${source_version}; consumers pin ${WANT_VERSION}."
    echo "        Expected while a reachable pseudo-version bridges the next ordered semver release."
    echo
  fi
fi

if ((${#mismatched[@]} == 0 && ${#missing_require[@]} == 0 && ${#undeclared[@]} == 0)); then
  echo "${C_GREEN}OK${C_OFF}: contracts parity guard passed — ${checked} module(s) all pinned to ${MODULE_PATH} ${WANT_VERSION}."
  exit 0
fi

echo "${C_RED}FAIL${C_OFF}: shared-kernel version skew detected."
echo

if ((${#mismatched[@]} > 0)); then
  echo "modules disagreeing with the declared version ${WANT_VERSION}:"
  for entry in "${mismatched[@]}"; do
    echo "  - ${entry%%=*} pins ${entry#*=}"
  done
  echo
  echo "distinct versions in use:"
  printf '%s' "${versions_seen}" | sort -u | sed 's/^/  - /'
  echo
  echo "to fix, in each disagreeing repo:"
  for entry in "${mismatched[@]}"; do
    echo "  cd $(repo_dir "${entry%%=*}") && \\"
    echo "    GOWORK=off go get ${MODULE_PATH}@${WANT_VERSION} && GOWORK=off go mod tidy"
  done
  echo
  echo "if the DECLARED version is the stale one, bump contracts.version in"
  echo "  ${MANIFEST}"
  echo "and re-run this guard."
  echo
fi

if ((${#missing_require[@]} > 0)); then
  echo "modules declared in the manifest but no longer requiring ${MODULE_PATH}:"
  printf '  - %s\n' "${missing_require[@]}"
  echo "  add the dependency back, or drop the module from modules[] in ${MANIFEST}."
  echo
fi

if ((${#undeclared[@]} > 0)); then
  echo "modules requiring ${MODULE_PATH} without being declared in the manifest:"
  printf '  - %s\n' "${undeclared[@]}"
  echo "  add each to modules[] in ${MANIFEST} so it is held to the declared version."
  echo
fi

exit 1
