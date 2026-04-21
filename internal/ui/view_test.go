package ui

import (
	"strings"
	"testing"

	pb "github.com/Rioverde/gongeons/internal/proto"
)

// TestRenderCellLayerPrecedence exercises the documented rendering layers:
// occupant > world-object (village / castle) > river > terrain. The table
// keeps each case self-contained so a regression in one layer doesn't drag
// the others down with it. Assertions use strings.Contains against the raw
// output to avoid hand-rolling ANSI-escape comparisons.
func TestRenderCellLayerPrecedence(t *testing.T) {
	t.Parallel()

	plainsRune := terrainRunes[pb.Terrain_TERRAIN_PLAINS]
	villageRune := objectRunes[pb.WorldObject_WORLD_OBJECT_VILLAGE]

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
			tile:        &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS, River: true},
			mustHave:    []string{riverRune},
			mustNotHave: []string{plainsRune},
		},
		{
			name: "village shows over plain terrain",
			tile: &pb.Tile{
				Terrain: pb.Terrain_TERRAIN_PLAINS,
				Object:  pb.WorldObject_WORLD_OBJECT_VILLAGE,
			},
			mustHave: []string{villageRune},
		},
		{
			name: "village wins over river",
			tile: &pb.Tile{
				Terrain: pb.Terrain_TERRAIN_PLAINS,
				River:   true,
				Object:  pb.WorldObject_WORLD_OBJECT_VILLAGE,
			},
			mustHave:    []string{villageRune},
			mustNotHave: []string{riverRune},
		},
		{
			name: "self player wins over village",
			myID: "me",
			tile: &pb.Tile{
				Terrain:  pb.Terrain_TERRAIN_PLAINS,
				Object:   pb.WorldObject_WORLD_OBJECT_VILLAGE,
				Occupant: pb.OccupantKind_OCCUPANT_PLAYER,
				EntityId: "me",
			},
			mustHave:    []string{runeSelf},
			mustNotHave: []string{villageRune, runeOther},
		},
		{
			name: "unknown WorldObject falls back to unspecified rune",
			tile: &pb.Tile{
				Terrain: pb.Terrain_TERRAIN_PLAINS,
				Object:  pb.WorldObject(99),
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
