SHELL := /bin/bash # Use bash syntax

GOPATH ?= "$(HOME)/go"
GO111MODULE=on
K8S_APIMACHINERY_VERSION = $(shell go list -m -f '{{ .Version }}' k8s.io/apimachinery)
K8S_APIMACHINERY_DIR = "$(GOPATH)/pkg/mod/k8s.io/apimachinery@$(K8S_APIMACHINERY_VERSION)"

.PHONY: all test clean-mocks mocks

all: test

test: | mocks	## Run code tests
	go test ${GO_TAGS} ./...

clean-mocks:	## Remove mocks directory
	rm -rf mocks

install-mockery:
	@scripts/install-mockery.sh

mocks: install-mockery ## Build mocks
	@echo -n "building mocks for pkg/types ... "
	@bin/mockery --quiet --all --tags=codegen --case=underscore --output=mocks/pkg/types --dir=pkg/types
	@echo "ok."
	@echo -n "building mocks for k8s.io/apimachinery/pkg/apis/meta/v1 ... "
	@bin/mockery --quiet --name=Object --case=underscore --output=mocks/apimachinery/pkg/apis/meta/v1 --dir="$(K8S_APIMACHINERY_DIR)/pkg/apis/meta/v1"
	@echo "ok."
	@echo -n "building mocks for k8s.io/apimachinery/runtime ... "
	@bin/mockery --quiet --name=Object --case=underscore --output=mocks/apimachinery/pkg/runtime --dir="$(K8S_APIMACHINERY_DIR)/pkg/runtime"
	@echo "ok."
	@echo -n "building mocks for k8s.io/apimachinery/runtime/schema ... "
	@bin/mockery --quiet --name=ObjectKind --case=underscore --output=mocks/apimachinery/pkg/runtime/schema --dir="$(K8S_APIMACHINERY_DIR)/pkg/runtime/schema"
	@echo "ok."

help:           ## Show this help.
	@grep -F -h "##" $(MAKEFILE_LIST) | grep -F -v grep | sed -e 's/\\$$//' \
		| awk -F'[:#]' '{print $$1 = sprintf("%-30s", $$1), $$4}'
