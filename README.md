# Gongeons

Turn-based square-grid RPG. Terminal client ([Bubble Tea](https://github.com/charmbracelet/bubbletea))
talks to an authoritative gRPC server over a bidirectional stream. Multiplayer,
one shared world per server process, localhost-only for now.

## Stack

- Go 1.25+
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss) — terminal UI
- [gRPC](https://grpc.io) + [Protocol Buffers](https://protobuf.dev) — wire protocol
- `sync.Mutex` around the world, context-aware goroutines, no persistence

## Layout

```
cmd/
  gongeons/        client binary (terminal UI)
  gongeonsd/       server binary
internal/
  game/            pure domain: World, Command, Event, ApplyCommand, rules
  ui/              bubbletea Model/Update/View + gRPC client glue
  server/          gRPC service impl, subscriber hub
  proto/           generated protobuf Go (committed; do not hand-edit)
proto/
  gongeons.proto   hand-written schema, source of truth
```

## Quick start

One-time host setup:

```
brew install protobuf
make tools
```

Fetch Go deps, generate protobuf, build:

```
go mod tidy
make proto
make build
```

Run — two terminals:

```
# terminal A
make run-server             # listens on :50051

# terminal B
make run-client             # connects to localhost:50051
```

Multiple clients can connect to the same server; they share one world.

## Controls

| Key            | Action       |
|----------------|--------------|
| `w`            | Move north   |
| `a`            | Move west    |
| `s`            | Move south   |
| `d`            | Move east    |
| `q` / `Ctrl-C` | Quit         |

Combat, monsters, items etc. are not in the MVP — only players walking on a
shared map. See the plan doc for what comes next.

## Make targets

| Target           | Description                                         |
|------------------|-----------------------------------------------------|
| `make build`     | Build both binaries into `bin/`                     |
| `make run-server`| Run `gongeonsd` in foreground                       |
| `make run-client`| Run `gongeons` against `localhost:50051`            |
| `make test`      | `go test -race ./...`                               |
| `make proto`     | Regenerate `internal/proto/*.pb.go`                 |
| `make tools`     | Install `protoc-gen-go` + `protoc-gen-go-grpc`      |
| `make tidy`      | `go mod tidy`                                       |
| `make clean`     | Remove `bin/`                                        |

## Development plan

See [`.omc/plans/gongeons-plan.md`](.omc/plans/gongeons-plan.md) for phased
milestones, design decisions, and ownership boundaries.
