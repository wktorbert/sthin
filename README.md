# lean

Boot an iOS simulator or Android emulator already slimmed, from one terminal list with one key, and restore it to stock with one key.

## Commands

```
lean                              TUI (falls back to `lean list` when stdout is not a TTY)
lean list [--json]                simulators and AVDs with state, slim state, footprint
lean boot <id|name> [--stock] [--except cat,cat] [--ram MB] [--headless] [--json]
lean restore <id|name> [--json]   undo exactly what Lean changed
lean shutdown <id|name>
lean measure <id|name> [--json]   host memory footprint of a running device
lean doctor [--json]              toolchain and Profile checks
lean profile <ios|android> [--json]
lean version
```

## TUI

`lean` opens a simutil-style screen: one bordered panel per platform on the left (iOS Simulators, Android Emulators), a Details panel on the right for the selected device (name, ID, platform, OS, state, slim state, memory, warnings), and a status bar with the key map. Dracula colours; a green dot marks a booted device.

| Key | Action |
| --- | --- |
| `↑`/`↓`, `j`/`k` | Move the selection |
| `tab` | Jump to the next platform panel |
| `enter` | Slim boot the selected device and open its window (progress view shows each stage) |
| `c` | Choose which categories stay enabled on this device, then slim boot. Saved per device |
| `w` | Wireless ADB: pair (optional) and connect a phone over Wi-Fi |
| `x` | Run the app in Lean's working directory on the selected device (see Run your app) |
| `l` | Follow the device log live: filter, level, save |
| `r` | Restore to stock, after a y/n confirm |
| `t` | Shut down |
| `?` | Help overlay |
| `q` | Quit |

Enter and `lean boot` open the device window after boot: Simulator.app on Xcode 26 and earlier, DeviceHub.app on Xcode 27 (which no longer ships Simulator.app). `lean doctor` reports which one it found as `ios-simulator-app`; `--headless` skips the window.

If a simulator's data folder has been deleted from disk (a cleanup tool, or a manual delete under `~/Library/Developer/CoreSimulator/Devices`), `simctl` still lists it but it boots to a black screen or refuses to boot. The list flags such devices with `device data missing on disk; run: xcrun simctl erase <udid>`; erase rebuilds the device from the runtime image.

### Logs

`lean logs <id>` prints the last lines of the device log (unified log on iOS, logcat on Android) through one normalised format: time, level, process, message. `--follow` streams live until Ctrl-C. `--filter text`, `--level warn`, `--out file`, and `--json` (one object per line) work in both modes. In the TUI, `l` opens the same stream in a full-width viewer: `/` edits the text filter, `L` cycles the minimum level, `f` toggles follow, `s` saves the filtered buffer to `~/.lean/logs/`, `c` clears, `esc` closes. Lines are batched so a chatty device never stalls the screen; the buffer keeps the last 50,000 lines. Live logs work on simulators, emulators, and Android phones; physical iOS devices only support the snapshot.

### Run your app

`lean run --device <id>` runs the project in the current directory on a device. It boots the device first if needed (with a window), then:

- **Flutter and React Native:** hands off to `flutter run -d <device>` or `npx react-native run-ios --udid` / `run-android --deviceId`, so hot reload and the framework's console work as usual. Quit it the normal way (`q` in Flutter).
- **Xcode and Gradle projects**, or any project with `--no-handoff` or `--app <path>`: installs the newest debug build (`.app` from `build/` or DerivedData, `.apk` from `app/build/outputs/apk/debug`), reads its bundle identifier or package name, launches it, and opens `--url` if given. Pass `--app-id` when the identifier cannot be read (no `aapt` in the SDK's build-tools, for example).

```
lean run -d "iPhone 17"                          # Flutter/RN hand-off
lean run -d Pixel_7_Pro --no-handoff --url myapp://home   # install the built APK, launch, open a deep link
lean run -d <udid> --app build/Debug-iphonesimulator/Demo.app
```

Physical devices are allowed here: the hand-off targets them by serial, and install and launch work through adb or simctl. Over MCP the `run` tool covers the install-and-launch path (build first), and `open_url` opens a link.

### Agents: MCP server and the lease pool

`lean mcp` serves every Lean operation over the Model Context Protocol on stdio. Register it once; Claude Code and any MCP client then get these tools: `devices_list`, `boot`, `shutdown`, `restore`, `measure`, `screenshot`, `tap`, `install`, `launch`, `logs`, `lease`, `release`, `leases`.

```json
{ "mcpServers": { "lean": { "command": "lean", "args": ["mcp"] } } }
```

The agent flow is lease, use, release. `lease` hands out an idle device (a booted one first, otherwise it slim boots one headless) and records a lease with a TTL, so a crashed agent never strands a device: after the TTL the device is free again. `release` drops it, optionally shutting the device down. Physical devices are never leased or booted, but `screenshot`, `install`, `launch`, `logs`, and (Android) `tap` work on them. `tap` is Android only; simctl has no input injection on iOS.

The same pool is on the CLI for scripts and CI:

```
lean lease --platform ios --ttl 20m --owner ci-shard-3 --json   # prints the device and lease
lean leases
lean release <id> --shutdown
lean boot <id> --headless --wait                               # boot always waits; --wait is accepted for scripts
```

Exit codes are stable: 0 success, 1 failure, 2 usage error or unknown device, 3 no device available to lease. `scripts/mcp-client.py` is a tiny stdio client for smoke tests: `python3 scripts/mcp-client.py bin/lean devices_list`.

### Windows

The Android half runs on Windows: listing, slim boot, restore, categories, logs, wireless ADB, `run`, `lease`, and the MCP server, in the TUI and on the CLI. iOS is hidden with the reason "iOS simulators require macOS" and `doctor` reports its checks as not applicable. Lean finds the SDK through `ANDROID_HOME`, `ANDROID_SDK_ROOT`, or Android Studio's default `%LOCALAPPDATA%\Android\Sdk`, using `adb.exe` and `emulator.exe`. The emulator is started in its own process group so it outlives Lean and survives Ctrl-C. Memory in the list is the emulator's working set (PowerShell `Win32_Process`), the nearest equivalent to macOS's phys_footprint; the dirty-RAM figure is macOS-only. On Linux the same paths use `ps` RSS. Every build is cross-compiled for Windows in the harness, but the Android flows have not been run on a Windows machine yet.

### Physical devices

A third panel lists physical phones and tablets: iPhones and iPads paired with this Mac (via `devicectl`, Xcode 15+) and Android devices adb can see over USB or Wi-Fi. They show connected or offline, appear in `lean list` with kind `physical`, and are never slimmed, restored, booted, or shut down by Lean; those keys and commands refuse with a clear message.

Wireless ADB: on the phone, enable Settings › Developer options › Wireless debugging. Pair once with the code shown under "Pair device with pairing code", then connect to the main address:

```
lean adb pair 192.168.1.20:37099 123456
lean adb connect 192.168.1.20:5555
lean adb disconnect 192.168.1.20:5555
```

In the TUI, `w` opens the same flow as a form. QR pairing is not implemented.

### Keeping services an app needs

Slimming turns off whole categories of daemons (see `lean profile ios` / `lean profile android`). If your app needs one of them, keep that category:

```
lean boot <id> --except photos              # this boot only
lean boot <id> --except photos,store --remember   # and make it the device's default
```

In the TUI, press `c` on the device, tick the categories to keep, and press Enter. The choice is saved per device and used by every later slim boot, from the TUI or the CLI, until you change it. Re-applying a changed selection re-enables what Lean had disabled for a newly kept category and leaves everything else as it was. For example, an app that reads the photo library needs `photos` (assetsd, photolibraryd, photoanalysisd); one that uses StoreKit or push needs `store`.

The list refreshes every 2 seconds while idle. Panels scroll to keep the selection visible when the terminal is short; on narrow terminals the Details panel moves below the list.

Exit codes: 0 success, 1 failure, 2 usage error or unknown device. Every `--json` output carries `"schema_version": 1`.

## Desktop app

`desktop/` is a Tauri app for macOS and Windows: a window with the same panels as the TUI, plus a menu-bar (Windows: notification-area) icon to boot, shut down, or restore a device without opening the window. Closing the window keeps Lean in the tray; quit from the tray menu or with Cmd+Q / Ctrl+Q.

It bundles its own `lean` and talks to it through `lean serve`, a JSON-RPC session on stdio with live boot progress, log lines, and device-list updates. The protocol is in [docs/serve-protocol.md](docs/serve-protocol.md); the choice of Tauri over a Go GUI is in [docs/adr/0001-desktop-as-tauri-process-over-serve.md](docs/adr/0001-desktop-as-tauri-process-over-serve.md).

Windows builds are a preview: Android only, and not yet verified on real hardware.

```
# needs Go, pnpm, and a Rust toolchain (rustup)
scripts/build-sidecar.sh          # lean for this host, into desktop/src-tauri/binaries/
cd desktop && pnpm install
pnpm tauri dev                    # run with hot reload
pnpm tauri build                  # .app / .dmg on macOS, NSIS installer on Windows
./verify.sh                       # typecheck, frontend tests, rustfmt, clippy, Rust tests
```

A `v*` tag builds, signs and notarizes the macOS apps, builds the Windows installer, and attaches the `lean` binaries to a draft GitHub Release (`.github/workflows/desktop.yml`).

## Build

```
go build -o bin/lean ./cmd/lean
./verify.sh
```

## How it works

- iOS: Lean writes the simulator's launchd override store while it is shut down, boots once, and reads the disabled set back to confirm. Restore sets every label Lean disabled back to enabled.
- Android: Lean tunes `config.ini` (backup kept), launches the emulator with low-RAM flags, disables the Profile's packages with `pm disable-user`, and records every change so restore can replay it.

Profiles (which daemons and packages get disabled) are embedded YAML under `internal/profile/`. See `NOTICE` for attribution.
