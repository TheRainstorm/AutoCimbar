# Wails GUI Implementation Notes

## Goal

Add a Wails v3 desktop GUI for AutoCimBar while preserving the existing high-performance CLI and native screen renderer.

## Decisions

- The QR/symbol display window stays on the existing native Windows rendering path because it is already optimized for high FPS and avoids WebView repaint overhead.
- The Wails window is only the control dashboard: file picking, receiver output selection, task control, logs, and metrics.
- Sender and receiver are separate backend services. Both can run concurrently in independent goroutines.
- Pause semantics:
  - Sender pause stops the current native encoder window and can resume by starting the same session again.
  - Receiver pause stops screen capture/decode and can resume by starting the same session again.
- Receiver save mode was simplified per requirement: output defaults to current directory. Users may select a directory or type a file path manually.
- The GUI uses Wails v3 services and events. The frontend runtime wrapper dynamically loads `/wails/runtime.js` inside the Wails webview and calls Go services by fully-qualified method name. Standalone Vite preview keeps a mock fallback for UI inspection.
- GUI `ecc=0` remains a valid explicit setting. Only an omitted value falls back to the default ECC.

## Backend Structure

- `cmd/gui/main.go`
  - Wails v3 app bootstrap.
  - Registers services.
  - Configures system tray.
  - Embeds `frontend/dist`.
- `cmd/gui/lite/main.go`
  - Wails v3 Lite app bootstrap.
  - Embeds `cmd/gui/lite/frontend/dist-lite`.
  - Forces a restricted sender configuration in backend services.
- `cmd/gui/internal/backend/app.go`
  - System APIs: file dialog, output directory dialog, screen list, autostart, show/hide/quit, tray menu.
- `cmd/gui/internal/backend/config.go`
  - Default and validation logic for GUI transfer config.
- `cmd/gui/internal/backend/encoder.go`
  - Sender session/task lifecycle.
  - Calls `pkg/app.EncodeFileToScreen`.
  - Runs the encoder in a goroutine and passes a stop channel into the native renderer.
- `cmd/gui/internal/backend/decoder.go`
  - Receiver session/task lifecycle.
  - Calls `pkg/app.DecodeScreenToPath`.
  - Runs capture/decode in a goroutine and passes a stop channel into the screen loop.
- `cmd/gui/internal/backend/logwriter.go`
  - Bridges backend progress logs into Wails events.
  - Parses useful receiver metrics as a first-pass compatibility layer.

## Core App Changes

- `pkg/app.ScreenEncodeConfig` and `pkg/app.ScreenDecodeConfig` now accept an optional `Stop <-chan struct{}`.
- `pkg/app.ErrStopped` is returned when a screen decode is stopped through that channel.
- Native Windows encoder and HTTP encoder backends listen for `Stop` and close/shutdown the display path.

## Frontend Structure

- `cmd/gui/frontend`
  - Vue 3 + TypeScript + TailwindCSS + Vite.
- `src/App.vue`
  - Dark glassmorphism dashboard.
  - Sender and Receiver panels run side by side.
  - Main UI exposes `RQ`, screen selection, and receiver capture backend.
  - Advanced settings accordion separates frame-format settings from runtime settings.
  - Frame-format settings are visually grouped and must match on both sides: backend, cell, ECC, packets, and zstd.
  - Runtime settings expose `B`, `fps`, and a simplified placement selector: four corners or center.
  - Niche CLI-only parameters such as external symbols and decode workers are no longer shown in the GUI.
  - Each visible setting has a hover tooltip aligned with the command-line help.
  - Lite build hides receiver and advanced frame-format controls, exposing only `RQ`, screen, placement, and `B`.
  - GUI defaults are loaded from `~/.autocimbar` using `[default]` plus `[gui]`; old `Q` values are accepted as `RQ` when `RQ` is absent.
- `src/runtime/api.ts`
  - Typed wrapper for Wails services and events.
  - Loads `/wails/runtime.js` at runtime, calls `window.wails.Call.ByName`, and subscribes with `window.wails.Events.On`.
  - Unwraps Wails v3 event payloads from `event.data` before updating Vue state.
  - Includes a mock fallback so the Vite UI can still build and preview outside Wails.

## Implementation Process

1. Added cancellable screen encode/decode configs in `pkg/app`.
2. Wired the Windows native encoder window and non-Windows HTTP fallback to close when the stop channel is closed.
3. Added Wails v3 app bootstrap under `cmd/gui`, with Windows-only build tags and a non-Windows CLI stub.
4. Added backend services:
   - `AppService` for dialogs, display enumeration, autostart, tray/window actions.
   - `ConfigService` for defaults and validation.
   - `EncoderService` and `DecoderService` for concurrent task lifecycle.
5. Added Vue3 + TypeScript + Tailwind frontend with independent Sender and Receiver panels, dark card UI, receiver metrics, logs, system tray controls, and advanced settings.
6. Fixed real Wails frontend calls by using `window.wails.Call.ByName` with fully-qualified service names instead of relying on generated bindings.
7. Rebuilt `frontend/dist` so the Windows GUI binary embeds the latest frontend bundle.
8. Compact layout pass:
   - Removed the large header from the main UI.
   - Moved `RQ` and screen selection to the top.
   - Moved autostart control to the tray menu.
   - Reduced default window size and panel spacing.
9. Changed the GUI size control from raw `Q` to reference `RQ`, wired it through tile-aware grid resolution, added missing advanced parameters, and changed the GUI default packet count to 1.
10. Adjusted window and tray behavior:
    - Closing the main Wails window now exits the app.
    - The top bar has a dedicated `To Tray` button for hiding the dashboard while keeping background tasks alive.
    - Tray `显示` restores, shows, and focuses the main window.
    - Sender and receiver logs stay on one line and scroll horizontally inside their own panels instead of widening the whole window.
11. Added Windows GUI application icon and release automation:
    - `cmd/gui/assets/icon.svg` is the editable icon source.
    - `cmd/gui/assets/icon.png` is embedded through `application.Options.Icon` and used by the system tray.
    - `cmd/gui/assets/icon.ico` is compiled into `cmd/gui/rsrc_windows_amd64.syso` so `gui.exe` has a native Windows file/taskbar icon.
    - `.github/workflows/release.yml` builds Windows amd64 CLI and GUI binaries when a `v*` tag is pushed, then uploads a zip and SHA256 file to GitHub Releases.
12. Promoted DXGI to a first-class receiver capture backend:
    - CLI exposes `-capture-backend auto|dxgi|gdi`.
    - GUI exposes the same backend choice in the main settings row.
    - Help and tooltips document that DXGI is fastest but HDR/color-managed displays can break high color-bit modes.
13. Refined GUI configuration:
    - Frame-format parameters are highlighted as a single group because mismatches cause decode failure.
    - Raw `X:Y` placement is replaced with four corners plus center in the GUI, while CLI keeps exact coordinates.
    - `symbols` and `decode-workers` remain available in the CLI but are hidden from the GUI.
14. Added GUI Lite:
    - Built from the same Vue source with `VITE_AUTOCIMBAR_LITE=1`.
    - Output is embedded from `cmd/gui/lite/frontend/dist-lite`.
    - Backend clamps `RQ` to `<= 40` and fixes frame/capture settings to `capture=gdi`, `cell=8t4s2c`, `ecc=3`, `packets=1`, zstd enabled, and `fps=30`.
    - `B` remains user-adjustable and defaults to `1`.

## Verification

Completed checks:

```bash
export PATH=/usr/local/go/bin:$PATH
go test ./...
cd cmd/gui/frontend && npm run build
GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o bin/gui.exe ./cmd/gui
```

All checks passed on 2026-06-06. The generated Windows binary is `bin/gui.exe`.

The icon resource can be regenerated with:

```bash
convert -background none cmd/gui/assets/icon.svg -resize 256x256 -depth 8 PNG32:cmd/gui/assets/icon.png
convert -background none cmd/gui/assets/icon.svg -depth 8 -define icon:auto-resize=256,128,64,48,32,16 cmd/gui/assets/icon.ico
rsrc -ico cmd/gui/assets/icon.ico -o cmd/gui/rsrc_windows_amd64.syso
```

## Debug Notes

- Wails v3 event callbacks receive an event object. The emitted payload is in `event.data`.
- If the frontend treats the event object as a `SenderSession`, controls later call backend methods with an empty or wrong session id. The backend then reports `sender session not found`.
- The Windows GUI should be built with `-ldflags="-H windowsgui"` to avoid opening a console window.
- The native sender window is created on a locked OS thread. Closing it from another goroutine should post `WM_CLOSE` to that window thread instead of directly calling `DestroyWindow`.
- Windows window classes remain registered for the process lifetime. `ERROR_CLASS_ALREADY_EXISTS` is expected after the first send and is safe to ignore.
- `Show()` alone may not foreground a hidden/minimized window from the tray. The GUI uses `Restore()`, `Show()`, then `Focus()` for the tray `显示` action.

## Follow-up

- Replace progress-log parsing with a structured progress callback from `pkg/app`.
- Generated Wails bindings can be added later, but the current `Call.ByName` path is explicit and works without generated files.
- Add Playwright screenshot verification once the Wails dev runner is available in CI/local workflow.
- Receiver pause/resume preserves the in-memory fountain decoder state by pausing capture/decode instead of stopping the task.
