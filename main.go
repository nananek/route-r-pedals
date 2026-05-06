// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 The route-r-pedals Authors.

package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const usage = `route-r-pedals — Linux configurator for Route-R USB foot pedals (RI-FP* family)

Usage:
  route-r-pedals [info]                            Show device version + current bindings

  route-r-pedals set <pedal> <combo>               Single key (with optional modifier)
                                                   e.g. 'a', 'f13', 'win+z', 'ctrl+shift+t'
  route-r-pedals set <pedal> mouse [flags]         Mouse button / movement / wheel
                  --btn N      bitmask (1=L, 2=R, 4=M)   default 0
                  --dx N       -128..127  default 0
                  --dy N       -128..127  default 0
                  --wheel N    -128..127  default 0
  route-r-pedals set <pedal> string "text"         Type a string when pedal is pressed
                  --enter      append Enter after the text
                  --longpress  state=0x06 (longpress) instead of 0x04
  route-r-pedals set <pedal> media <name>          Multimedia key (vol+/vol-/mute/play/...)
  route-r-pedals set <pedal> gamepad <name>        Gamepad direction or button (left/up/button1...)

  Common flags for 'set':
                  --short      use short-press (key/clear only)
                  --no-flash   skip the commit-to-NVRAM step
                  --dry-run    print packets only, no I/O

  route-r-pedals clear <pedal> [--no-flash] [--dry-run]
  route-r-pedals flash                             Commit pending writes to NVRAM
  route-r-pedals media-list                        List supported multimedia names
  route-r-pedals help

Privileged: opens /dev/hidraw*, run with sudo.
`

func main() {
	args := os.Args[1:]
	cmd := "info"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "info", "":
		os.Exit(runInfo())
	case "set":
		os.Exit(runSet(args[1:]))
	case "clear":
		os.Exit(runClear(args[1:]))
	case "flash":
		os.Exit(runFlash())
	case "media-list":
		for _, s := range mediaListSorted() {
			fmt.Println(s)
		}
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

func openConfig() (*Device, error) {
	cands, err := FindCandidates(PedalVID, PedalPID)
	if err != nil {
		return nil, err
	}
	if len(cands) == 0 {
		return nil, fmt.Errorf("device 413d:2107 not found in /dev/hidraw* (is it plugged in?)")
	}
	target := cands[0]
	for _, c := range cands {
		if c.IfNum > target.IfNum {
			target = c
		}
	}
	return Open(target.Path)
}

// ---------- info ----------

func runInfo() int {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "warning: not root — /dev/hidraw* usually requires sudo")
	}
	cands, err := FindCandidates(PedalVID, PedalPID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if len(cands) == 0 {
		fmt.Fprintln(os.Stderr, "no device 413d:2107 found in /dev/hidraw*")
		return 1
	}
	fmt.Println("Hidraw candidates for 413d:2107:")
	target := cands[0]
	for _, c := range cands {
		fmt.Printf("  %s  if=%d\n", c.Path, c.IfNum)
		if c.IfNum > target.IfNum {
			target = c
		}
	}
	fmt.Printf("Using: %s (interface %d)\n\n", target.Path, target.IfNum)

	dev, err := Open(target.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v (need root?)\n", target.Path, err)
		return 1
	}
	defer dev.Close()
	dev.Drain()

	fmt.Println("== ReadVersion (0x83) ==")
	if err := dev.Write(cmdReadVersion()); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		return 1
	}
	time.Sleep(500 * time.Millisecond)
	var versionBytes []byte
	for i := 0; i < 3; i++ {
		b, err := dev.ReadWithTimeout(300 * time.Millisecond)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read:", err)
			break
		}
		if b == nil {
			break
		}
		fmt.Printf("  [%d] %s  '%s'\n", i, hex.EncodeToString(b), asciiOnly(b))
		versionBytes = append(versionBytes, b...)
		time.Sleep(100 * time.Millisecond)
	}
	version := strings.Trim(asciiOnly(versionBytes), "\x00 ")
	pedalCount := guessPedalCount(version)
	fmt.Printf("Decoded version: %q\n", version)
	fmt.Printf("Pedal slots    : %d (firmware-reported; physical pedal count may be smaller)\n\n", pedalCount)

	fmt.Println("== ReadPedal (0x82) ==")
	dev.Drain()
	for n := 1; n <= pedalCount; n++ {
		if err := dev.Write(cmdReadPedal(byte(n))); err != nil {
			fmt.Printf("  pedal %d: write err: %v\n", n, err)
			continue
		}
		time.Sleep(200 * time.Millisecond)
		b, err := dev.ReadWithTimeout(500 * time.Millisecond)
		switch {
		case err != nil:
			fmt.Printf("  pedal %d: read err: %v\n", n, err)
		case b == nil:
			fmt.Printf("  pedal %d: (no response)\n", n)
		default:
			fmt.Printf("  pedal %d: %s   %s\n", n, hex.EncodeToString(b), decodePedal(b))
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0
}

// ---------- set ----------

type commonFlags struct {
	dryRun  bool
	noFlash bool
	short   bool
}

// extractFlags pulls out --short / --no-flash / --dry-run, returns the rest.
func extractFlags(args []string) (commonFlags, []string, error) {
	var f commonFlags
	var rest []string
	for _, a := range args {
		switch a {
		case "--short":
			f.short = true
		case "--long":
			f.short = false
		case "--dry-run":
			f.dryRun = true
		case "--no-flash":
			f.noFlash = true
		default:
			rest = append(rest, a)
		}
	}
	return f, rest, nil
}

func runSet(args []string) int {
	if len(args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	pedal, err := strconv.Atoi(args[0])
	if err != nil || pedal < 1 || pedal > 14 {
		fmt.Fprintf(os.Stderr, "invalid pedal %q (expected 1..14)\n", args[0])
		return 2
	}
	subtype := strings.ToLower(args[1])
	switch subtype {
	case "mouse":
		return runSetMouse(pedal, args[2:])
	case "string":
		return runSetString(pedal, args[2:])
	case "media":
		return runSetMedia(pedal, args[2:])
	case "gamepad", "game":
		return runSetGamepad(pedal, args[2:])
	default:
		// treat args[1] as combo (e.g. "win+z", "ctrl+shift+t", "f13")
		return runSetCombo(pedal, args[1:])
	}
}

func runSetCombo(pedal int, args []string) int {
	flags, rest, _ := extractFlags(args)
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "expected exactly one combo arg (e.g. 'win+z')")
		return 2
	}
	mod, key, err := parseCombo(rest[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		return 2
	}
	pkts := buildSetSingleKey(byte(pedal), mod, key, flags.short)
	desc := keyName(key)
	if mod != 0 {
		desc = modifierString(mod) + "+" + desc
	}
	return runWritePlan(fmt.Sprintf("pedal %d → %s (%s)", pedal, desc, pressLabel(flags.short)),
		pkts, flags)
}

func runSetMouse(pedal int, args []string) int {
	flags, rest, _ := extractFlags(args)
	var btn, dx, dy, wheel int
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		var k, v string
		if eq := strings.IndexByte(a, '='); eq >= 0 {
			k, v = a[:eq], a[eq+1:]
		} else if i+1 < len(rest) {
			k, v = a, rest[i+1]
			i++
		} else {
			fmt.Fprintf(os.Stderr, "mouse flag %q missing value\n", a)
			return 2
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mouse flag %s: invalid integer %q\n", k, v)
			return 2
		}
		switch k {
		case "--btn":
			btn = n
		case "--dx":
			dx = n
		case "--dy":
			dy = n
		case "--wheel":
			wheel = n
		default:
			fmt.Fprintf(os.Stderr, "unknown mouse flag %q\n", k)
			return 2
		}
	}
	if btn < 0 || btn > 7 {
		fmt.Fprintln(os.Stderr, "--btn must be 0..7 (bitmask: 1=L 2=R 4=M)")
		return 2
	}
	for _, p := range []struct {
		name string
		val  int
	}{{"--dx", dx}, {"--dy", dy}, {"--wheel", wheel}} {
		if p.val < -128 || p.val > 127 {
			fmt.Fprintf(os.Stderr, "%s out of range -128..127\n", p.name)
			return 2
		}
	}
	pkts := buildMouse(byte(pedal), byte(btn), int8(dx), int8(dy), int8(wheel))
	return runWritePlan(fmt.Sprintf("pedal %d → mouse btn=0x%02x dx=%d dy=%d wheel=%d", pedal, btn, dx, dy, wheel), pkts, flags)
}

func runSetString(pedal int, args []string) int {
	flags, rest, _ := extractFlags(args)
	withEnter := false
	longPress := false
	var text *string
	for _, a := range rest {
		switch a {
		case "--enter":
			withEnter = true
		case "--longpress":
			longPress = true
		default:
			if strings.HasPrefix(a, "--") {
				fmt.Fprintf(os.Stderr, "unknown string flag %q\n", a)
				return 2
			}
			if text != nil {
				fmt.Fprintln(os.Stderr, "string takes a single quoted text argument")
				return 2
			}
			t := a
			text = &t
		}
	}
	if text == nil {
		fmt.Fprintln(os.Stderr, "string requires a text argument, e.g.  set 1 string \"hello world\"")
		return 2
	}
	chars, err := stringToHIDBytes(*text)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if !longPress && len(chars)+1 > 64 {
		fmt.Fprintln(os.Stderr, "string too long (firmware limit; try splitting)")
		return 2
	}
	pkts := buildString(byte(pedal), chars, withEnter, longPress)
	mode := "shortpress"
	if longPress {
		mode = "longpress"
	}
	enterTag := ""
	if withEnter {
		enterTag = " +Enter"
	}
	return runWritePlan(fmt.Sprintf("pedal %d → string %q%s (%s)", pedal, *text, enterTag, mode), pkts, flags)
}

func runSetMedia(pedal int, args []string) int {
	flags, rest, _ := extractFlags(args)
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "media takes one name (try 'route-r-pedals media-list')")
		return 2
	}
	code, ok := mediaCodes[strings.ToLower(rest[0])]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown media name %q (try 'route-r-pedals media-list')\n", rest[0])
		return 2
	}
	pkts := buildMedia(byte(pedal), code)
	return runWritePlan(fmt.Sprintf("pedal %d → media %s (code %d)", pedal, mediaName(code), code), pkts, flags)
}

func runSetGamepad(pedal int, args []string) int {
	flags, rest, _ := extractFlags(args)
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "gamepad takes one name (left/right/up/down/button1..button8)")
		return 2
	}
	code, ok := gamepadCodes[strings.ToLower(rest[0])]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown gamepad name %q\n", rest[0])
		return 2
	}
	pkts := buildGamepad(byte(pedal), code)
	return runWritePlan(fmt.Sprintf("pedal %d → gamepad %s (code %d)", pedal, gamepadName(code), code), pkts, flags)
}

// ---------- clear / flash ----------

func runClear(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "clear requires a pedal number")
		return 2
	}
	pedal, err := strconv.Atoi(args[0])
	if err != nil || pedal < 1 || pedal > 14 {
		fmt.Fprintf(os.Stderr, "invalid pedal %q\n", args[0])
		return 2
	}
	flags, rest, _ := extractFlags(args[1:])
	if len(rest) != 0 {
		fmt.Fprintln(os.Stderr, "clear takes no positional arguments after the pedal number")
		return 2
	}
	pkts := buildClear(byte(pedal))
	return runWritePlan(fmt.Sprintf("pedal %d → clear", pedal), pkts, flags)
}

func runFlash() int {
	dev, err := openConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer dev.Close()
	dev.Drain()
	if err := dev.Write(cmdFlash()); err != nil {
		fmt.Fprintln(os.Stderr, "flash:", err)
		return 1
	}
	time.Sleep(200 * time.Millisecond)
	fmt.Println("flashed.")
	return 0
}

// ---------- shared write executor ----------

func runWritePlan(desc string, pkts [][]byte, flags commonFlags) int {
	fmt.Printf("Plan: %s\n", desc)
	for i, p := range pkts {
		fmt.Printf("  packet[%d] %s\n", i, hex.EncodeToString(p))
	}
	if !flags.noFlash {
		fmt.Printf("  flash      %s\n", hex.EncodeToString(cmdFlash()))
	}
	if flags.dryRun {
		fmt.Println("(dry-run, no I/O performed)")
		return 0
	}
	dev, err := openConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer dev.Close()
	dev.Drain()
	for i, p := range pkts {
		if err := dev.Write(p); err != nil {
			fmt.Fprintf(os.Stderr, "write packet %d: %v\n", i, err)
			return 1
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !flags.noFlash {
		if err := dev.Write(cmdFlash()); err != nil {
			fmt.Fprintf(os.Stderr, "flash: %v\n", err)
			return 1
		}
		time.Sleep(200 * time.Millisecond)
		fmt.Println("written + flashed.")
	} else {
		fmt.Println("written (not flashed — run 'flash' to commit).")
	}
	return 0
}

// ---------- helpers ----------

func asciiOnly(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c >= 32 && c < 127 {
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

func guessPedalCount(v string) int {
	switch {
	case strings.Contains(v, "FootSwitch2"):
		return 2
	case strings.Contains(v, "HK4-"), strings.Contains(v, "USB5Key"):
		return 4
	case strings.Contains(v, "HK6-"), strings.Contains(v, "USW6V"), strings.Contains(v, "HandKey"):
		return 6
	case strings.Contains(v, "DIY-"):
		return 14
	default:
		return 3
	}
}
