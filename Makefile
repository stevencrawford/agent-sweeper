PKG = github.com/stevencrawford/agent-sweeper
COMMIT = $(shell git rev-parse --short HEAD)

BUILD_LDFLAGS = "-s -w -X $(PKG)/version.Revision=$(COMMIT)"

default: test

ci: test

test:
	go test ./... -coverprofile=coverage.out -covermode=count -count=1

build:
	go build -ldflags=$(BUILD_LDFLAGS) -trimpath -o agent-sweeper .

lint:
	golangci-lint run ./...
	go vet -vettool=`which gostyle` -gostyle.config=$(PWD)/.gostyle.yml ./...

prerelease_for_tagpr:
	git add go.mod go.sum

.PHONY: default ci test build lint prerelease_for_tagpr
