#!/usr/bin/env bash

# A script that builds all of the common API types

set -eo pipefail

SCRIPTS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT_DIR="$SCRIPTS_DIR/.."

source "$SCRIPTS_DIR/lib/common.sh"
source "$SCRIPTS_DIR/lib/k8s.sh"

DEFAULT_TEMPLATES_DIR="$ROOT_DIR/templates"
TEMPLATES_DIR=${TEMPLATES_DIR:-$DEFAULT_TEMPLATES_DIR}

echo "Building common Kubernetes API objects"

common_config_output_dir=$ROOT_DIR/config

controller-gen paths=$ROOT_DIR/apis/... \
    crd object:headerFile=$TEMPLATES_DIR/boilerplate.txt \
    output:crd:artifacts:config=$common_config_output_dir/crd/bases

bases=$(find "$common_config_output_dir/crd/bases" -maxdepth 1 -type f | sed -e 's/.*\//  - bases\//')
cat <<EOF > $common_config_output_dir/crd/kustomization.yaml
# Code generated in runtime. DO NOT EDIT.

apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
$bases
EOF