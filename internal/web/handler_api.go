package web

import (
	"encoding/json"
	"errors"
	"log"
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

	// maxChunkCoord is the inclusive absolute limit on cx and cy query parameters.
	// Coordinates beyond this are rejected with 400 to prevent adversarial clients
	// from thrashing the LRU chunk cache with unreachable addresses.
	maxChunkCoord = 1 << 20
)

// Sentinel errors for API parameter validation. Using package-level vars avoids a
// heap allocation on every rejected request compared to errors.New at call sites.
var (
	errMissingCX            = errors.New("missing cx parameter")
	errInvalidCX            = errors.New("invalid cx parameter")
	errMissingCY            = errors.New("missing cy parameter")
	errInvalidCY            = errors.New("invalid cy parameter")
	errChunkCoordOutOfRange = errors.New("chunk coord out of range")
	errWorldNotReady        = errors.New("world not initialised")
)

// Error code constants provide machine-readable identifiers alongside human messages.
const (
	codeMissingCX            = "missing_cx"
	codeInvalidCX            = "invalid_cx"
	codeMissingCY            = "missing_cy"
	codeInvalidCY            = "invalid_cy"
	codeChunkCoordOutOfRange = "chunk_coord_out_of_range"
	codeWorldNotReady        = "world_not_ready"
)

// apiTile is the JSON shape for one hex sent to the client. River and Object are
// omitted from the payload when at their zero values so clients that do not yet know
// about those fields continue to receive compact responses.
type apiTile struct {
	Q       int    `json:"q"`
	R       int    `json:"r"`
	Terrain string `json:"terrain"`
	River   bool   `json:"river,omitempty"`
	Object  string `json:"object,omitempty"`
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
	Seed            int64             `json:"seed"`
	ChunkSize       int               `json:"chunkSize"`
	TileImageWidth  int               `json:"tileImageWidth"`
	TileImageHeight int               `json:"tileImageHeight"`
	Terrains        []apiTerrainAsset `json:"terrains"`
	Objects         []apiObjectAsset  `json:"objects"`
}

// apiError is the JSON shape for error responses. ErrorCode carries a machine-readable
// enum value so clients can branch on error type without parsing the human message.
type apiError struct {
	Error     string `json:"error"`
	ErrorCode string `json:"error_code"`
}

// writeJSON marshals v as JSON and writes it with the appropriate content-type header.
// If encoding fails after headers are already sent there is nothing useful to do, so
// the error is logged and silently discarded.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON encode: %v", err)
	}
}

// writeJSONError writes a JSON error body with the given HTTP status code, human-readable
// error message, and machine-readable error code.
func writeJSONError(w http.ResponseWriter, status int, err error, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiError{Error: err.Error(), ErrorCode: code})
}

// handleAPIMeta serves the static meta block describing the current world.
func (s *Server) handleAPIMeta(w http.ResponseWriter, _ *http.Request) {
	world := s.world.Load()
	seed := s.seed.Load()

	if world == nil || world.Generator() == nil {
		writeJSONError(w, http.StatusInternalServerError, errWorldNotReady, codeWorldNotReady)
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
// return 400 with a JSON error body. Coordinates outside ±maxChunkCoord are also
// rejected with 400 to prevent cache thrashing.
func (s *Server) handleAPIChunk(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	cxStr := query.Get("cx")
	if cxStr == "" {
		writeJSONError(w, http.StatusBadRequest, errMissingCX, codeMissingCX)
		return
	}
	cx, err := strconv.Atoi(cxStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, errInvalidCX, codeInvalidCX)
		return
	}

	cyStr := query.Get("cy")
	if cyStr == "" {
		writeJSONError(w, http.StatusBadRequest, errMissingCY, codeMissingCY)
		return
	}
	cy, err := strconv.Atoi(cyStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, errInvalidCY, codeInvalidCY)
		return
	}

	if cx > maxChunkCoord || cx < -maxChunkCoord || cy > maxChunkCoord || cy < -maxChunkCoord {
		writeJSONError(w, http.StatusBadRequest, errChunkCoordOutOfRange, codeChunkCoordOutOfRange)
		return
	}

	world := s.world.Load()
	if world == nil || world.Generator() == nil {
		writeJSONError(w, http.StatusInternalServerError, errWorldNotReady, codeWorldNotReady)
		return
	}

	cc := game.ChunkCoord{X: cx, Y: cy}
	chunk := world.ChunkAt(cc)
	minQ, _, minR, _ := chunk.Bounds()

	// Copy tile data into local apiTile values while we still hold access to the
	// chunk. Tile.Occupant is mutable by the combat layer, so we snapshot everything
	// we need before the chunk pointer leaves our control. The 16×16 = 256 copy is
	// negligible compared to JSON encoding cost.
	tiles := make([]apiTile, 0, game.ChunkSize*game.ChunkSize)
	for dr := 0; dr < game.ChunkSize; dr++ {
		for dq := 0; dq < game.ChunkSize; dq++ {
			t := chunk.Tiles[dr][dq]
			tiles = append(tiles, apiTile{
				Q:       minQ + dq,
				R:       minR + dr,
				Terrain: string(t.Terrain),
				River:   t.River,
				Object:  string(t.Object),
			})
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, apiChunk{CX: cx, CY: cy, Tiles: tiles})
}
