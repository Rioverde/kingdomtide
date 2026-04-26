# Kingdomtide

A deterministic medieval-world simulation. One `int64` seed grows a map of biomes, regions, and landmarks, then folds N years of history forward across kingdoms, cities, and villages — wars, plagues, schisms, succession crises. Players walk into the finished world through a terminal client and share it over gRPC.

## Run

```
make tools && make proto && make build
make run-server   # :50051
make run-client   # in another terminal
```

## Controls

`wasd` move · `q` quit
