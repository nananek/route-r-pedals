// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 The route-r-pedals Authors.

package main

import (
	"fmt"
	"sort"
	"strings"
)

const (
	PedalVID = 0x413d
	PedalPID = 0x2107

	ReportID byte = 0x00
	Marker   byte = 0x01

	OpFlash     byte = 0x80
	OpWrite     byte = 0x81
	OpRead      byte = 0x82
	OpReadVer   byte = 0x83
	OpReadModel byte = 0x85
	OpReadPress byte = 0x86

	PacketSize = 9

	ModCtrl byte = 0x01
	ModShft byte = 0x02
	ModAlt  byte = 0x04
	ModWin  byte = 0x08

	StateLong  byte = 0x01
	StateShort byte = 0x81

	CombinClear byte = 0x00
	CombinKey   byte = 0x01
	CombinMouse byte = 0x02
	CombinMacro byte = 0x06
	CombinStr4  byte = 0x04
	CombinStr6  byte = 0x06
	CombinMedia byte = 0x07
	CombinGame  byte = 0x08
)

// ---------- Read commands ----------

func cmdReadVersion() []byte {
	return []byte{ReportID, Marker, OpReadVer, 0x08, 0, 0, 0, 0, 0}
}

func cmdReadPedal(n byte) []byte {
	return []byte{ReportID, Marker, OpRead, 0x08, n, 0, 0, 0, 0}
}

func cmdReadModel() []byte {
	return []byte{ReportID, Marker, OpReadModel, 0x00, 0, 0, 0, 0, 0}
}

func cmdReadPressModel() []byte {
	return []byte{ReportID, Marker, OpReadPress, 0x00, 0, 0, 0, 0, 0}
}

func cmdFlash() []byte {
	return []byte{ReportID, Marker, OpFlash, 0x08, 0x01, 0, 0, 0, 0}
}

// ---------- Write packet builders ----------

func writeHeader(pedal, length byte) []byte {
	return []byte{ReportID, Marker, OpWrite, length, pedal, 0, 0, 0, 0}
}

// buildSetSingleKey: Combin 1 / demoted Combin 3 (single key + optional modifier).
func buildSetSingleKey(pedal, mod, key byte, shortPress bool) [][]byte {
	state := StateLong
	if shortPress {
		state = StateShort
	}
	return [][]byte{
		writeHeader(pedal, 0x08),
		{ReportID, 0x08, state, mod, key, 0, 0, 0, 0},
	}
}

// buildClear: Combin 0. Same wire packet as a single key with mod=0, key=0, longpress.
func buildClear(pedal byte) [][]byte {
	return [][]byte{
		writeHeader(pedal, 0x08),
		{ReportID, 0x08, StateLong, 0, 0, 0, 0, 0, 0},
	}
}

// buildMouse: Combin 2. button is a bitmask (1=left, 2=right, 4=middle).
// dx/dy/wheel are int8 (-128..127).
func buildMouse(pedal, button byte, dx, dy, wheel int8) [][]byte {
	return [][]byte{
		writeHeader(pedal, 0x08),
		{ReportID, 0x08, CombinMouse, 0, 0, button, byte(dx), byte(dy), byte(wheel)},
	}
}

// buildMedia: Combin 7. code is a multimedia function index (1..19).
func buildMedia(pedal, code byte) [][]byte {
	return [][]byte{
		writeHeader(pedal, 0x08),
		{ReportID, 0x08, CombinMedia, code, 0, 0, 0, 0, 0},
	}
}

// buildGamepad: Combin 8. code is a gamepad function index (1..12).
func buildGamepad(pedal, code byte) [][]byte {
	return [][]byte{
		writeHeader(pedal, 0x08),
		{ReportID, 0x08, CombinGame, code, 0, 0, 0, 0, 0},
	}
}

// buildString: Combin 4 (short) / 6 (long).  Encodes a sequence of HID-coded
// chars (use charToHID) into one or more 9-byte packets.  If withEnter is set,
// HID code 0x28 (Enter) is appended after the last char.
func buildString(pedal byte, chars []byte, withEnter, longPress bool) [][]byte {
	state := CombinStr4
	if longPress {
		state = CombinStr6
	}
	extra := 0
	if withEnter {
		extra = 1
	}
	num6 := len(chars) + extra
	num5 := byte(num6 + 2)

	pkts := [][]byte{writeHeader(pedal, num5)}

	getCharOrEnter := func(i int) byte {
		if withEnter && i == num6-1 {
			return 0x28
		}
		return chars[i]
	}

	if num6 < 6 {
		d := []byte{ReportID, num5, state, 0, 0, 0, 0, 0, 0}
		for l := 0; l < num6; l++ {
			d[3+l] = getCharOrEnter(l)
		}
		pkts = append(pkts, d)
		return pkts
	}

	// num6 >= 6 — first packet carries 6 chars at [3..8]
	first := []byte{ReportID, num5, state, 0, 0, 0, 0, 0, 0}
	for m := 0; m < 6; m++ {
		first[3+m] = getCharOrEnter(m)
	}
	pkts = append(pkts, first)

	// Continuation packets: 8 chars each at [1..8]
	idx := 6
	for idx < num6 {
		cont := []byte{ReportID, 0, 0, 0, 0, 0, 0, 0, 0}
		for j := 0; j < 8 && idx < num6; j++ {
			cont[1+j] = getCharOrEnter(idx)
			idx++
		}
		pkts = append(pkts, cont)
	}
	return pkts
}

// ---------- Char to HID (for string typing) ----------

// charToHID converts an ASCII char to the firmware's char encoding.
// High bit 0x80 means "Shift modifier required".  Returns 0 for unsupported chars.
// Mirrors USBHandleKeyBoard_14.ADL.KeyCodeConvertUSBCode.RetrunByte.
func charToHID(c byte) byte {
	switch {
	case c >= 'a' && c <= 'z':
		return c - 93
	case c >= 'A' && c <= 'Z':
		return (c - 61) | 0x80
	case c >= '1' && c <= '9':
		return c - 19
	}
	switch c {
	case '0':
		return c - 9
	case '!':
		return (c - 3) | 0x80
	case '#', '$', '%':
		return (c - 3) | 0x80
	case '@':
		return (c - 33) | 0x80
	case '^':
		return (c - 59) | 0x80
	case '&', '(', ')':
		return (c - 2) | 0x80
	case '*':
		return (c - 5) | 0x80
	case '_':
		return (c - 50) | 0x80
	case '-':
		return c
	case '+':
		return (c + 3) | 0x80
	case '=':
		return c - 15
	case '{':
		return (c - 76) | 0x80
	case '[':
		return c - 44
	case '}':
		return (c - 77) | 0x80
	case ']':
		return c - 45
	case '|':
		return (c - 75) | 0x80
	case '\\':
		return c - 43
	case ' ':
		return 44
	case ':':
		return (c - 7) | 0x80
	case ';':
		return 51
	case '"':
		return (c + 18) | 0x80
	case '\'':
		return 52
	case '~':
		return (c - 73) | 0x80
	case '<':
		return (c - 6) | 0x80
	case ',':
		return 54
	case '>', '?':
		return (c - 7) | 0x80
	case '.':
		return 55
	case '`':
		return 53
	case '/':
		return 56
	}
	return 0
}

// stringToHIDBytes encodes a UTF-8 string for Combin=4 string typing.
// Returns an error on any unsupported char.
func stringToHIDBytes(s string) ([]byte, error) {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		b := charToHID(s[i])
		if b == 0 && s[i] != 0 {
			return nil, fmt.Errorf("unsupported character %q at position %d (string-typing only handles ASCII printable)", s[i], i)
		}
		out = append(out, b)
	}
	return out, nil
}

// hidByteToChar reverses charToHID for display (best-effort, ASCII only).
func hidByteToChar(b byte) byte {
	switch {
	case b >= 4 && b <= 29:
		return b + 93 // a..z
	case b >= 132 && b <= 157:
		return (b - 128) + 61 // A..Z
	case b >= 30 && b <= 38:
		return b + 19 // 1..9
	case b == 39:
		return '0'
	case b == 44:
		return ' '
	case b == 51:
		return ';'
	case b == 52:
		return '\''
	case b == 53:
		return '`'
	case b == 54:
		return ','
	case b == 55:
		return '.'
	case b == 56:
		return '/'
	}
	return '?'
}

// ---------- Multimedia code table ----------

var mediaCodes = map[string]byte{
	"vol-": 1, "voldown": 1, "volume-down": 1,
	"vol+": 2, "volup": 2, "volume-up": 2,
	"mute": 3,
	"play": 4, "pause": 4, "play/pause": 4, "playpause": 4,
	"prev": 5, "previous": 5, "back-track": 5,
	"next": 6, "next-track": 6,
	"stop":   7,
	"player": 8, "open-player": 8,
	"home": 9, "browser-home": 9,
	"stopweb": 10, "stop-web": 10,
	"back": 11, "browser-back": 11,
	"forward": 12, "browser-forward": 12,
	"refresh":  13,
	"computer": 14, "my-computer": 14,
	"mail":       15,
	"calculator": 16, "calc": 16,
	"search":   17,
	"shutdown": 18,
	"sleep":    19,
}

var mediaNames = map[byte]string{
	1: "Vol-", 2: "Vol+", 3: "Mute", 4: "Play/Pause",
	5: "Prev", 6: "Next", 7: "Stop",
	8: "OpenPlayer", 9: "BrowserHome", 10: "StopWeb",
	11: "BrowserBack", 12: "BrowserForward", 13: "Refresh",
	14: "MyComputer", 15: "Mail", 16: "Calculator", 17: "Search",
	18: "Shutdown", 19: "Sleep",
}

func mediaName(code byte) string {
	if n, ok := mediaNames[code]; ok {
		return n
	}
	return fmt.Sprintf("media:0x%02x", code)
}

func mediaListSorted() []string {
	codes := make([]byte, 0, len(mediaNames))
	for c := range mediaNames {
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		out = append(out, fmt.Sprintf("%s (%d)", mediaNames[c], c))
	}
	return out
}

// ---------- Gamepad code table ----------

var gamepadCodes = map[string]byte{
	"left": 1, "right": 2, "up": 3, "down": 4,
	"button1": 5, "button2": 6, "button3": 7, "button4": 8,
	"button5": 9, "button6": 10, "button7": 11, "button8": 12,
	"b1": 5, "b2": 6, "b3": 7, "b4": 8, "b5": 9, "b6": 10, "b7": 11, "b8": 12,
}

var gamepadNames = map[byte]string{
	1: "Left", 2: "Right", 3: "Up", 4: "Down",
	5: "Button1", 6: "Button2", 7: "Button3", 8: "Button4",
	9: "Button5", 10: "Button6", 11: "Button7", 12: "Button8",
}

func gamepadName(code byte) string {
	if n, ok := gamepadNames[code]; ok {
		return n
	}
	return fmt.Sprintf("gamepad:0x%02x", code)
}

// ---------- Combo / key parser ----------

// hidKeyName maps a USB HID Keyboard Usage ID (Page 0x07) to a human label.
var hidKeyName = func() map[byte]string {
	m := map[byte]string{
		0x00: "(none)",
		0x28: "Enter", 0x29: "Esc", 0x2a: "Backspace", 0x2b: "Tab",
		0x2c: "Space",
		0x2d: "-", 0x2e: "=", 0x2f: "[", 0x30: "]", 0x31: "\\",
		0x33: ";", 0x34: "'", 0x35: "`",
		0x36: ",", 0x37: ".", 0x38: "/",
		0x39: "CapsLock",
		0x46: "PrintScreen", 0x47: "ScrollLock", 0x48: "Pause",
		0x49: "Insert", 0x4a: "Home", 0x4b: "PageUp",
		0x4c: "Delete", 0x4d: "End", 0x4e: "PageDown",
		0x4f: "Right", 0x50: "Left", 0x51: "Down", 0x52: "Up",
		0x53: "NumLock",
		0x54: "KP/", 0x55: "KP*", 0x56: "KP-", 0x57: "KP+", 0x58: "KPEnter",
		0x59: "KP1", 0x5a: "KP2", 0x5b: "KP3", 0x5c: "KP4", 0x5d: "KP5",
		0x5e: "KP6", 0x5f: "KP7", 0x60: "KP8", 0x61: "KP9", 0x62: "KP0",
		0x63: "KP.", 0x65: "Application",
	}
	for i := byte(0); i < 26; i++ {
		m[0x04+i] = string('a' + rune(i))
	}
	for i := byte(0); i < 9; i++ {
		m[0x1e+i] = string('1' + rune(i))
	}
	m[0x27] = "0"
	for i := byte(0); i < 12; i++ {
		m[0x3a+i] = fmt.Sprintf("F%d", i+1)
	}
	for i := byte(0); i < 12; i++ {
		m[0x68+i] = fmt.Sprintf("F%d", i+13)
	}
	return m
}()

func keyName(code byte) string {
	if n, ok := hidKeyName[code]; ok {
		return n
	}
	return fmt.Sprintf("HID:0x%02x", code)
}

func modifierString(mask byte) string {
	if mask == 0 {
		return ""
	}
	var parts []string
	if mask&ModCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mask&ModShft != 0 {
		parts = append(parts, "Shift")
	}
	if mask&ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if mask&ModWin != 0 {
		parts = append(parts, "Win")
	}
	if extra := mask &^ (ModCtrl | ModShft | ModAlt | ModWin); extra != 0 {
		parts = append(parts, fmt.Sprintf("0x%02x", extra))
	}
	return strings.Join(parts, "+")
}

var modifierByName = map[string]byte{
	"ctrl": ModCtrl, "control": ModCtrl,
	"shift": ModShft,
	"alt":   ModAlt, "option": ModAlt, "opt": ModAlt,
	"win": ModWin, "windows": ModWin, "super": ModWin, "meta": ModWin, "cmd": ModWin,
}

var keyByName = func() map[string]byte {
	m := make(map[string]byte)
	for code, name := range hidKeyName {
		if code == 0 {
			continue
		}
		m[strings.ToLower(name)] = code
	}
	m["return"] = 0x28
	m["esc"] = 0x29
	m["bs"] = 0x2a
	m["bksp"] = 0x2a
	m["pgup"] = 0x4b
	m["pgdn"] = 0x4e
	m["del"] = 0x4c
	m["ins"] = 0x49
	return m
}()

func parseCombo(combo string) (byte, byte, error) {
	parts := strings.Split(combo, "+")
	var mod, key byte
	keySet := false
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if m, ok := modifierByName[p]; ok {
			mod |= m
			continue
		}
		if k, ok := keyByName[p]; ok {
			if keySet {
				return 0, 0, fmt.Errorf("more than one non-modifier key in %q", combo)
			}
			key = k
			keySet = true
			continue
		}
		return 0, 0, fmt.Errorf("unknown key/modifier %q", p)
	}
	if !keySet {
		return 0, 0, fmt.Errorf("no key specified in %q (only modifiers)", combo)
	}
	return mod, key, nil
}

func pressLabel(short bool) string {
	if short {
		return "shortpress"
	}
	return "longpress"
}

// ---------- Decoder for read responses ----------

func decodePedal(b []byte) string {
	if len(b) < 8 {
		return fmt.Sprintf("(short response, %d bytes)", len(b))
	}
	if b[0] != 0x08 {
		// Possibly a longer payload (e.g. string typing >5 chars with custom length field).
		// Length is in b[0]; treat anything not 0x08 as variable-length payload.
		return decodeVariablePedal(b)
	}
	combin := b[1]
	shortPress := combin&0x80 != 0
	bare := combin &^ 0x80
	switch bare {
	case 0x00, 0x01:
		mod := modifierString(b[2])
		key := keyName(b[3])
		if mod == "" && b[3] == 0 {
			return "unset"
		}
		if mod != "" {
			return fmt.Sprintf("key %s+%s (%s)", mod, key, pressLabel(shortPress))
		}
		return fmt.Sprintf("key %s (%s)", key, pressLabel(shortPress))
	case 0x02:
		// 8-byte wire indexing: [4]=btn, [5]=dx, [6]=dy, [7]=wheel.
		return fmt.Sprintf("mouse btn=0x%02x dx=%d dy=%d wheel=%d",
			b[4], int8(b[5]), int8(b[6]), int8(b[7]))
	case 0x04:
		return fmt.Sprintf("string (shortpress) chars=%q", decodeStringChars(b[2:]))
	case 0x06:
		// Either string-longpress or a multi-key macro.  We can't always
		// tell them apart from the on-wire bytes alone.
		return fmt.Sprintf("string-longpress or macro (%s)", decodeStringChars(b[2:]))
	case 0x07:
		return fmt.Sprintf("media %s", mediaName(b[2]))
	case 0x08:
		return fmt.Sprintf("gamepad %s", gamepadName(b[2]))
	default:
		return fmt.Sprintf("(unknown combin 0x%02x)", combin)
	}
}

func decodeVariablePedal(b []byte) string {
	// b[0] is the payload length, b[1] is combin/state, rest is data.
	if len(b) < 3 {
		return fmt.Sprintf("(short var packet, %d bytes)", len(b))
	}
	switch b[1] {
	case 0x04, 0x84:
		return fmt.Sprintf("string (shortpress) chars=%q (multi-packet)", decodeStringChars(b[2:]))
	case 0x06, 0x86:
		return fmt.Sprintf("string-longpress chars=%q (multi-packet)", decodeStringChars(b[2:]))
	}
	return fmt.Sprintf("(variable packet len=0x%02x combin=0x%02x)", b[0], b[1])
}

func decodeStringChars(data []byte) string {
	var sb strings.Builder
	for _, c := range data {
		if c == 0 {
			continue
		}
		sb.WriteByte(hidByteToChar(c))
	}
	return sb.String()
}
