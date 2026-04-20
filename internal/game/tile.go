package game

// Tile is the atomic unit of the world map. Terrain drives rendering and
// biome logic; Occupant holds an entity standing on the tile (player, monster,
// NPC). River marks tiles that have been traced as part of a river path. Object
// holds an optional point-of-interest overlay (village, castle, etc.). Road
// marks tiles that form part of a generated road network between POIs. River,
// Object, and Road are omitted from JSON when their zero value so existing
// clients see no extra noise.
type Tile struct {
	Terrain  Terrain    `json:"terrain"`
	Occupant Occupant   `json:"occupant,omitempty"`
	River    bool       `json:"river,omitempty"`
	Object   ObjectKind `json:"object,omitempty"`
	Road     bool       `json:"road,omitempty"`
}
