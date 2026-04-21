package ui

import (
	"strings"
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// TestRenderCellLayerPrecedence exercises the documented rendering layers:
// occupant > structure (village / castle) > river overlay > terrain. The
// table keeps each case self-contained so a regression in one layer doesn't
// drag the others down with it. Assertions use strings.Contains against the
// raw output to avoid hand-rolling ANSI-escape comparisons.
func TestRenderCellLayerPrecedence(t *testing.T) {
	t.Parallel()

	plainsRune := terrainRunes[pb.Terrain_TERRAIN_PLAINS]
	villageRune := structureRunes[pb.Structure_STRUCTURE_VILLAGE]

	cases := []struct {
		name        string
		myID        string
		tile        *pb.Tile
		mustHave    []string
		mustNotHave []string
	}{
		{
			name:        "plain terrain shows terrain rune, not player",
			tile:        &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS},
			mustHave:    []string{plainsRune},
			mustNotHave: []string{runeSelf, runeOther},
		},
		{
			name:        "river overrides terrain",
			tile:        &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS, Overlays: uint32(game.OverlayRiver)},
			mustHave:    []string{riverRune},
			mustNotHave: []string{plainsRune},
		},
		{
			name: "village shows over plain terrain",
			tile: &pb.Tile{
				Terrain:   pb.Terrain_TERRAIN_PLAINS,
				Structure: pb.Structure_STRUCTURE_VILLAGE,
			},
			mustHave: []string{villageRune},
		},
		{
			name: "village wins over river",
			tile: &pb.Tile{
				Terrain:   pb.Terrain_TERRAIN_PLAINS,
				Overlays:  uint32(game.OverlayRiver),
				Structure: pb.Structure_STRUCTURE_VILLAGE,
			},
			mustHave:    []string{villageRune},
			mustNotHave: []string{riverRune},
		},
		{
			name: "self player wins over village",
			myID: "me",
			tile: &pb.Tile{
				Terrain:   pb.Terrain_TERRAIN_PLAINS,
				Structure: pb.Structure_STRUCTURE_VILLAGE,
				Occupant:  pb.OccupantKind_OCCUPANT_PLAYER,
				EntityId:  "me",
			},
			mustHave:    []string{runeSelf},
			mustNotHave: []string{villageRune, runeOther},
		},
		{
			name: "unknown Structure falls back to unspecified rune",
			tile: &pb.Tile{
				Terrain:   pb.Terrain_TERRAIN_PLAINS,
				Structure: pb.Structure(99),
			},
			mustHave:    []string{runeUnspecified},
			mustNotHave: []string{plainsRune},
		},
		{
			name:     "nil tile renders unspecified rune",
			tile:     nil,
			mustHave: []string{runeUnspecified},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &Model{myID: tc.myID}
			out := m.renderCell(tc.tile)
			for _, want := range tc.mustHave {
				if !strings.Contains(out, want) {
					t.Errorf("output %q missing expected glyph %q", out, want)
				}
			}
			for _, bad := range tc.mustNotHave {
				if strings.Contains(out, bad) {
					t.Errorf("output %q unexpectedly contains glyph %q", out, bad)
				}
			}
		})
	}
}
