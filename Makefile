# Pathly Kubernetes Operator

IMG ?= pathlyhq/pathly-k8s-operator:0.1.0
GO ?= go

.PHONY: all test cover build run manifests docker-build tidy

all: test build

tidy:
	$(GO) mod tidy

test:
	$(GO) test ./internal/client/ ./internal/controller/ -coverprofile=covinternal.txt -covermode=atomic -count=1
	$(GO) tool cover -func=covinternal.txt
	$(GO) run ./scripts/check_cover.go -profile=covinternal.txt -fail-under=100
	$(GO) test ./... -count=1 -cover

cover: test

build:
	$(GO) build -o bin/manager ./cmd/manager

run: build
	./bin/manager

manifests:
	@echo "CRDs live under config/crd/bases/ — apply with: kubectl apply -f config/crd/bases/"

docker-build:
	docker build -t $(IMG) .
