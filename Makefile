export GO15VENDOREXPERIMENT=1

exe = github.com/krameff/syver/cmd/syver
pkgs = $(shell ./novendor.sh)
cmd = syver
GO111MODULE=on
GO_FILES = $(shell git ls-files -- '*.go' ':!:*vendor*_test.go')
VENV := $(shell echo $${VIRTUAL_ENV-.venv})
PYTHON := $(VENV)/bin/python
DOCS_DEPS := $(VENV)/.docs.dependencies

.PHONY: all build install test release bench fmt lint vet test-int-all gen centos7

all: test-short-all test-int-all dgoss-sha256 dcgoss-sha256 kgoss-sha256 dsyver-sha256 dcsyver-sha256 ksyver-sha256

test-short-all: fmt lint vet test

install: release/syver-linux-amd64
	$(info INFO: Starting build $@)
	cp release/$(cmd)-linux-amd64 $(GOPATH)/bin/syver

test:
	$(info INFO: Starting build $@)
	./ci/go-test.sh

cov:
	go test -coverpkg=./... -coverprofile=c.out ./...
	# go tool cover -func ./c.out

funcov:
	go test -coverpkg=./... -coverprofile=c.out ./...
	go tool cover -func ./c.out

htmlcov:
	go test -v -coverpkg=./... -coverprofile=c.out ./...
	go tool cover -html ./c.out

lint:
	$(info INFO: Starting build $@)
	golangci-lint run --timeout 5m $(pkgs)

vet:
	$(info INFO: Starting build $@)
	go vet $(pkgs)

fmt:
	$(info INFO: Starting build $@)
	./ci/go-fmt.sh

bench:
	$(info INFO: Starting build $@)
	go test -bench=.

test-int-validate-%: release/syver-%
	$(info INFO: Starting build $@)
	./integration-tests/run-validate-tests.sh $*

test-int-serve-%: release/syver-%
	$(info INFO: Starting build $@)
	./integration-tests/run-serve-tests.sh $*

release/syver-%: $(GO_FILES)
	./release-build.sh -p $* -v $(or $(RELEASE_TAG),$(shell git describe --tags --always 2>/dev/null),0.0.0)

release:
	$(MAKE) clean
	$(MAKE) build

build: release/syver-darwin-amd64 release/syver-darwin-arm64 release/syver-linux-amd64 release/syver-linux-arm release/syver-linux-arm64 release/syver-linux-s390x release/syver-linux-ppc64le release/syver-windows-amd64

gen:
	$(info INFO: Starting build $@)
	go generate -tags genny $(pkgs)

clean:
	$(info INFO: Starting build $@)
	rm -rf ./release
	rm -rf ./dist
	rm -rf ./site
	rm -rf ${VENV}

build-images:
	$(info INFO: Starting build $@)
	development/build_images.sh

push-images:
	$(info INFO: Starting build $@)
	development/push_images.sh

# Update the matcher test golden files
update-matcher-tests:
	go test -v -run '^TestMatchers' . -update

test-darwin-all: test-short-all test-int-darwin-all
# linux _does_ have the docker-style testing, but does _not_ currently have the same style integration tests darwin+windows do, _because_ of the docker-style testing.
test-linux-all: test-short-all test-int-64
test-windows-all: test-short-all test-int-windows-all

test-int-64: rockylinux9 almalinux10 bullseye jammy alpine3 arch test-int-serve-linux-amd64
test-int-darwin-all: test-int-validate-darwin-amd64 test-int-serve-darwin-amd64 test-int-validate-darwin-arm64 test-int-serve-darwin-arm64
test-int-windows-all: test-int-validate-windows-amd64 test-int-serve-windows-amd64
test-int-all: test-int-64

centos7: release/syver-linux-amd64
	$(info INFO: Starting build $@)
	cd integration-tests/ && ./test.sh centos7 amd64
.PHONY: rockylinux9
rockylinux9: release/syver-linux-amd64
	$(info INFO: Starting build $@)
	cd integration-tests/ && ./test.sh rockylinux9 amd64
.PHONY: almalinux10
almalinux10: release/syver-linux-amd64
	$(info INFO: Starting build $@)
	cd integration-tests/ && ./test.sh almalinux10 amd64
.PHONY: bullseye
bullseye: release/syver-linux-amd64
	$(info INFO: Starting build $@)
	cd integration-tests/ && ./test.sh bullseye amd64
.PHONY: jammy
jammy: release/syver-linux-amd64
	$(info INFO: Starting build $@)
	cd integration-tests/ && ./test.sh jammy amd64
.PHONY: alpine3
alpine3: release/syver-linux-amd64
	$(info INFO: Starting build $@)
	cd integration-tests/ && ./test.sh alpine3 amd64
.PHONY: arch
arch: release/syver-linux-amd64
	$(info INFO: Starting build $@)
	cd integration-tests/ && ./test.sh arch amd64

dgoss-sha256:
	cd extras/dgoss/ && sha256sum dgoss > dgoss.sha256

dcgoss-sha256:
	cd extras/dcgoss/ && sha256sum dcgoss > dcgoss.sha256

kgoss-sha256:
	cd extras/kgoss/ && sha256sum kgoss > kgoss.sha256

dsyver-sha256:
	cd extras/dgoss/ && sha256sum dsyver > dsyver.sha256

dcsyver-sha256:
	cd extras/dcgoss/ && sha256sum dcsyver > dcsyver.sha256

ksyver-sha256:
	cd extras/kgoss/ && sha256sum ksyver > ksyver.sha256

.PHONY: lint-yaml
lint-yaml:
	$(info INFO: Starting $@)
	yamllint -c .yamllint .

.PHONY: lint-markdown
lint-markdown:
	$(info INFO: Starting $@)
	./ci/lint-markdown.sh

.PHONY: test-discovery-e2e
test-discovery-e2e:
	$(info INFO: Starting $@)
	./ci/discovery-e2e.sh

.PHONY: test-depends-on-e2e
test-depends-on-e2e:
	$(info INFO: Starting $@)
	./ci/depends-on-e2e.sh

.PHONY: test-security
test-security:
	$(info INFO: Starting $@)
	./ci/security-scan.sh

.PHONY: check
check: test test-discovery-e2e test-depends-on-e2e lint-markdown test-security
	$(info INFO: Starting $@)

# Fast checks to run before every commit: formatting, vet, unit tests.
.PHONY: pre-commit
pre-commit: fmt vet
	$(info INFO: Starting $@)
	go test ./...

# Full local bundle to run before pushing / opening a PR: adds lint and the
# same E2E + security checks CI runs in its coverage job.
.PHONY: pre-push
pre-push: fmt vet lint check
	$(info INFO: Starting $@)

$(PYTHON):
	$(info Creating virtualenv in $(VENV))
	@python -m venv $(VENV)

$(DOCS_DEPS): $(PYTHON) docs/requirements.txt
	$(info Installing dependencies)
	@pip install --upgrade pip
	@pip install --requirement docs/requirements.txt
	@touch $(DOCS_DEPS)

docs/setup: $(DOCS_DEPS)

docs/serve: docs/setup
	$(info Running documentation live development server)
	@mkdocs serve --strict

.PHONY: docs
docs: docs/setup
	$(info Building documentation)
	@mkdocs build --strict
