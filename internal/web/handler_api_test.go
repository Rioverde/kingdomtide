package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer returns a Server wired with the chi router, suitable for httptest.
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	srv, err := NewServer(Config{
		TilesDir: "../../assets/tiles",
		Seed:     42,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv.Handler()
}

func TestHandleAPIMeta(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var meta apiMeta
	if err := json.NewDecoder(rr.Body).Decode(&meta); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if meta.Seed == 0 {
		t.Error("expected non-zero seed in meta")
	}
	if meta.ChunkSize == 0 {
		t.Error("expected non-zero chunkSize in meta")
	}
	if meta.TileImageWidth == 0 {
		t.Error("expected non-zero tileImageWidth in meta")
	}
	if meta.TileImageHeight == 0 {
		t.Error("expected non-zero tileImageHeight in meta")
	}
	if len(meta.Terrains) == 0 {
		t.Error("expected non-empty terrains in meta")
	}
	if len(meta.Objects) == 0 {
		t.Error("expected non-empty objects in meta")
	}
	for _, obj := range meta.Objects {
		if obj.Kind == "" {
			t.Error("object entry has empty kind")
		}
		if obj.Asset == "" {
			t.Errorf("object kind %q has empty asset", obj.Kind)
		}
	}
}

func TestHandleAPIChunk_MissingParams(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/chunk", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	var body apiError
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error == "" {
		t.Error("expected non-empty error field in response")
	}
}

func TestHandleAPIChunk_ValidParams(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/chunk?cx=0&cy=0", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var chunk apiChunk
	if err := json.NewDecoder(rr.Body).Decode(&chunk); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	const wantTiles = 256 // ChunkSize(16) * ChunkSize(16)
	if len(chunk.Tiles) != wantTiles {
		t.Errorf("expected %d tiles, got %d", wantTiles, len(chunk.Tiles))
	}
}

func TestHandleAPIChunk_InvalidCX(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/chunk?cx=abc", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	var body apiError
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error == "" {
		t.Error("expected non-empty error field in response")
	}
}

// itoa is a local helper to avoid importing strconv in the test file.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
