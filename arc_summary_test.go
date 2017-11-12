// Test file for arc_summary.go
// Scot W. Stevenson
// First version: 11. Nov 2017
// This version: 11. Nov 2017
package main

import (
	"math"
	"testing"
)

func TestFormatHits(t *testing.T) {
	var tests = []struct {
		have uint64
		want string
	}{
		{0, "0"},
		{64, "64"},
		{1000, "1.0k"},
		{1001, "1.0k"},
		{2101, "2.1k"},
		{uint64(math.Pow10(3)), "1.0k"},
		{uint64(math.Pow10(3) + 1), "1.0k"},
		{uint64(math.Pow10(6)), "1.0M"},
		{uint64(math.Pow10(6) + 1), "1.0M"},
		{uint64(math.Pow10(9)), "1.0G"},
		{uint64(math.Pow10(9) + 1), "1.0G"},
		{uint64(math.Pow10(12)), "1.0T"},
		{uint64(math.Pow10(12) + 1), "1.0T"},
		{uint64(math.Pow10(15)), "1.0P"},
		{uint64(math.Pow10(15) + 1), "1.0P"},
		{uint64(math.Pow10(18)), "1.0E"},
		{uint64(math.Pow10(18) + 1), "1.0E"},
		{math.MaxUint64, "18.4E"},
	}

	for _, test := range tests {
		got := formatHits(test.have)
		if got != test.want {
			t.Errorf("formatHits(%d) = %v (wanted \"%v\")", test.have, got, test.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	var tests = []struct {
		have uint64
		want string
	}{
		{0, "0 Bytes"},
		{10, "10 Bytes"},
		{1023, "1023 Bytes"},
		{1024, "1.0 KiB"},
		{uint64(math.Pow(2, 10) + 25), "1.0 KiB"},
		{uint64(math.Pow(2, 20)), "1.0 MiB"},
		{uint64(math.Pow(2, 20) + 1), "1.0 MiB"},
		{uint64(math.Pow(2, 30)), "1.0 GiB"},
		{uint64(math.Pow(2, 30) + 1), "1.0 GiB"},
		{uint64(math.Pow(2, 40)), "1.0 TiB"},
		{uint64(math.Pow(2, 40) + 1), "1.0 TiB"},
		{uint64(math.Pow(2, 50)), "1.0 PiB"},
		{uint64(math.Pow(2, 50) + 1), "1.0 PiB"},
		{uint64(math.Pow(2, 60)), "1.0 EiB"},
		{uint64(math.Pow(2, 60) + 1), "1.0 EiB"},
		{math.MaxUint64, "16.0 EiB"},
	}

	for _, test := range tests {
		got := formatBytes(test.have)
		if got != test.want {
			t.Errorf("formatBytes(%d) = %v (wanted \"%v\")", test.have, got, test.want)
		}
	}
}

func TestIsLegalSection(t *testing.T) {
	var tests = []struct {
		have string
		want bool
	}{
		{"arc", true},
		{"dmu", true},
		{"l2arc", true},
		{"tunables", true},
		{"vdev", true},
		{"xuio", true},
		{"zfetch", true},
		{"zil", true},

		{"ZFS", false},
		{"So say we all", false},
	}

	for _, test := range tests {
		got := isLegalSection(test.have)
		if got != test.want {
			t.Errorf("isLegalSection(%s) = %v (wanted \"%v\")", test.have, got, test.want)
		}
	}
}
