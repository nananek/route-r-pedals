// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 The route-r-pedals Authors.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type HidrawCandidate struct {
	Path  string
	IfNum int
}

func FindCandidates(vid, pid int) ([]HidrawCandidate, error) {
	matches, err := filepath.Glob("/dev/hidraw*")
	if err != nil {
		return nil, err
	}
	var out []HidrawCandidate
	for _, dev := range matches {
		name := filepath.Base(dev)
		sysDev := "/sys/class/hidraw/" + name + "/device"
		target, err := filepath.EvalSymlinks(sysDev)
		if err != nil {
			continue
		}
		var foundVID, foundPID, ifNum = 0, 0, -1
		for d := target; d != "/" && d != "."; d = filepath.Dir(d) {
			if buf, err := os.ReadFile(filepath.Join(d, "bInterfaceNumber")); err == nil && ifNum < 0 {
				fmt.Sscanf(strings.TrimSpace(string(buf)), "%x", &ifNum)
			}
			if buf, err := os.ReadFile(filepath.Join(d, "idVendor")); err == nil && foundVID == 0 {
				fmt.Sscanf(strings.TrimSpace(string(buf)), "%x", &foundVID)
			}
			if buf, err := os.ReadFile(filepath.Join(d, "idProduct")); err == nil && foundPID == 0 {
				fmt.Sscanf(strings.TrimSpace(string(buf)), "%x", &foundPID)
			}
			if foundVID != 0 && foundPID != 0 && ifNum >= 0 {
				break
			}
		}
		if foundVID == vid && foundPID == pid {
			out = append(out, HidrawCandidate{Path: dev, IfNum: ifNum})
		}
	}
	return out, nil
}

type Device struct {
	f *os.File
}

func Open(path string) (*Device, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &Device{f: f}, nil
}

func (d *Device) Close() error { return d.f.Close() }

func (d *Device) Write(buf []byte) error {
	_, err := d.f.Write(buf)
	return err
}

func (d *Device) Drain() {
	fd := int(d.f.Fd())
	for {
		pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, _ := unix.Poll(pfd, 0)
		if n == 0 {
			return
		}
		buf := make([]byte, 64)
		_, _ = unix.Read(fd, buf)
	}
}

func (d *Device) ReadWithTimeout(timeout time.Duration) ([]byte, error) {
	fd := int(d.f.Fd())
	pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	ms := int(timeout / time.Millisecond)
	n, err := unix.Poll(pfd, ms)
	if err != nil {
		return nil, err
	}
	if n == 0 || pfd[0].Revents&unix.POLLIN == 0 {
		return nil, nil
	}
	buf := make([]byte, 64)
	rn, err := unix.Read(fd, buf)
	if err != nil {
		return nil, err
	}
	return buf[:rn], nil
}
