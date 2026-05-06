# SPDX-License-Identifier: Apache-2.0

GOIMAGE ?= golang:1.23-alpine
DOCKER_RUN = docker run --rm -v "$$PWD:/work" -w /work \
	-e GOCACHE=/tmp/gocache -e GOMODCACHE=/tmp/gomod -e HOME=/tmp \
	$(GOIMAGE)

BIN := route-r-pedals

.PHONY: build test vet clean help

help:
	@echo 'Targets:'
	@echo '  build  - build $(BIN) (static linux/amd64) inside docker'
	@echo '  test   - run unit tests inside docker'
	@echo '  vet    - go vet inside docker'
	@echo '  clean  - remove built binary'

build:
	$(DOCKER_RUN) sh -c 'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(BIN) .'

test:
	$(DOCKER_RUN) go test ./...

vet:
	$(DOCKER_RUN) go vet ./...

clean:
	rm -f $(BIN)
