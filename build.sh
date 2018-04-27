#!/bin/bash

set -u

readonly SCRIPT_PATH="$0"
readonly SCRIPT_NAME="$(basename $SCRIPT_PATH)"
readonly SCRIPT_DIR_REL="$(dirname $SCRIPT_PATH)"
readonly SCRIPT_DIR="$(cd -P -- "$SCRIPT_DIR_REL" && pwd)"

main() {
    local version=$(git tag | sort -V | tail -n 1)
    rm -rf "$SCRIPT_DIR/build"
    build darwin $version
    build linux $version
    build windows $version
}

build() {
    local os="$1"
    local version="$2"
    local filename="dyndnsd-${version}_${os}_amd64.zip"
    local destdir="${SCRIPT_DIR}/build/${os}"
    mkdir -p "${destdir}"
    (cd "$SCRIPT_DIR" && GOOS=${os} GOARCH=amd64 go build -o "${destdir}"/dyndnsd)
    (cd "${destdir}" && zip "../${filename}" dyndnsd)
    local shasum=$(cat "${SCRIPT_DIR}/build/${filename}" | shasum -a 256)
    echo "${filename} sha256=${shasum}"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi