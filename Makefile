FIND_EXCLUSIONS= \
	-not \( \( -path '*/.git/*' -o -path './build/*' -o -path './vendor/*' -o -path '*/.terraform/*' \) -prune \)
GO_SRC_FILES := $(shell find . $(FIND_EXCLUSIONS) -type f -name '*.go' -not -name '*_test.go')
GO_FMT_FILES := $(shell find . $(FIND_EXCLUSIONS) -type f -name '*.go' -print0 | xargs -0 grep -E --null -L '^// Code generated .* DO NOT EDIT\.$$' | tr '\0' ' ')

default: build

build/paralleltestctx: $(GO_SRC_FILES) go.mod go.sum
	mkdir -p ./build
	go build -o ./build/paralleltestctx .

build: build/paralleltestctx
.PHONY: build

fmt:
	go mod tidy
	go run golang.org/x/tools/cmd/goimports@v0.35.0 -w $(GO_FMT_FILES)
	go run mvdan.cc/gofumpt@v0.8.0 -w -l $(GO_FMT_FILES)
.PHONY: fmt

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0 run ./...
.PHONY: lint

test:
	go test -test.v -timeout 30s -cover ./...
.PHONY: test
