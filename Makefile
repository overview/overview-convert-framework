# .PHONY: every `make` should recompile everything
.PHONY: all build bin/run

test: build
	test/convert-stream-to-mime-multipart/suite.bats
	test/convert-single-file/suite.bats
	test/run/suite.bats

all: build

build: bin/run bin/convert-single-file bin/convert-stream-to-mime-multipart bin/test-convert-single-file

bin/run: go-deps
	CGO_ENABLED=0 go build -ldflags="-s -w" -tags netgo -o $@ cmd/run/main.go \
		&& stat -c '%n %s' $@

bin/convert-single-file: go-deps
	CGO_ENABLED=0 go build -ldflags="-s -w" -tags netgo -o $@ cmd/convert-single-file/main.go \
		&& stat -c '%n %s' $@

bin/convert-stream-to-mime-multipart: go-deps
	CGO_ENABLED=0 go build -ldflags="-s -w" -tags netgo -o $@ cmd/convert-stream-to-mime-multipart/main.go \
		&& stat -c '%n %s' $@

bin/test-convert-single-file: go-deps
	CGO_ENABLED=0 go build -ldflags="-s -w" -tags netgo -o $@ cmd/test-convert-single-file/main.go \
		&& stat -c '%n %s' $@

go-deps:
	go get -d -v ./...
	go install -v ./...
