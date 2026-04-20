(function () {
  'use strict';

  // ── Constants ──────────────────────────────────────────────────────────────
  const MAX_IMGS = 4000;
  const CHUNK_BUFFER = 1;       // extra chunks to load beyond visible edge
  const EVICT_DISTANCE = 3;     // chunks beyond visible to evict
  const LOAD_DEBOUNCE_MS = 50;
  const PAN_SPEED = 60;         // pixels per keypress

  // ── Tile geometry (pointy-top hex) ────────────────────────────────────────
  // These are the raw sprite dimensions; axial→pixel uses them directly.
  const TILE_W = 256;
  const TILE_H = 384;
  // Kenney-style pointy-top tiles: the hex base occupies roughly the bottom 256px of the
  // 384px image; the top 128px is decoration (trees, peaks, structures). Column spacing is
  // the full tile width, row spacing is 3/4 of the hex height (standard pointy-top packing).
  const HEX_BASE = 256;
  const ROW_SPACING = 192;

  function axialToPixel(q, r) {
    const px = TILE_W * (q + r / 2);
    const py = ROW_SPACING * r - (TILE_H - HEX_BASE);
    return { px, py };
  }

  // ── State ──────────────────────────────────────────────────────────────────
  let meta = null;       // fetched from /api/meta
  let objectSprites = {};  // kind → filename, populated from meta.objects
  let camera = { x: 0, y: 0, zoom: 0.4 };
  // Each chunk entry: { cx, cy, tiles, imgs: Map<"q,r", img>, overlays: Map<"q,r", img> }
  const chunks = new Map();

  // ── DOM ────────────────────────────────────────────────────────────────────
  const viewport = document.getElementById('viewport');
  const slider   = document.getElementById('zoom');
  const zoomLbl  = document.getElementById('zoomLabel');
  const coordsEl = document.getElementById('coords');
  const seedForm = document.getElementById('seedForm');
  const seedInput = document.getElementById('seed');

  // Single world div; positioned absolutely inside viewport.
  const worldEl = document.createElement('div');
  worldEl.className = 'world';
  viewport.appendChild(worldEl);

  // ── Seed form ──────────────────────────────────────────────────────────────
  seedForm.addEventListener('submit', (e) => {
    e.preventDefault();
    const v = seedInput.value.trim();
    window.location.href = v ? '/?seed=' + encodeURIComponent(v) : '/';
  });

  // ── Zoom helpers ───────────────────────────────────────────────────────────
  function clampZoom(z) {
    return Math.max(0.1, Math.min(2.0, z));
  }

  function applyZoom(newZoom, pivotVx, pivotVy) {
    // pivotVx/Vy: viewport-relative pixel that stays fixed during zoom.
    // Default: center of viewport.
    if (pivotVx == null) pivotVx = viewport.clientWidth / 2;
    if (pivotVy == null) pivotVy = viewport.clientHeight / 2;

    const oldZoom = camera.zoom;
    newZoom = clampZoom(newZoom);

    // World-space point under pivot must remain at same viewport position.
    // worldX = (pivotVx - camera.x) / oldZoom
    const wx = (pivotVx - camera.x) / oldZoom;
    const wy = (pivotVy - camera.y) / oldZoom;
    camera.x = pivotVx - wx * newZoom;
    camera.y = pivotVy - wy * newZoom;
    camera.zoom = newZoom;

    const pct = Math.round(newZoom * 100);
    slider.value = Math.round(pct / 5) * 5; // snap to step
    zoomLbl.textContent = pct + '%';

    scheduleRender();
    scheduleChunkLoad();
  }

  slider.addEventListener('input', () => {
    applyZoom(slider.value / 100);
  });

  // ── Pan helpers ────────────────────────────────────────────────────────────
  function pan(dx, dy) {
    camera.x += dx;
    camera.y += dy;
    scheduleRender();
    scheduleChunkLoad();
  }

  // ── Mouse drag ────────────────────────────────────────────────────────────
  let drag = null;

  viewport.addEventListener('mousedown', (e) => {
    if (e.button !== 0) return;
    drag = { startX: e.clientX, startY: e.clientY, camX: camera.x, camY: camera.y };
    viewport.classList.add('dragging');
    e.preventDefault();
  });

  window.addEventListener('mousemove', (e) => {
    if (!drag) return;
    camera.x = drag.camX + (e.clientX - drag.startX);
    camera.y = drag.camY + (e.clientY - drag.startY);
    scheduleRender();
    scheduleChunkLoad();
  });

  window.addEventListener('mouseup', () => {
    drag = null;
    viewport.classList.remove('dragging');
  });

  // ── Mouse wheel zoom ───────────────────────────────────────────────────────
  viewport.addEventListener('wheel', (e) => {
    e.preventDefault();
    const factor = e.deltaY < 0 ? 1.1 : 0.9;
    const rect = viewport.getBoundingClientRect();
    applyZoom(camera.zoom * factor, e.clientX - rect.left, e.clientY - rect.top);
  }, { passive: false });

  // ── Keyboard pan ──────────────────────────────────────────────────────────
  window.addEventListener('keydown', (e) => {
    // Don't hijack input fields
    if (e.target.tagName === 'INPUT') return;
    switch (e.key) {
      case 'ArrowLeft':  case 'a': pan( PAN_SPEED, 0); break;
      case 'ArrowRight': case 'd': pan(-PAN_SPEED, 0); break;
      case 'ArrowUp':    case 'w': pan(0,  PAN_SPEED); break;
      case 'ArrowDown':  case 's': pan(0, -PAN_SPEED); break;
    }
  });

  // ── Coord readout ─────────────────────────────────────────────────────────
  viewport.addEventListener('mousemove', (e) => {
    if (!meta) return;
    const rect = viewport.getBoundingClientRect();
    // viewport pixel → world pixel → axial
    const wx = (e.clientX - rect.left - camera.x) / camera.zoom;
    const wy = (e.clientY - rect.top  - camera.y) / camera.zoom;
    // invert axial→pixel:  px = TILE_W*(q + r/2),  py = ROW_SPACING*r - (TILE_H-HEX_BASE)
    const r = Math.round((wy + (TILE_H - HEX_BASE)) / ROW_SPACING);
    const q = Math.round(wx / TILE_W - r / 2);
    coordsEl.textContent = q + ', ' + r;
  });

  // ── Chunk coordinate maths ────────────────────────────────────────────────
  function chunkKey(cx, cy) { return cx + ',' + cy; }

  // World-pixel bounds of a chunk (tight bounding box of its tiles).
  function chunkPixelBounds(cx, cy) {
    const cs = meta.chunkSize;
    // chunk origin in axial: q0 = cx*cs, r0 = cy*cs
    const q0 = cx * cs, r0 = cy * cs;
    const { px: left, py: top } = axialToPixel(q0, r0);
    const { px: right, py: bottom } = axialToPixel(q0 + cs - 1, r0 + cs - 1);
    return { left, top, right: right + TILE_W, bottom: bottom + TILE_H };
  }

  // Viewport-visible world-pixel rect.
  function visibleWorldRect() {
    const vw = viewport.clientWidth;
    const vh = viewport.clientHeight;
    const left   = -camera.x / camera.zoom;
    const top    = -camera.y / camera.zoom;
    const right  = left + vw / camera.zoom;
    const bottom = top  + vh / camera.zoom;
    return { left, top, right, bottom };
  }

  // Enumerate chunks whose pixel bounds intersect the buffered visible rect.
  function visibleChunkRange() {
    if (!meta) return null;
    const cs = meta.chunkSize;
    const vis = visibleWorldRect();
    // Expand by buffer chunks (in world pixels)
    const bufPx = CHUNK_BUFFER * cs * TILE_W;
    const el = vis.left - bufPx, er = vis.right + bufPx;
    const et = vis.top  - bufPx, eb = vis.bottom + bufPx;

    // Axial extents from pixel extents (rough; iterate extra)
    // px = TILE_W*(q + r/2) → q ≈ px/TILE_W
    const qMin = Math.floor(el / TILE_W) - cs;
    const qMax = Math.ceil(er  / TILE_W) + cs;
    const rMin = Math.floor((et + (TILE_H - HEX_BASE)) / ROW_SPACING) - cs;
    const rMax = Math.ceil((eb  + (TILE_H - HEX_BASE)) / ROW_SPACING) + cs;

    const cxMin = Math.floor(qMin / cs);
    const cxMax = Math.ceil(qMax  / cs);
    const cyMin = Math.floor(rMin / cs);
    const cyMax = Math.ceil(rMax  / cs);
    return { cxMin, cxMax, cyMin, cyMax };
  }

  // ── Chunk loading ─────────────────────────────────────────────────────────
  let loadTimer = null;

  function scheduleChunkLoad() {
    clearTimeout(loadTimer);
    loadTimer = setTimeout(doChunkLoad, LOAD_DEBOUNCE_MS);
  }

  function doChunkLoad() {
    if (!meta) return;
    const range = visibleChunkRange();
    if (!range) return;
    const { cxMin, cxMax, cyMin, cyMax } = range;

    // Evict far-away chunks
    for (const [key, chunk] of chunks) {
      if (
        chunk.cx < cxMin - EVICT_DISTANCE || chunk.cx > cxMax + EVICT_DISTANCE ||
        chunk.cy < cyMin - EVICT_DISTANCE || chunk.cy > cyMax + EVICT_DISTANCE
      ) {
        evictChunk(key, chunk);
      }
    }

    // Load missing chunks
    for (let cy = cyMin; cy <= cyMax; cy++) {
      for (let cx = cxMin; cx <= cxMax; cx++) {
        const key = chunkKey(cx, cy);
        if (!chunks.has(key)) {
          chunks.set(key, { cx, cy, tiles: null, imgs: new Map(), overlays: new Map(), loading: true });
          fetchChunk(cx, cy, key);
        }
      }
    }
  }

  function fetchChunk(cx, cy, key) {
    fetch('/api/chunk?cx=' + cx + '&cy=' + cy)
      .then(r => r.json())
      .then(data => {
        const chunk = chunks.get(key);
        if (!chunk) return; // evicted while in flight
        chunk.tiles = data.tiles;
        chunk.loading = false;
        renderChunkContents(chunk);
        scheduleRender();
      })
      .catch(err => {
        console.warn('chunk load failed', cx, cy, err);
        chunks.delete(key);
      });
  }

  function evictChunk(key, chunk) {
    // Remove all base tile img elements this chunk owns.
    for (const img of chunk.imgs.values()) {
      img.parentNode && img.parentNode.removeChild(img);
    }
    // Remove all overlay img elements this chunk owns.
    for (const img of chunk.overlays.values()) {
      img.parentNode && img.parentNode.removeChild(img);
    }
    chunks.delete(key);
  }

  // renderChunkContents builds the DOM for every tile + overlay inside chunk. Called ONCE
  // when a chunk's tile data arrives. Positions are fixed in world-pixel space and never
  // change — pan/zoom only retransforms the parent worldEl.
  function renderChunkContents(chunk) {
    if (!chunk.tiles || !meta) return;
    for (const tile of chunk.tiles) {
      const { q, r, terrain, object } = tile;
      const tileKey = q + ',' + r;
      const file = meta.terrains[terrain];
      if (!file) continue;

      const { px, py } = axialToPixel(q, r);
      const zidx = py + 10000;

      if (!chunk.imgs.has(tileKey)) {
        const img = document.createElement('img');
        img.src = '/tiles/' + file;
        img.style.width  = TILE_W + 'px';
        img.style.height = TILE_H + 'px';
        img.style.left   = px + 'px';
        img.style.top    = py + 'px';
        img.style.zIndex = zidx;
        img.alt = '';
        img.dataset.q = q;
        img.dataset.r = r;
        worldEl.appendChild(img);
        chunk.imgs.set(tileKey, img);
      }

      if (object && objectSprites[object] && !chunk.overlays.has(tileKey)) {
        const ov = document.createElement('img');
        ov.className = 'overlay';
        ov.src = '/tiles/' + objectSprites[object];
        ov.style.width  = TILE_W + 'px';
        ov.style.height = TILE_H + 'px';
        ov.style.left   = px + 'px';
        ov.style.top    = py + 'px';
        ov.style.zIndex = zidx + 1;
        ov.alt = '';
        ov.dataset.q = q;
        ov.dataset.r = r;
        worldEl.appendChild(ov);
        chunk.overlays.set(tileKey, ov);
      }
    }
  }

  // ── Rendering ─────────────────────────────────────────────────────────────
  let rafId = null;

  function scheduleRender() {
    if (!rafId) rafId = requestAnimationFrame(doRender);
  }

  function doRender() {
    rafId = null;
    if (!meta) return;

    // Pan + zoom is a single cheap CSS transform; tile DOM is static.
    worldEl.style.transform =
      'translate(' + camera.x + 'px, ' + camera.y + 'px) scale(' + camera.zoom + ')';
  }

  // ── Bootstrap ─────────────────────────────────────────────────────────────
  fetch('/api/meta')
    .then(r => r.json())
    .then(data => {
      meta = data;
      // Flatten the terrains array [{terrain, asset}] into a terrain→asset lookup so
      // the render loop can do meta.terrains[name] as an O(1) map access.
      if (Array.isArray(data.terrains)) {
        const map = {};
        for (const entry of data.terrains) {
          map[entry.terrain] = entry.asset;
        }
        meta.terrains = map;
      }
      // Same treatment for the objects manifest.
      if (Array.isArray(data.objects)) {
        for (const entry of data.objects) {
          objectSprites[entry.kind] = entry.asset;
        }
      }
      // Center camera on world origin.
      camera.x = viewport.clientWidth  / 2;
      camera.y = viewport.clientHeight / 2;
      scheduleChunkLoad();
      scheduleRender();
    })
    .catch(err => console.error('failed to load /api/meta', err));

})();
