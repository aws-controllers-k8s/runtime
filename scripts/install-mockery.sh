#!/usr/bin/env bash

# A script that installs the mockery CLI tool that is used to build Go mocks
# for our interfaces to use in unit testing. This script installs mockery into
# the bin/mockery path and really should just be used in testing scripts.
#
# We build mockery from source to ensure it uses the same Go version as the
# project, which is required for parsing Go files with newer language features.

set -eo pipefail

SCRIPTS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT_DIR="$SCRIPTS_DIR/.."
BIN_DIR="$ROOT_DIR/bin"

VERSION=v2.53.3

if [[ ! -f $BIN_DIR/mockery ]]; then
    echo -n "Installing mockery into bin/mockery ... "
    mkdir -p $BIN_DIR
    GOBIN="$BIN_DIR" go install github.com/vektra/mockery/v2@${VERSION}
    echo "ok."
fi
