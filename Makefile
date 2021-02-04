SHELL := /bin/bash # Use bash syntax

# Set up variables
GO111MODULE=on

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

help:           ## Show this help.
	@grep -F -h "##" $(MAKEFILE_LIST) | grep -F -v grep | sed -e 's/\\$$//' \
		| awk -F'[:#]' '{print $$1 = sprintf("%-30s", $$1), $$4}'
