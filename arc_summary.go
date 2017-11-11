// Print basic data on ZFS Adjustable Replacement Cache (ARC) on Linux systems
// Copyright (c) 2017 Scot W. Stevenson <scot.stevenson@gmail.com>
//
// Based on arc_summary.py by Ben Rockwood, Martin Matushka, Jason Hellenthal
// and others
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
//
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
//
//  THIS SOFTWARE IS PROVIDED BY AUTHOR AND CONTRIBUTORS ``AS IS'' AND
//  ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
//  IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
//  ARE DISCLAIMED.  IN NO EVENT SHALL AUTHOR OR CONTRIBUTORS BE LIABLE
//  FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
//  DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS
//  OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION)
//  HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT
//  LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY
//  OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF
//  SUCH DAMAGE.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	lineLen      = 72
	procPath     = "/proc/spl/kstat/zfs/"
	tunablesPath = "/sys/module/zfs/parameters"
	dateFormat   = "Mon Jan 1 03:04:00 2006"
)

var (
	useAltDisplay = flag.Bool("a", false, "Alternate display of tunables")
	printDesc     = flag.Bool("d", false, "Include descriptions of tunables")
	printRaw      = flag.Bool("r", false, "Print raw (but sorted) data")
	printGraphic  = flag.Bool("g", false, "Print basic information as graphic")
	showSection   = flag.String("s", "", "Pick one section (arc, dmu, l2arc, tunables, vdev, xuio, zfetch, zil)")

	procPaths []string

	kstats = make(map[string][]string)

	// These are also the short inputs for the -p flag, in addition to "tunables"
	// and "l2arc" (part of arcstats)
	sectionPaths = map[string]string{
		"arc":    procPath + "arcstats",
		"dmu":    procPath + "dmu_tx",
		"vdev":   procPath + "vdev_cache_stats",
		"xuio":   procPath + "xuio_stats",
		"zfetch": procPath + "zfetchstats",
		"zil":    procPath + "zil",
	}
)

// formatBytes creates a human-readable version of the number of bytes in SI
// units. This works for 64 bit values (16 EiB)
func formatBytes(b uint64) string {

	units := []string{"Bytes", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}

	var limit, value float64
	var unit string

	if b < 1024 {
		return fmt.Sprintf("%d Bytes", b)
	}

	fbytes := float64(b)

	for i := len(units) - 1; i > 0; i-- {

		limit = math.Pow(float64(2), float64(i*10))

		if fbytes >= limit {
			value = fbytes / limit
			unit = units[i]
			break
		}
	}
	return fmt.Sprintf("%0.1f %s", value, unit)
}

// formatHits returns a human-readable version of the number of hits with SI
// units to describe the size. This works up to a 64 bit number (18.4 EB for
// unsigned int64); see
// https://blogs.oracle.com/bonwick/you-say-zeta,-i-say-zetta for details
func formatHits(hits uint64) string {

	units := []string{" ", "k", "M", "G", "T", "P", "E"}

	var limit, value float64
	var unit string

	// Keep this separate so we give back smaller numbers of hits without
	// decimal points
	if hits < 1000 {
		return fmt.Sprintf("%d", hits)
	}

	fhits := float64(hits)

	// TODO test with i > 1
	for i := len(units) - 1; i > 0; i-- {

		limit = math.Pow10(i * 3)

		if fhits >= limit {
			value = fhits / limit
			unit = units[i]
			break
		}
	}

	return fmt.Sprintf("%0.1f%s", value, unit)
}

// getKstats collects information on the ZFS subsystem from the /proc virtual
// file system. Fun fact: The name "kstat" is a holdover from the Solaris utility
// of the same name
func getKstats(m map[string][]string) {

	for _, s := range sectionPaths {

		f, err := os.Open(s)

		if err != nil {
			log.Fatal("Could not open ", s, " for reading")
		}
		defer f.Close()

		var parameters []string
		input := bufio.NewScanner(f)

		for input.Scan() {
			parameters = append(parameters, input.Text())
		}

		// The first two lines of output are header stuff we don't need
		parameters = parameters[2:len(parameters)]
		sort.Strings(parameters)
		m[s] = parameters
	}
}

// printHeader prints a title strings with the date
func printHeader() {
	line := strings.Repeat("-", lineLen)
	t := time.Now()
	ts := t.Format(dateFormat)
	fmt.Printf("\n%s\nZFS Subsystem Report\t\t\t\t%s\n", line, ts)
}

// printRawData displays the output of all parameters without any formatting.
// TODO missing tunables
func printRawData() {

	var paths []string

	for _, sp := range sectionPaths {
		paths = append(paths, sp)
	}

	sort.Strings(paths)

	for _, p := range paths {
		fmt.Printf("\n%s\n", p)

		for _, l := range kstats[p] {
			fmt.Println("\t", l)
		}
	}
}

func main() {

	flag.Parse()
	getKstats(kstats)
	printHeader()

	if *printGraphic {
		fmt.Println("TODO print graphic")
		os.Exit(0)
	}

	if *printRaw {
		printRawData()
		os.Exit(0)
	}

}
