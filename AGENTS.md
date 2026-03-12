## Overview

**Noti** is a personal note-taking desktop application built with **Wails (Go)** for the backend and **TypeScript/Vite** for the frontend. It combines traditional note organization with voice recording and AI capabilities, allowing users to capture thoughts via microphone and process them using LLMs.

---

## Tech Stack

| Layer | Technology |
|-------|-------------|
| **Framework** | Wails v2 (Go) |
| **Backend** | Go |
| **Frontend** | Vanilla TypeScript + Vite |
| **Package Manager** | Bun |
| **Build Target** | macOS/Desktop |

---

## Common Commands

- To test use: ./scripts/test.sh --no-cache
- To build the debug build: ./scripts/build.sh debug
- To build the prod build: ./scripts/build.sh
