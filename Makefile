VERSION := 0.1+

PACKAGE   = github.com/kata-containers/ksm-throttler
BASE      = $(GOPATH)/src/$(PACKAGE)
PREFIX    = /usr
BIN_DIR   = $(PREFIX)/bin
INPUT_DIR = $(GOPATH)/src/$(PACKAGE)/input
GO        = go
PKGS      = $(or $(PKG),$(shell cd $(BASE) && env GOPATH=$(GOPATH) $(GO) list ./... | grep -v "/vendor/"))

DESCRIBE := $(shell git describe 2> /dev/null || true)
DESCRIBE_DIRTY := $(if $(shell git status --porcelain --untracked-files=no 2> /dev/null),${DESCRIBE}-dirty,${DESCRIBE})
ifneq ($(DESCRIBE_DIRTY),)
VERSION := $(DESCRIBE_DIRTY)
endif

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
	$(QUIET_GOBUILD)go build $(PKGS)

throttler:
	$(QUIET_GOBUILD)go build -o ksm-throttler -ldflags "-X main.Version=$(VERSION)" throttler.go ksm.go

kicker:
	$(QUIET_GOBUILD)go build -o $(INPUT_DIR)/kicker/$@ $(INPUT_DIR)/kicker/*.go

binaries: throttler kicker

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
	rm -f $(INPUT_DIR)/kicker/kicker

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
