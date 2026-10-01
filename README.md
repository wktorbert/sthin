# sthin

Boot an iOS simulator or Android emulator already slimmed, from one terminal list with one key, and restore it to stock with one key.

## Install

Homebrew (macOS and Linux):

```
brew install wktorbert/tap/sthin
```

With Go 1.26 or newer:

```
go install github.com/wktorbert/sthin/cmd/sthin@latest
```

Or download an archive from the [releases page](https://github.com/wktorbert/sthin/releases) for macOS (arm64, amd64), Linux (amd64, arm64) or Windows (amd64), and verify it against `checksums.txt`:

```
shasum -a 256 -c --ignore-missing checksums.txt
```

Releases are cut from a `v*` tag: the desktop workflow builds and signs the app, and the same tag attaches the CLI archives, the checksum file, Homebrew bottles for macOS and Linux, and updates the Homebrew tap (`scripts/release-cli.sh` and `scripts/homebrew-formula.sh` are what run there, and both work locally). The bottles are what let `brew install` work without Xcode Command Line Tools or a compiler: Homebrew pours them instead of treating the formula as a source build.

### Shell completions

Homebrew installs them. Otherwise, with the binary on your PATH:

```
# zsh (add before compinit in ~/.zshrc)
echo 'source <(sthin completion zsh)' >> ~/.zshrc
# bash
echo 'source <(sthin completion bash)' >> ~/.bashrc
# fish
sthin completion fish > ~/.config/fish/completions/sthin.fish
# PowerShell
sthin completion powershell | Out-String | Invoke-Expression
```

Release archives carry the same scripts in `completions/`. Completion knows your devices: `sthin boot <TAB>` offers simulators and AVDs by name and ID with their OS and state, filtered to what the command accepts (no physical devices for `boot`, only booted ones for `measure`, only shut-down ones for `delete`). A name shared by several devices, such as the same iPad across two runtimes, is offered by ID only. `--except <TAB>` lists the categories of the named device's platform, `--level` and `--platform` their values. The device list is cached for 30 seconds under the Sthin home and dropped by any command that changes a device.

## Commands

```
sthin                              TUI (falls back to `sthin list` when stdout is not a TTY)
sthin list [--json]                simulators and AVDs with state, slim state, footprint
sthin boot <id|name> [--stock] [--except cat,cat] [--ram MB] [--headless] [--json]
sthin restore <id|name> [--json]   undo exactly what Sthin changed
sthin shutdown <id|name>
sthin delete <id|name> [--yes]     delete a simulator or AVD for good (asks first)
sthin measure <id|name> [--json]   host memory footprint of a running device
sthin doctor [--json]              toolchain and Profile checks
sthin profile <ios|android> [--json]
sthin version
```

## TUI

`sthin` opens a simutil-style screen: one bordered panel per platform on the left (iOS Simulators, Android Emulators), a Details panel on the right for the selected device (name, ID, platform, OS, state, slim state, memory, warnings), and a status bar with the key map. Dracula colours; a green dot marks a booted device.

| Key | Action |
| --- | --- |
| `↑`/`↓`, `j`/`k` | Move the selection |
| `tab` | Jump to the next platform panel |
| `enter` | Slim boot the selected device and open its window (progress view shows each stage) |
| `c` | Choose which categories stay enabled on this device, then slim boot. Saved per device |
| `w` | Wireless ADB: pair (optional) and connect a phone over Wi-Fi |
| `x` | Run the app in Sthin's working directory on the selected device (see Run your app) |
| `l` | Follow the device log live: filter, level, save |
| `o` | Android launch options for the AVD: cold boot, audio, low-RAM, headless, RAM. Saved per AVD |
| `n` | Rename the simulator or AVD |
| `r` | Restore to stock, after a y/n confirm |
| `t` | Shut down |
| `?` | Help overlay |
| `q` | Quit |

Enter and `sthin boot` open the device window after boot: Simulator.app on Xcode 26 and earlier, DeviceHub.app on Xcode 27 (which no longer ships Simulator.app). `sthin doctor` reports which one it found as `ios-simulator-app`; `--headless` skips the window.

If a simulator's data folder has been deleted from disk (a cleanup tool, or a manual delete under `~/Library/Developer/CoreSimulator/Devices`), `simctl` still lists it but it boots to a black screen or refuses to boot. The list flags such devices with `device data missing on disk; run: xcrun simctl erase <udid>`; erase rebuilds the device from the runtime image.

### Logs

`sthin logs <id>` prints the last lines of the device log (unified log on iOS, logcat on Android) through one normalised format: time, level, process, message. `--follow` streams live until Ctrl-C. `--filter text`, `--level warn`, `--out file`, and `--json` (one object per line) work in both modes. In the TUI, `l` opens the same stream in a full-width viewer: `/` edits the text filter, `L` cycles the minimum level, `f` toggles follow, `s` saves the filtered buffer to `~/.sthin/logs/`, `c` clears, `esc` closes. Lines are batched so a chatty device never stalls the screen; the buffer keeps the last 50,000 lines. Live logs work on simulators, emulators, and Android phones; physical iOS devices only support the snapshot.

### Run your app

`sthin run --device <id>` runs the project in the current directory on a device. It boots the device first if needed (with a window), then:

- **Flutter and React Native:** hands off to `flutter run -d <device>` or `npx react-native run-ios --udid` / `run-android --deviceId`, so hot reload and the framework's console work as usual. Quit it the normal way (`q` in Flutter).
- **Xcode and Gradle projects**, or any project with `--no-handoff` or `--app <path>`: installs the newest debug build (`.app` from `build/` or DerivedData, `.apk` from `app/build/outputs/apk/debug`), reads its bundle identifier or package name, launches it, and opens `--url` if given. Pass `--app-id` when the identifier cannot be read (no `aapt` in the SDK's build-tools, for example).

```
sthin run -d "iPhone 17"                          # Flutter/RN hand-off
sthin run -d Pixel_7_Pro --no-handoff --url myapp://home   # install the built APK, launch, open a deep link
sthin run -d <udid> --app build/Debug-iphonesimulator/Demo.app
```

Physical devices are allowed here: the hand-off targets them by serial, and install and launch work through adb or simctl. Over MCP the `run` tool covers the install-and-launch path (build first), and `open_url` opens a link.

### Agents: MCP server and the lease pool

`sthin mcp` serves every Sthin operation over the Model Context Protocol on stdio. Register it once; Claude Code and any MCP client then get these tools: `devices_list`, `boot`, `shutdown`, `restore`, `measure`, `screenshot`, `tap`, `install`, `launch`, `logs`, `lease`, `release`, `leases`.

```json
{ "mcpServers": { "sthin": { "command": "sthin", "args": ["mcp"] } } }
```

The agent flow is lease, use, release. `lease` hands out an idle device (a booted one first, otherwise it slim boots one headless) and records a lease with a TTL, so a crashed agent never strands a device: after the TTL the device is free again. `release` drops it, optionally shutting the device down. Physical devices are never leased or booted, but `screenshot`, `install`, `launch`, `logs`, and (Android) `tap` work on them. `tap` is Android only; simctl has no input injection on iOS.

The same pool is on the CLI for scripts and CI:

```
sthin lease --platform ios --ttl 20m --owner ci-shard-3 --json   # prints the device and lease
sthin leases
sthin release <id> --shutdown
sthin boot <id> --headless --wait                               # boot always waits; --wait is accepted for scripts
```

Exit codes are stable: 0 success, 1 failure, 2 usage error or unknown device, 3 no device available to lease. `scripts/mcp-client.py` is a tiny stdio client for smoke tests: `python3 scripts/mcp-client.py bin/sthin devices_list`.

### Naming devices

`sthin rename <id> "<name>"` renames a simulator (`simctl rename`, immediate) or an AVD (`avd.ini.displayname` in its `config.ini`, shown from the emulator's next start). `sthin boot <id> --name "<name>"` renames first and then boots, so the window opens under the new name; in the TUI, `n` opens a rename box. The AVD's id and Sthin's saved preferences are unaffected. Physical devices are never renamed.

### Deleting devices

`sthin delete <id>` removes a simulator (`simctl delete`) or an AVD (its `.avd` folder and `.ini`, what `avdmanager delete avd` does, without needing Java) for good, together with Sthin's saved preferences, change record and any lease on it. It asks first; `--yes` skips the prompt for scripts, and a cancelled prompt exits 2. The device must be shut down. In the TUI, `d` asks y/n on the selected row; agents have the `delete` MCP tool and `serve` method. Physical devices are never deleted.

### Android launch options

Per AVD, Sthin remembers how you like the emulator started: `--cold-boot` (ignore the quick-boot snapshot), `--audio` (keep audio on; off by default), `--lowram` (pass `-lowram`; off by default because some images fail to load their initrd with it), `--headless`, and `--ram <MB>`. Pass them on `sthin boot` for one boot, add `--remember` to make them the AVD's defaults, or press `o` on the AVD in the TUI to set them in a dialog. Later boots from the TUI, the CLI, MCP, or the desktop app use the saved switches; any flag given explicitly wins for that boot. The Details panel shows the saved set.

```
sthin boot Pixel_7_Pro --cold-boot --audio --ram 1536 --remember
```

### Windows

The Android half runs on Windows: listing, slim boot, restore, categories, logs, wireless ADB, `run`, `lease`, and the MCP server, in the TUI and on the CLI. iOS is hidden with the reason "iOS simulators require macOS" and `doctor` reports its checks as not applicable. Sthin finds the SDK through `ANDROID_HOME`, `ANDROID_SDK_ROOT`, or Android Studio's default `%LOCALAPPDATA%\Android\Sdk`, using `adb.exe` and `emulator.exe`. The emulator is started in its own process group so it outlives Sthin and survives Ctrl-C. Memory in the list is the emulator's working set (PowerShell `Win32_Process`), the nearest equivalent to macOS's phys_footprint; the dirty-RAM figure is macOS-only. On Linux the same paths use `ps` RSS. Every build is cross-compiled for Windows in the harness, but the Android flows have not been run on a Windows machine yet.

### Physical devices

A third panel lists physical phones and tablets: iPhones and iPads paired with this Mac (via `devicectl`, Xcode 15+) and Android devices adb can see over USB or Wi-Fi. They show connected or offline, appear in `sthin list` with kind `physical`, and are never slimmed, restored, booted, or shut down by Sthin; those keys and commands refuse with a clear message.

Wireless ADB: on the phone, enable Settings › Developer options › Wireless debugging. Pair once with the code shown under "Pair device with pairing code", then connect to the main address:

```
sthin adb pair 192.168.1.20:37099 123456
sthin adb connect 192.168.1.20:5555
sthin adb disconnect 192.168.1.20:5555
```

In the TUI, `w` opens the same flow as a form. QR pairing is not implemented.

### Keeping services an app needs

Slimming turns off whole categories of daemons (see `sthin profile ios` / `sthin profile android`). If your app needs one of them, keep that category:

```
sthin boot <id> --except photos              # this boot only
sthin boot <id> --except photos,store --remember   # and make it the device's default
```

In the TUI, press `c` on the device, tick the categories to keep, and press Enter. The choice is saved per device and used by every later slim boot, from the TUI or the CLI, until you change it. Re-applying a changed selection re-enables what Sthin had disabled for a newly kept category and leaves everything else as it was. For example, an app that reads the photo library needs `photos` (assetsd, photolibraryd, photoanalysisd); one that uses StoreKit or push needs `store`.

The list refreshes every 2 seconds while idle. Panels scroll to keep the selection visible when the terminal is short; on narrow terminals the Details panel moves below the list.

Exit codes: 0 success, 1 failure, 2 usage error or unknown device. Every `--json` output carries `"schema_version": 1`.

## Desktop app

`desktop/` is a Tauri app for macOS and Windows: a window with the same panels as the TUI, plus a menu-bar (Windows: notification-area) icon to boot, shut down, or restore a device without opening the window. Closing the window keeps Sthin in the tray; quit from the tray menu or with Cmd+Q / Ctrl+Q.

It bundles its own `sthin` and talks to it through `sthin serve`, a JSON-RPC session on stdio with live boot progress, log lines, and device-list updates. The protocol is in [docs/serve-protocol.md](docs/serve-protocol.md); the choice of Tauri over a Go GUI is in [docs/adr/0001-desktop-as-tauri-process-over-serve.md](docs/adr/0001-desktop-as-tauri-process-over-serve.md).

Windows builds are a preview: Android only, and not yet verified on real hardware.

```
# needs Go, pnpm, and a Rust toolchain (rustup)
scripts/build-sidecar.sh          # sthin for this host, into desktop/src-tauri/binaries/
cd desktop && pnpm install
pnpm tauri dev                    # run with hot reload
pnpm tauri build                  # .app / .dmg on macOS, NSIS installer on Windows
./verify.sh                       # typecheck, frontend tests, rustfmt, clippy, Rust tests
```

A `v*` tag builds, signs and notarizes the macOS apps, builds the Windows installer, and attaches the `sthin` binaries to a draft GitHub Release (`.github/workflows/desktop.yml`).

## Renamed from lean

The tool was called `lean` until 2026-09-27. The binary, the MCP server name, the TUI and the desktop app now say `sthin`. State moved from `~/.lean` to `~/.sthin`: the first run renames the old folder if the new one does not exist, `LEAN_HOME` still works as a fallback for `STHIN_HOME`, and Android `config.ini.lean.bak` backups are adopted as `.sthin.bak` on the next slim boot or restore. Reinstall the binary (`go install ./cmd/sthin`) and re-register the MCP server as `sthin mcp`.

## Build

```
go build -o bin/sthin ./cmd/sthin
./verify.sh
```

## How it works

- iOS: Sthin writes the simulator's launchd override store while it is shut down, boots once, and reads the disabled set back to confirm. Restore sets every label Sthin disabled back to enabled.
- Android: Sthin tunes `config.ini` (backup kept), launches the emulator with low-RAM flags, disables the Profile's packages with `pm disable-user`, and records every change so restore can replay it.

Profiles (which daemons and packages get disabled) are embedded YAML under `internal/profile/`. See `NOTICE` for attribution.
