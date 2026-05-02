# SSE Raw Encoding + xterm.js Write Batching

## Problem Statement

How might we reduce unnecessary CPU work and bandwidth overhead in the terminal output stream without changing the transport protocol?

## Recommended Direction

**Drop base64, keep SSE.** Use multi-line `data:` fields (SSE spec auto-joins with `\n`) to pass terminal bytes as raw text. Escape `\r` using byte-stuffing with SOH (`\x01`) as the escape prefix:

- `\r` → `\x01r`
- `\x01` → `\x01\x01`
- Everything else passes through verbatim

**Why SOH?** It's a legacy control byte (0x01) that essentially never appears in terminal output. The escape is unambiguous, fixed-length (always 2 bytes), trivial to implement in both Go and JS, and only adds overhead on the rare occasion `\r` or SOH actually appear.

Add `?encoding=raw` query param — server falls back to base64 JSON when absent, enabling gradual rollout and A/B testing.

Pair with **rAF write batching** on the client: accumulate chunks within one animation frame, flush once. Bypass for small single writes (≤64 bytes) to keep keystroke echo instant.

## Key Assumptions to Validate

- [ ] SSE multi-line `data:` faithfully round-trips all 256 byte values after SOH escaping — fuzz test
- [ ] SOH (0x01) doesn't appear in output from common tools (npm, go, cargo, shell prompts) — grep real captures
- [ ] rAF batching doesn't add perceptible latency to interactive keystroke echo — measure with timestamps

## MVP Scope

**In:**
- Server: `writeSSERawChunk` with SOH escaping, gated behind `?encoding=raw`
- Client: unescape function + remove base64 decode when using raw mode
- Client: rAF write batcher with small-write bypass
- Unit tests for encode/decode round-trip

**Out:**
- Changing the REST `GET /api/terminals/:id` bulk endpoint (stays base64 — only the streaming SSE path gets raw encoding)
- Removing legacy base64 path (keep it as fallback)
- Any transport change (no WebSocket, no gRPC)

## Not Doing (and Why)

- **WebSocket migration** — adds connection lifecycle complexity for negligible latency gain on localhost
- **gRPC / Connect** — needs proxy layer or codegen; solves a problem we don't have
- **Shared memory IPC** — platform-specific, tight coupling, debugging nightmare
- **Removing the legacy base64 path** — keep it until raw mode is proven stable
- **Raw encoding on REST endpoint** — only the streaming SSE path is performance-critical

## Decisions

- **Escape character:** SOH (`\x01`) — fixed-length 2-byte escape, never appears in real terminal output
- **Rollout strategy:** `?encoding=raw` query param; client opts in, server defaults to legacy base64 JSON
- **REST endpoint:** Stays base64 forever (not performance-critical)
- **Batch flush threshold:** ≤64 bytes flushes immediately (keystroke echo); larger writes batch per rAF

---

## Tasks

### Phase 1: Server-side — Raw text SSE encoding

#### Task 1: New `writeSSERawChunk` function

Add a new function that writes terminal output as raw text using multi-line `data:` fields with SOH escaping. Keep `writeSSEChunk` intact.

**Acceptance criteria:**
- [ ] `writeSSERawChunk(w, chunk)` emits `id:<unix_millis>\n` + `data:<segment>\n` per newline-delimited segment + trailing `\n`
- [ ] `\r` bytes escaped as `\x01r`; `\x01` bytes escaped as `\x01\x01`
- [ ] Empty chunks produce valid SSE frames
- [ ] Unit test: round-trip with `\n`, `\r`, `\r\n`, `\x01`, printable ASCII, and all 256 byte values

**Dependencies:** None  
**Files:** `internal/server/server.go`, `internal/server/sse_encoding_test.go`  
**Scope:** Small

---

#### Task 2: Gate raw encoding behind `?encoding=raw`

Wire up `writeSSERawChunk` in `handleEvents` and `handleShellEvents`, but only when the client passes `?encoding=raw`. Otherwise use the existing base64 JSON path.

**Acceptance criteria:**
- [ ] `?encoding=raw` → raw SSE frames; absent → legacy base64 JSON
- [ ] Both handlers respect the param
- [ ] No change to the REST `GET /api/terminals/:id` endpoint

**Verification:**
- [ ] `curl /events?id=x&encoding=raw` → readable text frames
- [ ] `curl /events?id=x` → base64 JSON (unchanged)

**Dependencies:** Task 1  
**Files:** `internal/server/server.go`  
**Scope:** Small

---

### Phase 2: Client-side — Raw text decoding + write batching

#### Task 3: Client-side SOH unescaping

Replace the base64 decode path with raw text unescape when connecting with `?encoding=raw`.

**Acceptance criteria:**
- [ ] New `unescapeRaw(data: string): Uint8Array` reverses SOH escaping
- [ ] `_connectSSE` appends `&encoding=raw` to the EventSource URL
- [ ] `onmessage` reads `event.lastEventId` for timestamp, `event.data` as raw text (no JSON.parse)
- [ ] Unit test: round-trip correctness

**Dependencies:** Task 2  
**Files:** `cmd/vortex-ui/web/components/vortex-terminal.ts`, `cmd/vortex-ui/web/types.ts`  
**Scope:** Small

---

#### Task 4: xterm.js rAF write batching

Accumulate incoming chunks and flush once per animation frame. Bypass for small writes.

**Acceptance criteria:**
- [ ] Writes within one frame concatenated into single `Uint8Array`, flushed in one `write()` call
- [ ] Writes ≤64 bytes flush immediately (no added latency for keystroke echo)
- [ ] Batching cancelled on disconnect (no stale rAF callbacks)
- [ ] Heavy output (`yes | head -10000`) results in fewer `write()` calls than chunks received

**Dependencies:** Task 3 (or parallel — orthogonal concern)  
**Files:** `cmd/vortex-ui/web/components/vortex-terminal.ts`  
**Scope:** Small

---

### Phase 3: Cleanup & validation

#### Task 5: End-to-end validation

Verify the full path works correctly and remove any dead code in the raw path only.

**Acceptance criteria:**
- [ ] `go test ./internal/...` passes
- [ ] Frontend builds clean
- [ ] Interactive shell + job output both work over raw encoding
- [ ] Opening a terminal with existing buffered history (REST endpoint) still works (base64)
- [ ] Legacy base64 SSE path still works when `?encoding=raw` is omitted

**Dependencies:** Tasks 2, 3, 4  
**Files:** Various  
**Scope:** XS

---

### Checkpoint: After all tasks

- [ ] Both encoding paths work side-by-side
- [ ] No regressions in existing functionality
- [ ] Heavy output renders smoothly
- [ ] Keystroke echo has no perceptible added delay
