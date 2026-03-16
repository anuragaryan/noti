# NotI

NotI is an AI-first note-taking desktop app.

**Note + AI = NotI**: you capture ideas quickly, then use built-in AI to structure, rewrite, summarize, or expand them without leaving your notes workflow.

At a high level, NotI combines:

- local-first note and folder management
- speech-to-text (Whisper) for voice capture
- LLM-powered writing assistance (local models or API provider)
- a native desktop experience built with Wails (Go backend + TypeScript frontend)

## Development

### 1) Install prerequisites (macOS-first)

- Xcode Command Line Tools
  - `xcode-select --install`
- Go 1.24+
- Bun (for frontend dependencies and scripts)
- Wails CLI v2
  - `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### 2) Clone and install dependencies

```bash
git clone <your-fork-or-repo-url>
cd noti
bun install --cwd frontend
```

### 3) Build and test

```bash
./scripts/test.sh
./scripts/build.sh debug
./scripts/build.sh production
```

`./scripts/build.sh debug` generates `build/bin/noti.app`.

`./scripts/build.sh production` generates `build/bin/noti.app` and packages it into `build/bin/noti.dmg`.

### 4) Important local setup note

Current build/test scripts assume a local `whisper.cpp` checkout and macOS CGO flags are available. If your local path differs, update `scripts/build.sh` and `scripts/test.sh` before building.

If build/link errors mention `libwhisper`/`ggml` ABI or deployment target mismatches, rebuild the local whisper artifacts:

```bash
./scripts/rebuild-whisper.sh
```

You can pass a custom whisper checkout path:

```bash
./scripts/rebuild-whisper.sh /absolute/path/to/whisper.cpp
```

## Architecture

NotI is a Wails desktop app with a Go backend and a TypeScript frontend.

- `frontend/`
  - Vite + TypeScript UI + Tailwind CSS
  - stateful note editor, sidebar, settings, prompts, and runtime events
- `main.go`
  - Wails app bootstrap, menu setup, logging, and production error reporting wiring
- `app.go`
  - Wails-bound application methods exposed to the frontend
  - orchestration across notes, folders, config, STT, LLM, prompts, and audio
  - on first run, STT/LLM initialization (and model downloads) is deferred until the Getting Started setup is saved
- `internal/service/`
  - service layer for business logic and lifecycle management
  - includes `STTManager`, `LLMManager`, `AudioManager`, `NoteService`, `FolderService`, and `PromptService`
- `internal/stt/whisper/`
  - real-time Whisper transcription pipeline with VAD, chunking, assembly, and event emission
- `internal/llm/`
  - provider abstraction for local (`llama-server`) and API-based LLMs
  - model/binary download and runtime health management
- `internal/infrastructure/downloader/`
  - model and binary download registry + progress callbacks

Storage defaults:

- Notes: `~/Documents/noti/notes`
- Folder structure metadata: `~/Documents/noti/notes/structure.json`
- App config/models/binaries: user config directory under `Noti` (for example on macOS: `~/Library/Application Support/Noti`)

First-run behavior:

- `config.json` is created from the embedded template on first launch.
- The Getting Started screen is shown and model downloads do not start automatically.
- Model initialization/download begins only after the user clicks save in Getting Started.

## Requirements

macOS-first development environment:

- macOS with microphone permissions enabled for the app/terminal during development
- Go `1.24+`
- Bun `1.x+`
- Wails `v2` CLI
- CGO-enabled Go toolchain
- Native dependencies used by this project:
  - `whisper.cpp` (Go bindings)
  - PortAudio
  - Apple frameworks used in current scripts (Accelerate, Foundation, Metal)

Optional but recommended:

- Local LLM runtime support for `llama-server` model execution
- API endpoint + API key if using API provider mode

## Data & Privacy

NotI is designed to be local-first:

- Notes are stored as local files on your machine.
- STT and local LLM flows can run entirely on-device.
- Model files and binaries are downloaded to local application directories.

When external services are used:

- If you configure an API-based LLM provider, prompts/content sent to that provider leave your machine.
- API credentials are stored in local app configuration.

Good practices:

- Use local provider mode for sensitive notes when possible.
- Treat API keys like secrets and rotate them if exposed.
- Exclude personal note directories and local config from backups/shares if needed.
