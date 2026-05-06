// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 The route-r-pedals Authors.

package main

import (
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
)

// hexPkts converts a list of 9-byte buffers to lowercase hex strings.
func hexPkts(pkts [][]byte) []string {
	out := make([]string, len(pkts))
	for i, p := range pkts {
		out[i] = hex.EncodeToString(p)
	}
	return out
}

func TestBuildSetSingleKey(t *testing.T) {
	cases := []struct {
		name        string
		pedal       byte
		mod, key    byte
		short       bool
		wantPackets []string
	}{
		{
			// Verified against live device: pedal 2 reads back as 0881081d00000000.
			name:  "Win+z shortpress (pedal 2)",
			pedal: 2, mod: ModWin, key: 0x1d, short: true,
			wantPackets: []string{
				"000181080200000000",
				"000881081d00000000",
			},
		},
		{
			name:  "Ctrl+z shortpress (pedal 2)",
			pedal: 2, mod: ModCtrl, key: 0x1d, short: true,
			wantPackets: []string{
				"000181080200000000",
				"000881011d00000000",
			},
		},
		{
			// Factory default observed for slot 1: 0801000400000000.
			name:  "key 'a' longpress (pedal 1)",
			pedal: 1, mod: 0, key: 0x04, short: false,
			wantPackets: []string{
				"000181080100000000",
				"000801000400000000",
			},
		},
		{
			name:  "F13 shortpress (pedal 3)",
			pedal: 3, mod: 0, key: 0x68, short: true,
			wantPackets: []string{
				"000181080300000000",
				"000881006800000000",
			},
		},
		{
			name:  "Ctrl+Shift+t longpress (pedal 1)",
			pedal: 1, mod: ModCtrl | ModShft, key: 0x17, short: false,
			wantPackets: []string{
				"000181080100000000",
				"000801031700000000",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hexPkts(buildSetSingleKey(tc.pedal, tc.mod, tc.key, tc.short))
			if !reflect.DeepEqual(got, tc.wantPackets) {
				t.Errorf("got  %v\nwant %v", got, tc.wantPackets)
			}
		})
	}
}

func TestBuildClear(t *testing.T) {
	got := hexPkts(buildClear(2))
	want := []string{
		"000181080200000000",
		"000801000000000000",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBuildMouse(t *testing.T) {
	cases := []struct {
		name          string
		pedal, btn    byte
		dx, dy, wheel int8
		wantPackets   []string
	}{
		{
			name:  "left button only",
			pedal: 1, btn: 1, dx: 0, dy: 0, wheel: 0,
			wantPackets: []string{
				"000181080100000000",
				"000802000001000000",
			},
		},
		{
			// dx=10, dy=-5, wheel=0 → byte 0x0a, 0xfb, 0x00 (signed)
			name:  "left + delta",
			pedal: 1, btn: 1, dx: 10, dy: -5, wheel: 0,
			wantPackets: []string{
				"000181080100000000",
				"0008020000010afb00",
			},
		},
		{
			name:  "wheel scroll up",
			pedal: 2, btn: 0, dx: 0, dy: 0, wheel: 1,
			wantPackets: []string{
				"000181080200000000",
				"000802000000000001",
			},
		},
		{
			name:  "wheel scroll down (signed)",
			pedal: 2, btn: 0, dx: 0, dy: 0, wheel: -1,
			wantPackets: []string{
				"000181080200000000",
				"0008020000000000ff",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hexPkts(buildMouse(tc.pedal, tc.btn, tc.dx, tc.dy, tc.wheel))
			if !reflect.DeepEqual(got, tc.wantPackets) {
				t.Errorf("got  %v\nwant %v", got, tc.wantPackets)
			}
		})
	}
}

func TestBuildMedia(t *testing.T) {
	cases := []struct {
		name string
		ped  byte
		code byte
		want []string
	}{
		{"vol-", 1, 1, []string{"000181080100000000", "000807010000000000"}},
		{"vol+", 1, 2, []string{"000181080100000000", "000807020000000000"}},
		{"play/pause", 1, 4, []string{"000181080100000000", "000807040000000000"}},
		{"sleep", 3, 19, []string{"000181080300000000", "000807130000000000"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hexPkts(buildMedia(tc.ped, tc.code))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBuildGamepad(t *testing.T) {
	cases := []struct {
		name string
		ped  byte
		code byte
		want []string
	}{
		{"left", 1, 1, []string{"000181080100000000", "000808010000000000"}},
		{"button1", 2, 5, []string{"000181080200000000", "000808050000000000"}},
		{"button8", 3, 12, []string{"000181080300000000", "0008080c0000000000"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hexPkts(buildGamepad(tc.ped, tc.code))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBuildString(t *testing.T) {
	type tc struct {
		name      string
		text      string
		withEnter bool
		longPress bool
		want      []string
	}
	cases := []tc{
		{
			// "hi": chars=2, +0 enter, num5=4, num6=2 → single packet
			// data[1]=num5=4, data[2]=state=4, chars at [3..4]
			name: "two-char short",
			text: "hi",
			want: []string{
				"000181040100000000",
				"0004040b0c00000000",
			},
		},
		{
			// "hello": chars=5, num5=7, num6=5 → single packet
			name: "5-char no enter",
			text: "hello",
			want: []string{
				"000181070100000000",
				"0007040b080f0f1200",
			},
		},
		{
			// "hello"+enter: num5=8, num6=6 → triggers num6>=6 branch
			// First packet has 5 chars at [3..7] + Enter at [8]
			name:      "5-char with enter",
			text:      "hello",
			withEnter: true,
			want: []string{
				"000181080100000000",
				"0008040b080f0f1228",
			},
		},
		{
			// Longpress flips state byte 0x04 → 0x06.
			name:      "longpress",
			text:      "hi",
			longPress: true,
			want: []string{
				"000181040100000000",
				"0004060b0c00000000",
			},
		},
		{
			// 14 chars → num6=14, num5=0x10. First packet 6 chars, continuation 8 chars.
			name: "14-char no enter",
			text: "abcdefghijklmn",
			want: []string{
				"000181100100000000",
				"001004040506070809",
				"000a0b0c0d0e0f1011",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chars, err := stringToHIDBytes(c.text)
			if err != nil {
				t.Fatalf("encode %q: %v", c.text, err)
			}
			got := hexPkts(buildString(1, chars, c.withEnter, c.longPress))
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %v\nwant %v", got, c.want)
			}
		})
	}
}

func TestCharToHID(t *testing.T) {
	cases := map[byte]byte{
		'a': 0x04, 'z': 0x1d,
		'A': 0x84, 'Z': 0x9d, // 0x80 high bit = shifted
		'1': 0x1e, '9': 0x26, '0': 0x27,
		' ':  0x2c,
		'-':  0x2d,
		'=':  0x2e,
		'[':  0x2f,
		']':  0x30,
		'\\': 0x31,
		';':  0x33,
		'\'': 0x34,
		'`':  0x35,
		',':  0x36,
		'.':  0x37,
		'/':  0x38,
	}
	for in, want := range cases {
		got := charToHID(in)
		if got != want {
			t.Errorf("charToHID(%q)=0x%02x, want 0x%02x", in, got, want)
		}
	}
}

func TestStringToHIDBytesError(t *testing.T) {
	_, err := stringToHIDBytes("hello\n")
	if err == nil {
		t.Errorf("expected error for newline character, got nil")
	}
	_, err = stringToHIDBytes("hello")
	if err != nil {
		t.Errorf("unexpected error for valid string: %v", err)
	}
}

func TestParseCombo(t *testing.T) {
	cases := []struct {
		in      string
		wantMod byte
		wantKey byte
		wantErr bool
	}{
		{"a", 0, 0x04, false},
		{"win+z", ModWin, 0x1d, false},
		{"ctrl+shift+t", ModCtrl | ModShft, 0x17, false},
		{"f13", 0, 0x68, false},
		{"Win+Z", ModWin, 0x1d, false}, // case-insensitive
		{"cmd+a", ModWin, 0x04, false}, // cmd alias
		{"super+space", ModWin, 0x2c, false},
		{"ctrl", 0, 0, true},   // no key
		{"a+b", 0, 0, true},    // two non-mods
		{"foobar", 0, 0, true}, // unknown
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			mod, key, err := parseCombo(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if mod != tc.wantMod || key != tc.wantKey {
				t.Errorf("got mod=0x%02x key=0x%02x, want mod=0x%02x key=0x%02x",
					mod, key, tc.wantMod, tc.wantKey)
			}
		})
	}
}

func TestDecodePedal(t *testing.T) {
	cases := []struct {
		hexIn      string
		wantSubstr string
	}{
		// Live device responses verified against ReadPedal output.
		{"0801000400000000", "key a (longpress)"},
		{"0881081d00000000", "key Win+z (shortpress)"},
		{"0801000600000000", "key c (longpress)"},

		// Wire form (8 bytes; the leading report-ID 0x00 is stripped by Linux hidraw).
		{"0807040000000000", "media Play/Pause"},
		{"08080c0000000000", "gamepad Button8"},
		{"08020000010afb00", "mouse btn=0x01"},
		{"0801000000000000", "unset"},
	}
	for _, tc := range cases {
		t.Run(tc.hexIn, func(t *testing.T) {
			b, err := hex.DecodeString(tc.hexIn)
			if err != nil {
				t.Fatalf("bad hex: %v", err)
			}
			got := decodePedal(b)
			if !strings.Contains(got, tc.wantSubstr) {
				t.Errorf("got %q, want substring %q", got, tc.wantSubstr)
			}
		})
	}
}

func TestMediaCodeNames(t *testing.T) {
	for name, code := range mediaCodes {
		if code < 1 || code > 19 {
			t.Errorf("media name %q maps to out-of-range code %d", name, code)
		}
	}
	if mediaCodes["voldown"] != 1 || mediaCodes["volup"] != 2 {
		t.Errorf("vol- / vol+ aliases wrong")
	}
}

func TestGamepadCodeNames(t *testing.T) {
	for name, code := range gamepadCodes {
		if code < 1 || code > 12 {
			t.Errorf("gamepad name %q maps to out-of-range code %d", name, code)
		}
	}
}
