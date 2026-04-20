package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Rioverde/gongeons/internal/game"
)

// Tile PNG dimensions in pixels. The hex base fills the full image width and about 296px of the
// height; the remaining vertical space is headroom for objects that may spill above the hex.
// These constants are served via /api/meta so the browser renderer can compute layout without
// hard-coding them.
const (
	tileImageWidth  = 256
	tileImageHeight = 384
)

// apiTile is the JSON shape for one hex sent to the client. River and Object are
// omitted from the payload when at their zero values so clients that do not yet know
// about those fields continue to receive compact responses.
type apiTile struct {
	Q       int             `json:"q"`
	R       int             `json:"r"`
	Terrain string          `json:"terrain"`
	River   bool            `json:"river,omitempty"`
	Object  game.ObjectKind `json:"object,omitempty"`
}

// apiChunk is the JSON shape for the /api/chunk response.
type apiChunk struct {
	CX    int       `json:"cx"`
	CY    int       `json:"cy"`
	Tiles []apiTile `json:"tiles"`
}

// apiTerrainAsset pairs a terrain enum string with its PNG filename.
type apiTerrainAsset struct {
	Terrain string `json:"terrain"`
	Asset   string `json:"asset"`
}

// apiObjectAsset pairs an object kind string with its PNG overlay filename so the client
// can pre-cache POI sprites when it boots.
type apiObjectAsset struct {
	Kind  string `json:"kind"`
	Asset string `json:"asset"`
}

// apiMeta is the JSON shape for /api/meta.
type apiMeta struct {
	Seed            int64            `json:"seed"`
	ChunkSize       int              `json:"chunkSize"`
	TileImageWidth  int              `json:"tileImageWidth"`
	TileImageHeight int              `json:"tileImageHeight"`
	Terrains        []apiTerrainAsset `json:"terrains"`
	Objects         []apiObjectAsset  `json:"objects"`
}

// apiError is the JSON shape for error responses.
type apiError struct {
	Error string `json:"error"`
}

// writeJSON marshals v as JSON and writes it with the appropriate content-type header.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// writeJSONError writes a JSON error body with the given HTTP status code.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiError{Error: msg})
}

// handleAPIMeta serves the static meta block describing the current world.
func (s *Server) handleAPIMeta(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	world := s.world
	seed := s.seed
	s.mu.RUnlock()

	if world == nil || world.Generator() == nil {
		writeJSONError(w, http.StatusInternalServerError, "world not initialised")
		return
	}

	terrains := game.AllTerrains()
	manifest := make([]apiTerrainAsset, 0, len(terrains))
	for _, t := range terrains {
		manifest = append(manifest, apiTerrainAsset{
			Terrain: string(t),
			Asset:   game.TileAsset(t),
		})
	}

	kinds := game.AllObjectKinds()
	objects := make([]apiObjectAsset, 0, len(kinds))
	for _, k := range kinds {
		objects = append(objects, apiObjectAsset{
			Kind:  string(k),
			Asset: game.ObjectSprite(k),
		})
	}

	writeJSON(w, apiMeta{
		Seed:            seed,
		ChunkSize:       game.ChunkSize,
		TileImageWidth:  tileImageWidth,
		TileImageHeight: tileImageHeight,
		Terrains:        manifest,
		Objects:         objects,
	})
}

// handleAPIChunk serves one chunk keyed by ?cx=X&cy=Y. Missing or malformed params
// return 400 with a JSON error body.
func (s *Server) handleAPIChunk(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	cxStr := query.Get("cx")
	if cxStr == "" {
		writeJSONError(w, http.StatusBadRequest, "missing cx parameter")
		return
	}
	cx, err := strconv.Atoi(cxStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid cx: must be an integer")
		return
	}

	cyStr := query.Get("cy")
	if cyStr == "" {
		writeJSONError(w, http.StatusBadRequest, "missing cy parameter")
		return
	}
	cy, err := strconv.Atoi(cyStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid cy: must be an integer")
		return
	}

	s.mu.RLock()
	world := s.world
	s.mu.RUnlock()

	if world == nil || world.Generator() == nil {
		writeJSONError(w, http.StatusInternalServerError, "world not initialised")
		return
	}

	cc := game.ChunkCoord{X: cx, Y: cy}
	chunk := world.ChunkAt(cc)
	minQ, _, minR, _ := chunk.Bounds()

	tiles := make([]apiTile, 0, game.ChunkSize*game.ChunkSize)
	for dr := 0; dr < game.ChunkSize; dr++ {
		for dq := 0; dq < game.ChunkSize; dq++ {
			t := chunk.Tiles[dr][dq]
			tiles = append(tiles, apiTile{
				Q:       minQ + dq,
				R:       minR + dr,
				Terrain: string(t.Terrain),
				River:   t.River,
				Object:  t.Object,
			})
		}
	}

	writeJSON(w, apiChunk{CX: cx, CY: cy, Tiles: tiles})
}
