# .PHONY: every `make` should recompile everything
.PHONY: all build bin/run

test: build
	test/run/suite.bats
	test/convert-single-file/suite.bats

all: build

build: bin/run bin/convert-single-file bin/test-convert-single-file

bin/run: go-deps
	CGO_ENABLED=0 go build -ldflags="-s -w" -tags netgo -o $@ cmd/run/main.go \
		&& stat -c '%n %s' $@

bin/convert-single-file: go-deps
	CGO_ENABLED=0 go build -ldflags="-s -w" -tags netgo -o $@ cmd/convert-single-file/main.go \
		&& stat -c '%n %s' $@

bin/test-convert-single-file: go-deps
	CGO_ENABLED=0 go build -ldflags="-s -w" -tags netgo -o $@ cmd/test-convert-single-file/main.go \
		&& stat -c '%n %s' $@

go-deps:
	go get -d -v ./...
	go install -v ./...
