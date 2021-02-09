#!/usr/bin/env bash

CONTROLLER_TOOLS_VERSION="v0.4.0"

THIS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT_DIR="$THIS_DIR/../.."
SCRIPTS_DIR="$ROOT_DIR/scripts"

# controller_gen_version_equals accepts a string version and returns 0 if the
# installed version of controller-gen matches the supplied version, otherwise
# returns 1
#
# Usage:
#
#   if controller_gen_version_equals "v0.4.0"; then
#       echo "controller-gen is at version 0.4.0"
#   fi
k8s_controller_gen_version_equals() {
    currentver="$(controller-gen --version | cut -d' ' -f2 | tr -d '\n')";
    requiredver="$1";
    if [ "$currentver" = "$requiredver" ]; then
        return 0
    else
        return 1
    fi;
}