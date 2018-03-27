# .PHONY: every `make` should recompile everything
.PHONY: all build bin/run

test: build
	test/convert-single-file/suite.bats

all: build

build: bin/run bin/convert-single-file

bin/run: go-deps
	go build -ldflags="-s -w" -o $@ cmd/run/main.go \
		&& stat -c '%n %s' $@

bin/convert-single-file: go-deps
	go build -ldflags="-s -w" -o $@ cmd/convert-single-file/main.go \
		&& stat -c '%n %s' $@

go-deps:
	go get -d -v ./...
	go install -v ./...
