# Keep toolchain/module/build caches inside the workspace.
export GOPATH := $(CURDIR)/.gopath
export GOCACHE := $(CURDIR)/.gocache

.PHONY: build test vet check fmt
build:
	go build -o metracker .
	go build -o ironman-reminder ./cmd/ironman

test:
	go test -race ./...

vet:
	go vet ./...

check: test vet

fmt:
	gofmt -w main.go internal cmd
