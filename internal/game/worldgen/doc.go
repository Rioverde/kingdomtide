// Package worldgen holds the deterministic procedural-generation pipeline
// for gongeons. Everything here is a pure function of (seed, coord): the
// same inputs always produce the same world.Tile. Output types come from
// internal/game; worldgen never mutates external state.
package worldgen
