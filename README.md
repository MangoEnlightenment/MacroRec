# MacroRec

A simple, good-looking macro recorder for Windows. Record your keyboard, mouse clicks, scroll wheel and camera movement, then replay them with one key. No command line, no code injection.

## Features

- **Records everything you do:** keys (including WASD, modifiers and combos), mouse buttons, scroll wheel and raw relative mouse movement, so camera turns replay properly in games.
- **Native Windows window:** dark theme, smooth animations, live timer and activity bars. No console.
- **Hotkeys that work while another app is in front:** `F8` records, `F9` plays.
- **Two replay modes:**
  - **Game** replays relative mouse movement (camera).
  - **Desktop** replays exact cursor positions (menus and clicking in fixed spots).
- **Starts from the same place every time:** remembers where your mouse was when recording began and returns there before each run and each loop.
- **Mouse-bump protection:** if you bump the real mouse during playback, choose **Off**, **Stop**, or **Fix**. Fix snaps the cursor back onto the macro's path and keeps going (a click still stops it).
- **Speed, repeat and start delay:** 0.25x to 4x speed, 1 to 99 repeats or forever, and a countdown so you can switch windows.
- **Never leaves keys stuck:** anything still held is released when playback stops.
- **Open / Save as:** macros are plain text files. Your last recording is saved automatically and loads next time.
- **Pin on top** so the window stays visible over your game.

## Download

Get `MacroRec.exe` from the [Releases](../../releases) page (Windows 64-bit). No installer: unzip and double-click.

The exe is unsigned, so Windows SmartScreen may warn about an unknown publisher. You can check the source and build it yourself (below).

## How to use

1. Open `MacroRec.exe`.
2. Press **F8** (or the Record button) to start recording, then **F8** again to stop.
3. Press **F9** (or the Play button) to replay, and **F9** again to stop.
4. Adjust mode, speed, repeat, start delay and mouse-bump behavior in the window.

Tips:
- Use the hotkeys rather than the buttons while recording, so button clicks don't end up in the macro.
- For clicking at fixed spots, use **Desktop** mode and keep the target window the same size and position as when you recorded.
- If the target app runs as administrator, run MacroRec as administrator too, or Windows will block the replayed input.

## How it works (and what it doesn't do)

MacroRec only uses two standard Windows features:

- **Raw Input** to watch your keyboard and mouse while recording.
- **SendInput** to replay them, the same method used by tools like AutoHotkey and TinyTask.

It does **not** open other programs' processes, read or write their memory, inject DLLs, hook into them, or use the network.

## Security: only open macro files you trust

A macro file is a list of key presses and clicks that MacroRec performs on your PC **as if you typed them**. A file from a stranger could open programs, type commands, change settings or delete files, and it would run with your permissions.

- Only open macro files you made yourself, or that came from people you know and trust.
- MacroRec checks that a file is a valid macro and rejects malformed files, but it **cannot tell whether what a macro does is safe**.
- MacroRec shows a warning popup on first launch and every time you open a file.
- Recording captures every key you press, including passwords, while it is running. Only record when you mean to, and keep macro files private.

## A note on games and online services

Many online games and services ban automated input in their rules or detect it with anti-cheat. Using any macro tool, including this one, can get an account banned. Check the rules of the game or service before you use it. Single-player and offline use is the safest.

## Build from source

You need [Go](https://go.dev/dl/) 1.24 or newer. The app is Windows-only but can be cross-compiled from any system.

```bash
# build the windowed app (the icon is embedded from rsrc_windows_amd64.syso)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-H windowsgui -s -w" -o MacroRec.exe .

# run the tests (they cover the platform-independent code)
go test ./...
```

On Windows PowerShell:

```powershell
$env:CGO_ENABLED = "0"
go build -ldflags="-H windowsgui -s -w" -o MacroRec.exe .
```

To regenerate the icon from the logo, see `tools/render.js` and `tools/build_res.py`.

## Project layout

| File | Purpose |
| --- | --- |
| `core.go`, `core_extra.go` | Macro file format, input decoding, validation (platform-independent, tested) |
| `engine_windows.go` | Recording with Raw Input, playback with SendInput, mouse-bump handling |
| `ui_windows.go`, `ui_draw_windows.go`, `gdip_windows.go` | Window, input handling, animation and anti-aliased drawing (GDI+) |
| `win_windows.go`, `main_windows.go` | Windows API bindings and app entry point |
| `rsrc_windows_amd64.syso`, `MacroRec.ico` | Embedded app icon |

## Where your data goes

Your last recording is saved to `%AppData%\MacroRec\last-macro.txt`. Nothing is sent anywhere.

## License

Add a license of your choice (for example MIT) as a `LICENSE` file.
