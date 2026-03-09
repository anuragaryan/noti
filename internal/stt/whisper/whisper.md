# Whisper Transcriber (High-Level)
This package implements real-time speech-to-text (STT) using Whisper and is organized into small components so behavior is easier to reason about, test, and maintain.
## What the transcriber does
At runtime, the transcriber:
1. receives audio chunks from capture (`ProcessChunk`)
2. filters invalid chunks (sample rate / channel checks)
3. performs lightweight voice activity detection (VAD)
4. buffers audio until a pause or max segment duration is reached
5. transcribes accepted chunks with Whisper
6. builds a cumulative transcript and emits partial updates
7. on stop, finalizes remaining tail audio and emits a final event
It is designed to be asynchronous and thread-safe for real-time streaming.
---
## Main components
## `transcriber.go` (orchestrator)
`Transcriber` is the facade used by the service layer. It owns lifecycle, shared state, goroutines, synchronization, and coordination across all helper components.
Public lifecycle/API methods:
- `NewTranscriber(...)` checks model file path and initializes defaults
- `SetContext(ctx)` configures event emission target (Wails/no-op)
- `Initialize()` loads Whisper model and creates engine
- `StartProcessing()` starts realtime loop and resets session
- `ProcessChunk(chunk)` validates and ingests audio
- `StopProcessing()` stops loop and schedules async finalization
- `CancelProcessing()` stops without emitting final result
- `Cleanup()` waits in-flight work and closes resources
Internal orchestration highlights:
- realtime loop runs with ticker (`PauseCheckInterval`)
- chunk processing happens in worker goroutines
- finalization runs in background after stop
- transcript ordering is preserved via model serialization + controlled accumulation
---
## `options.go` (configuration)
Defines tunable behavior via `Options` and `DefaultOptions()`:
- sample rate and timing windows
- VAD threshold bounds
- baseline warmup size
- minimum chunk/tail sizes
- whisper language/thread settings
Also defines `Clock` abstraction for deterministic tests (`realClock` in production).
---
## `engine.go` (Whisper boundary)
Defines `speechEngine` abstraction and `whisperEngine` implementation.
Responsibilities:
- create Whisper context
- set language/threads/translate flags
- process audio
- iterate segments into final text
- close model resource
The transcriber talks to this interface rather than hardcoding Whisper internals everywhere.
---
## `emitter.go` (event boundary)
Defines `transcriptionEmitter` abstraction:
- `EmitPartial(...)`
- `EmitDone(...)`
Implementations:
- `wailsEmitter` sends runtime events to frontend
- `noopEmitter` for nil-context or non-UI contexts
This keeps UI event concerns out of core transcription logic.
---
## `vad.go` (speech detection)
Handles adaptive VAD:
- `updateBaseline(...)`
  - collects initial ambient samples
  - computes noise floor RMS
  - sets initial speech threshold (clamped)
- `updateSpeechState(...)`
  - updates last speech timestamp when chunk is above threshold
  - continuously adapts threshold from observed audio
- `clampSpeechThreshold(...)`
  - enforces safe min/max threshold limits
---
## `chunker.go` (segmentation policy)
Decides when/how to transcribe:
- `shouldTranscribe(now)` returns:
  - whether to transcribe now
  - whether pause-triggered
  - joiner (`" "` or `"\n"`)
  - measured silence duration
- `takeNextChunk()`:
  - copies unprocessed audio segment
  - applies minimum size guard
  - advances processed pointer
  - resets segment markers
---
## `assembler.go` (text assembly)
Keeps transcript output clean and readable:
- `cleanTranscription(...)` removes model artifacts (`[BLANK_AUDIO]`) and trims
- `joinTranscript(...)` appends chunk text with whitespace normalization
- `joinerFromPause(...)` chooses newline for longer-than-average pauses, else space
---
## `session_state.go` (session snapshots/state reset)
Encapsulates per-session mutable state operations:
- `resetSession(now)` clears all recording/session state
- `stopSnapshotLocked(now)` captures immutable snapshot for finalization
- `stopSnapshot` stores buffer/progress/pause statistics for safe background processing
---
## Concurrency model (important)
The transcriber uses separate locks for separate concerns:
- `lifecycleMu`:
  - serializes start/stop/cancel/cleanup lifecycle transitions
- `bufferMutex`:
  - protects session mutable state (buffer pointers, VAD state, transcript)
- `modelMutex`:
  - serializes access to Whisper inference path
- `emitterMu`:
  - protects context/emitter swaps and reads
Wait groups:
- `realtimeWg`: realtime loop goroutine
- `processingWg`: chunk transcription workers
- `backgroundWg`: stop/finalization worker
This separation keeps lock scope focused and reduces deadlock risk.
---
## End-to-end flow
1. **Initialize**
   - model file exists
   - Whisper model loaded
2. **Start**
   - previous session is drained/stopped if needed
   - state reset
   - realtime loop starts
3. **Stream**
   - chunks validated and appended
   - baseline/VAD updated
   - on pause/max-duration, a chunk is extracted and transcribed
   - partial transcript emitted
4. **Stop**
   - realtime loop is stopped
   - snapshot captured
   - background finalization:
     - waits in-flight chunk workers
     - transcribes tail if large enough
     - optional fallback full-buffer pass if nothing transcribed
   - final transcript emitted
5. **Cleanup**
   - all goroutines drained
   - engine/model closed
---
## Output behavior
- Partial updates are emitted during recording (`transcription:partial`)
- Final update is emitted after stop (`transcription:done`)
- If no context is configured, events are skipped safely and logged
- Invalid chunk metadata is dropped (warn logged) to avoid corrupting state
---
## Why this structure
This layout follows a “thin orchestrator + focused helpers” design:
- easier to read
- easier to test per concern
- easier to evolve (swap policy/engine/emitter independently)
- fewer hidden side effects across unrelated responsibilities