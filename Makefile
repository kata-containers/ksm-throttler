PACKAGE       = github.com/kata-containers/ksm-throttler
BASE          = $(GOPATH)/src/$(PACKAGE)
PREFIX        = /usr
BIN_DIR       = $(PREFIX)/bin
LIBEXECDIR    = $(PREFIX)/libexec
LOCALSTATEDIR = /var
SOURCES       = $(shell find . 2>&1 | grep -E '.*\.(c|h|go)$$')
KSM_SOCKET    = $(LOCALSTATEDIR)/run/ksm-throttler/ksm.sock
TRIGGER_DIR   = $(GOPATH)/src/$(PACKAGE)/trigger
GO            = go
PKGS          = $(or $(PKG),$(shell cd $(BASE) && env GOPATH=$(GOPATH) $(GO) list ./... | grep -v "/vendor/"))

VERSION_FILE := ./VERSION
VERSION := $(shell grep -v ^\# $(VERSION_FILE))
COMMIT_NO := $(shell git rev-parse HEAD 2> /dev/null || true)
COMMIT := $(if $(shell git status --porcelain --untracked-files=no),${COMMIT_NO}-dirty,${COMMIT_NO})
VERSION_COMMIT := $(if $(COMMIT),$(VERSION)-$(COMMIT),$(VERSION))

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
	$(QUIET_GOBUILD)go build -o ksm-throttler -ldflags \
		"-X main.DefaultURI=$(KSM_SOCKET) -X main.version=$(VERSION_COMMIT)" throttler.go ksm.go

kicker:
	$(QUIET_GOBUILD)go build -o $(TRIGGER_DIR)/kicker/$@ \
		-ldflags "-X main.DefaultURI=$(KSM_SOCKET)" $(TRIGGER_DIR)/kicker/*.go

virtcontainers:
	$(QUIET_GOBUILD)go build -o $(TRIGGER_DIR)/virtcontainers/vc \
		-ldflags "-X main.DefaultURI=$(KSM_SOCKET)" $(TRIGGER_DIR)/virtcontainers/*.go

binaries: throttler kicker virtcontainers

#
# systemd files
#

HAVE_SYSTEMD := $(shell pkg-config --exists systemd 2>/dev/null && echo 'yes')

ifeq ($(HAVE_SYSTEMD),yes)
UNIT_DIR := $(shell pkg-config --variable=systemdsystemunitdir systemd)
UNIT_FILES = ksm-throttler.service vc-throttler.service
GENERATED_FILES += $(UNIT_FILES)
endif

#
# Tests
#

check: check-go-static check-go-test

check-go-static:
	bash .ci/go-lint.sh

check-go-test:
	bash .ci/go-test.sh

#
# install
#

define INSTALL_EXEC
	$(QUIET_INST)install -D $1 $(DESTDIR)$2/$1 || exit 1;

endef
define INSTALL_FILE
	$(QUIET_INST)install -D -m 644 $1 $(DESTDIR)$2/$1 || exit 1;

endef

all-installable: ksm-throttler virtcontainers $(UNIT_FILES)

install: all-installable
	$(call INSTALL_EXEC,ksm-throttler,$(LIBEXECDIR)/ksm-throttler)
	$(call INSTALL_EXEC,trigger/virtcontainers/vc,$(LIBEXECDIR)/ksm-throttler)
	$(foreach f,$(UNIT_FILES),$(call INSTALL_FILE,$f,$(UNIT_DIR)))

#
# Clean
#

clean:
	rm -f ksm-throttler
	rm -f $(TRIGGER_DIR)/kicker/kicker
	rm -f $(TRIGGER_DIR)/virtcontainers/vc

$(GENERATED_FILES): %: %.in Makefile
	@mkdir -p `dirname $@`
	$(QUIET_GEN)sed \
		-e 's|[@]bindir[@]|$(BINDIR)|g' \
		-e 's|[@]libexecdir[@]|$(LIBEXECDIR)|' \
		-e "s|[@]localstatedir[@]|$(LOCALSTATEDIR)|" \
		"$<" > "$@"

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
