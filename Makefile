PREFIX := /usr
BIN_DIR := $(PREFIX)/bin

#
# Pretty printing
#

V	      = @
Q	      = $(V:1=)
QUIET_GOBUILD = $(Q:@=@echo    '     GOBUILD  '$@;)

#
# Build
#

all: build binaries

build:
	$(QUIET_GOBUILD)go build $(go list ./... | grep -v /vendor/)

throttler:
	$(QUIET_GOBUILD)go build -o ksm-throttler throttler.go ksm.go

binaries: throttler

#
# Tests
#

check: check-go-static check-go-test

check-go-static:
	bash .ci/go-lint.sh

check-go-test:
	bash .ci/go-test.sh

#
# Clean
#

clean:
	rm -f ksm-throttler

.PHONY: \
	all \
	build \
	binaries \
	check \
	check-go-static \
	check-go-test \
	install \
	uninstall \
	clean
