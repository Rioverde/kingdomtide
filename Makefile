# Gongeons — multiplayer turn-based RPG (terminal client + gRPC server).
#
# Common flow:
#   make tools     # one-time install of protoc plugins
#   make proto     # regenerate internal/proto/*.pb.go from proto/*.proto
#   make build     # build both binaries into bin/
#   make run-server  (terminal A)
#   make run-client  (terminal B)

MODULE      := github.com/Rioverde/gongeons
PROTO_DIR   := proto
PROTO_FILE  := $(PROTO_DIR)/gongeons.proto
PROTO_OUT   := internal/proto

GO          ?= go
PROTOC      ?= protoc

CLIENT_BIN  := gongeons
SERVER_BIN  := gongeonsd

SERVER_ADDR ?= :50051
CLIENT_ADDR ?= localhost:50051

.PHONY: help build build-client build-server run-server run-client test tidy proto clean tools

help:
	@echo "Gongeons make targets:"
	@echo "  build         - build both binaries into bin/"
	@echo "  build-client  - build $(CLIENT_BIN)"
	@echo "  build-server  - build $(SERVER_BIN)"
	@echo "  run-server    - run server on $(SERVER_ADDR)"
	@echo "  run-client    - run client against $(CLIENT_ADDR)"
	@echo "  test          - go test -race ./..."
	@echo "  proto         - regenerate pb.go from $(PROTO_FILE)"
	@echo "  tidy          - go mod tidy"
	@echo "  tools         - install protoc-gen-go + protoc-gen-go-grpc"
	@echo "  clean         - remove build artefacts"

build: build-client build-server

build-client:
	$(GO) build -o bin/$(CLIENT_BIN) ./cmd/gongeons

build-server:
	$(GO) build -o bin/$(SERVER_BIN) ./cmd/gongeonsd

run-server:
	$(GO) run ./cmd/gongeonsd -addr $(SERVER_ADDR)

run-client:
	$(GO) run ./cmd/gongeons -server $(CLIENT_ADDR)

test:
	$(GO) test -race ./...

tidy:
	$(GO) mod tidy

proto:
	$(PROTOC) \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(PROTO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_OUT) --go-grpc_opt=paths=source_relative \
		$(PROTO_FILE)

tools:
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

clean:
	rm -rf bin/
