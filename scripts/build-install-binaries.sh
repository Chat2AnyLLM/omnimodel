#!/usr/bin/env bash
set -u

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/.build/bin"
BIN_DIR="${HOME}/.local/bin"

mkdir -p "${OUT_DIR}" "${BIN_DIR}"

build_one() {
  local pkg="$1"
  local out="$2"
  local output_path="${OUT_DIR}/${out}"
  local install_path="${BIN_DIR}/${out}"

  echo "Building ${out}..."
  if go build -o "${output_path}" "${pkg}"; then
    if cp -f "${output_path}" "${install_path}"; then
      printf '%s\t%s\t%s\n' "${out}" "built+installed" "${install_path}"
    else
      printf '%s\t%s\t%s\n' "${out}" "failed" "copy failed to ${install_path}" >&2
    fi
  else
    printf '%s\t%s\t%s\n' "${out}" "failed" "go build failed" >&2
  fi
}

results=()
results+=("$(build_one "${ROOT_DIR}" "omnillm")")
results+=("$(build_one "${ROOT_DIR}/cmd/omniproxy" "omniproxy")")
results+=("$(build_one "${ROOT_DIR}/cmd/omnicode" "omnicode")")

echo "Results:"
for row in "${results[@]}"; do
  [ -n "${row}" ] && printf '%b\n' "${row}"
done
