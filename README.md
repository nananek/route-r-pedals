# route-r-pedals

Linux configurator for Route-R USB foot pedal switches (RI-FP1MG, RI-FP3MG, and other devices in the same "FootSwitch" firmware family). The vendor's official configuration utility is Windows-only — this project is a small Go CLI that speaks the same wire protocol so the pedal can be reprogrammed from Linux.

> **⚠️  Unofficial third-party project.** This software is not affiliated with, endorsed by, sponsored by, or in any way connected to Route-R Co., Ltd. or any of its affiliates. All product names referenced here are trademarks of their respective owners and are used solely to identify the hardware this project interoperates with. The wire protocol was determined by independent interoperability analysis of the publicly distributed Windows configuration utility; **no source code, binaries, or documentation from that utility is redistributed by this project.** Use at your own risk.

---

## Status

Working and verified against a real RI-FP1MG (firmware string `FootSwitch1-F3.4`):

- ✅ Read device version + current per-slot bindings
- ✅ Write single keys with optional modifiers (`win+z`, `ctrl+shift+t`, `f13`, …)
- ✅ Write mouse button / movement / wheel actions
- ✅ Write string-typing actions (with optional trailing Enter)
- ✅ Write multimedia keys (volume, play/pause, browser navigation, …)
- ✅ Write gamepad directions / buttons
- ✅ Clear a slot
- ✅ Flash settings to NVRAM (persists across reboots and across hosts)

What's *not* implemented:

- The Windows tool's "Iswuchong" multi-non-modifier-key macro mode (Combin 3 with state byte 0x06 — niche; can be added if anyone needs it)
- macOS / Windows ports (Linux-only by design — Linux's `hidraw` is what makes this so simple)

## Hardware compatibility

The CLI looks for USB VID `413d` PID `2107`, which is the device ID used across the Route-R "FootSwitch" series (and possibly some related multi-key handheld devices that share the firmware). The exact slot count is read from the firmware version string at startup and currently understands these prefixes:

| Version-string substring | Slots |
|---|---|
| `FootSwitch2`               | 2 |
| `FootSwitch3` / `USBswitch3` / `FootSwitch1` / `FootSwitchF` / `FS_sylvac1` / `FS2016` / `USW3V` / `USW1V` | 3 |
| `HK4-` / `USB5Key`          | 4 |
| `HK6-` / `USW6V` / `HandKey`| 6 |
| `DIY-`                      | 14 |

If your physical device has fewer pedals than the firmware reports (the RI-FP1MG used during development reports as `FootSwitch1` = 3 slots even though only one pedal exists), the unused slots can still be read and written — they simply don't have a physical button wired to them.

## Build

The recommended build path is via the bundled `Makefile`, which uses an ephemeral `golang:*-alpine` container so you don't need a Go toolchain on the host:

```sh
make build         # produces ./route-r-pedals (static x86_64 Linux binary)
make test          # runs the protocol unit tests
```

If you have Go ≥ 1.21 installed locally:

```sh
go build -trimpath -ldflags="-s -w" -o route-r-pedals .
go test ./...
```

## Permissions

Reading or writing the configuration interface requires opening `/dev/hidraw*`, which is root-owned by default. Two options:

**Option A — just use sudo:**

```sh
sudo ./route-r-pedals info
sudo ./route-r-pedals set 1 win+z --short
```

**Option B — install a udev rule** so any user in the `plugdev` group can access the pedal without sudo:

```sh
sudo tee /etc/udev/rules.d/70-route-r-pedals.rules <<'EOF'
SUBSYSTEM=="hidraw", ATTRS{idVendor}=="413d", ATTRS{idProduct}=="2107", MODE="0660", GROUP="plugdev"
EOF
sudo udevadm control --reload-rules
sudo udevadm trigger
# then unplug + replug the pedal
```

After that, any member of `plugdev` can run `./route-r-pedals` without sudo.

## Usage

### Inspect current state

```sh
sudo ./route-r-pedals
# or
sudo ./route-r-pedals info
```

Output (example, single-pedal RI-FP1MG with pedal 2 mapped to Win+Z):

```
Hidraw candidates for 413d:2107:
  /dev/hidraw1  if=0
  /dev/hidraw2  if=1
Using: /dev/hidraw2 (interface 1)

== ReadVersion (0x83) ==
  [0] 466f6f7453776974  'FootSwit'
  [1] 6368312d46332e34  'ch1-F3.4'
Decoded version: "FootSwitch1-F3.4"
Pedal slots    : 3 (firmware-reported; physical pedal count may be smaller)

== ReadPedal (0x82) ==
  pedal 1: 0801000400000000   key a (longpress)
  pedal 2: 0881081d00000000   key Win+z (shortpress)
  pedal 3: 0801000600000000   key c (longpress)
```

### Set a single key (with optional modifiers)

```sh
sudo ./route-r-pedals set 2 ctrl+z --short        # Ctrl+Z, single press per pedal-down
sudo ./route-r-pedals set 1 f13                   # F13, longpress (auto-repeats while held)
sudo ./route-r-pedals set 3 ctrl+shift+t          # browser reopen-tab
```

Modifier names accepted: `ctrl` / `control`, `shift`, `alt` / `opt` / `option`, `win` / `windows` / `super` / `meta` / `cmd`. Key names follow the USB HID Keyboard usage list (a–z, 0–9, `f1`..`f24`, `enter`, `esc`, `space`, arrows, etc.).

`--short` switches the firmware state from longpress (auto-repeat) to single-press-per-actuation. `--long` is the default.

### Mouse

```sh
sudo ./route-r-pedals set 1 mouse --btn 1                  # left click
sudo ./route-r-pedals set 1 mouse --btn 0 --wheel 1        # scroll up one tick
sudo ./route-r-pedals set 1 mouse --btn 0 --dx 10 --dy -5  # relative move
```

`--btn` is a bitmask: 1 = left, 2 = right, 4 = middle. `--dx` / `--dy` / `--wheel` are signed bytes (-128 to 127).

### String typing

```sh
sudo ./route-r-pedals set 1 string "hello world"
sudo ./route-r-pedals set 1 string "myemail@example.com" --enter
sudo ./route-r-pedals set 1 string "x" --longpress
```

Supports printable ASCII (the firmware doesn't have a Unicode IME path). Use `--enter` to append a final Return. `--longpress` switches the firmware state byte from `0x04` to `0x06`.

### Multimedia keys

```sh
sudo ./route-r-pedals set 1 media play
sudo ./route-r-pedals set 1 media vol+
sudo ./route-r-pedals set 1 media mute
sudo ./route-r-pedals media-list                 # see all supported names
```

### Gamepad directions / buttons

```sh
sudo ./route-r-pedals set 1 gamepad left
sudo ./route-r-pedals set 1 gamepad button1
```

Names: `left`, `right`, `up`, `down`, `button1` … `button8` (aliases `b1`..`b8`).

### Clear / flash

```sh
sudo ./route-r-pedals clear 1                    # reset slot 1 to "no action"
sudo ./route-r-pedals flash                      # commit any pending writes to NVRAM
```

`set` and `clear` automatically flash by default. Pass `--no-flash` if you want to batch several writes and flash at the end.

### Common flags for `set` / `clear`

| Flag | Effect |
|---|---|
| `--dry-run` | Print the packet bytes that would be sent and exit. No I/O, no root needed. |
| `--no-flash` | Write the new binding but skip the trailing `Flash` (`0x80`) packet. |
| `--short` (single-key only) | Use shortpress state byte (`0x81` for keys, `0x06` for strings). |

## Protocol reference

For posterity, below is a compact summary of the discovered wire protocol. This is a description of facts about how the device behaves; it is not a reproduction of any vendor source code.

The configuration interface is **interface 1** of the device's USB descriptor, exposed on Linux as `/dev/hidraw*`. It is disguised as a HID Mouse but has both `IN` and `OUT` interrupt endpoints with `wMaxPacketSize = 8`. The Windows utility uses `HidD_SetOutputReport` for writes; on Linux the equivalent is a plain `write(2)` to the hidraw node.

All packets are 9 bytes when written via the Windows / Linux HID API (the leading byte is the report-ID byte, always `0x00` since the descriptor doesn't define IDs). Linux strips that byte on the wire, so the device sees and returns 8 bytes.

### Frame format (write side, 9-byte buffer)

```
[0] 0x00      report ID (always zero)
[1] 0x01      frame marker for command headers; 0x08 for data continuations
[2] opcode    see table
[3] length    typically 0x08; varies for string-typing
[4..8]        opcode-specific payload
```

### Opcodes

| `[2]` | Direction | Meaning |
|---|---|---|
| `0x80` | host → device | Flash (commit current settings to NVRAM) |
| `0x81` | host → device | Write a slot binding (header packet, followed by a data packet) |
| `0x82` | host → device | Read slot binding (`[4]` = slot number, response is one IN report) |
| `0x83` | host → device | Read firmware version string (response is up to 3 IN reports) |
| `0x85` | host → device | Read model |
| `0x86` | host → device | Read press model |

### Write sequence

To assign a binding to slot N, send two packets back-to-back with a ~200 ms gap between them:

1. **Header**: `00 01 81 <length> <N> 00 00 00 00`
2. **Data**: layout depends on the binding type ("Combin"); see below.

Then optionally send the **Flash** packet `00 01 80 08 01 00 00 00 00` to persist.

### Data-packet layouts ("Combin" types)

All values shown are the 9-byte buffer; on the wire (and in `/dev/hidraw*` reads on Linux) the leading `0x00` is dropped, giving 8 bytes. Indices in this table refer to the 9-byte form.

| Type | `[2]` (state) | `[3]` | `[4]` | `[5]` | `[6]` | `[7]` | `[8]` |
|---|---|---|---|---|---|---|---|
| Clear                   | `0x01`           | 0   | 0       | 0    | 0  | 0  | 0     |
| Single key longpress    | `0x01`           | mod | keycode | 0    | 0  | 0  | 0     |
| Single key shortpress   | `0x81`           | mod | keycode | 0    | 0  | 0  | 0     |
| Mouse                   | `0x02`           | 0   | 0       | btn  | dx | dy | wheel |
| String typing shortpress | `0x04`          | char | char | char | char | char | char |
| String typing longpress  | `0x06`          | char | char | char | char | char | char |
| Multimedia              | `0x07`           | code | 0   | 0    | 0  | 0  | 0     |
| Gamepad                 | `0x08`           | code | 0   | 0    | 0  | 0  | 0     |

Modifier mask: `Ctrl=1`, `Shift=2`, `Alt=4`, `Win=8`. OR them together for combos.

Mouse `btn` is a bitmask (`1=L, 2=R, 4=M`); `dx`, `dy`, `wheel` are signed 8-bit deltas.

### String typing length and continuation

For Combin 4 / 6 the header `[3]` and data `[1]` carry the total payload length `num5 = (chars) + (1 if --enter) + 2`. The first data packet carries up to 6 chars at `[3..8]`; if more chars are needed, additional 9-byte continuation packets follow, each carrying up to 8 chars at `[1..8]`. If `--enter` is set, HID Usage `0x28` (Enter) is appended after the last user-supplied char.

The string-typing character byte itself is a custom encoding (not raw HID Usage): `'a'..'z'` map to `0x04..0x1d` (no Shift), `'A'..'Z'` map to `0x84..0x9d` (high bit `0x80` indicates "press with Shift"). See `charToHID()` in `protocol.go` for the full ASCII table.

### Multimedia codes

`1=Vol−, 2=Vol+, 3=Mute, 4=Play/Pause, 5=Prev, 6=Next, 7=Stop, 8=OpenPlayer, 9=BrowserHome, 10=StopWeb, 11=BrowserBack, 12=BrowserForward, 13=Refresh, 14=MyComputer, 15=Mail, 16=Calculator, 17=Search, 18=Shutdown, 19=Sleep`

### Gamepad codes

`1=Left, 2=Right, 3=Up, 4=Down, 5..12 = Button1..Button8`

## Contributing

Issues and PRs welcome. Useful directions:

- More device IDs: if you have a device that reports as one of the `*` strings in the table above and the slot count is wrong, please open an issue with your `info` output.
- Multi-non-modifier-key macros (Combin 3 with state `0x06`)
- A small declarative config format (e.g. TOML) so a whole pedal layout can be applied with one command

## License

Apache-2.0. See [LICENSE](./LICENSE) and [NOTICE](./NOTICE).

## Final disclaimer

To repeat: this is an **unofficial, independently developed third-party project**, not affiliated with or endorsed by Route-R Co., Ltd. The author of this project is not a customer support channel for Route-R hardware; please direct hardware issues to the vendor and software issues to this repository's issue tracker. The protocol description above is provided as documentation of observed device behavior and is not derived from any internal Route-R material.
