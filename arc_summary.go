// Print basic data on ZFS Adjustable Replacement Cache (ARC) on Linux systems
// Copyright (c) 2017 Scot W. Stevenson <scot.stevenson@gmail.com>
//
// Based on arc_summary.py by Ben Rockwood, Martin Matushka, Jason Hellenthal
// and others. Number of hits and byte sizes limited to 64 bit (float64/uint64)
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
//
// 1. Redistributions of source code must retain the above copyright notice,
//    this list of conditions and the following disclaimer.
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
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
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
	sections    = []string{"arc", "dmu", "l2arc", "tunables", "vdev", "xuio", "zfetch", "zil"}
	sectionHelp = "Print single section (" + strings.Join(sections, ", ") + ")"

	OptPrintAlt     = flag.Bool("a", false, "Alternate (compact) display of tunables")
	OptPrintDesc    = flag.Bool("d", false, "Include descriptions of tunables")
	OptPrintRaw     = flag.Bool("r", false, "Print raw data, sorted alphabetically, and quit")
	OptPrintGraphic = flag.Bool("g", false, "Print basic information as graphic and quit")
	OptPrintSection = flag.String("s", "", sectionHelp)

	procPaths []string

	kstats       = make(map[string][]string)
	tunables     = make(map[string]string)
	tunableDescs = make(map[string]string)

	// These are also the short inputs for the -s flag, in addition to "tunables"
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
// units. This works for 64 bit values (16 EiB); for details see
// https://blogs.oracle.com/bonwick/you-say-zeta,-i-say-zetta
func formatBytes(b uint64) string {

	// First element "Bytes" is dummy value to aid indexing
	units := []string{"Bytes", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}

	var limit, value float64
	var result, unit string

	// Keep this separate so we can return small byte values without decimal
	// points
	if b < 1024 {
		result = fmt.Sprintf("%d Bytes", b)
	} else {
		fbytes := float64(b)

		for i := len(units) - 1; i > 0; i-- {
			limit = math.Pow(float64(2), float64(i*10))

			if fbytes >= limit {
				value = fbytes / limit
				unit = units[i]
				break
			}
		}
		result = fmt.Sprintf("%0.1f %s", value, unit)
	}
	return result
}

// formatHits returns a human-readable version of the number of hits with SI
// units to describe the size. This works up to a 64 bit number (18.4 EB for
// unsigned int64)
func formatHits(hits uint64) string {

	// First element " " is dummy value to aid indexing
	units := []string{" ", "k", "M", "G", "T", "P", "E"}

	var limit, value float64
	var result, unit string

	// Keep this separate so we give back smaller numbers of hits without
	// decimal points
	if hits < 1000 {
		result = fmt.Sprintf("%d", hits)
	} else {
		fhits := float64(hits)

		for i := len(units) - 1; i > 0; i-- {
			limit = math.Pow10(i * 3)

			if fhits >= limit {
				value = fhits / limit
				unit = units[i]
				break
			}
		}
		result = fmt.Sprintf("%0.1f%s", value, unit)
	}
	return result
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

// getTunables collects information on the tunable parameters of the ZFS
// subsystem and returns a string list
func getTunables(m map[string]string) {

	var paraNames []string

	paras, err := ioutil.ReadDir(tunablesPath)
	if err != nil {
		log.Fatal("Couldn't open", tunablesPath, "for tunable parameters")
	}

	for _, p := range paras {
		paraNames = append(paraNames, p.Name())
	}

	for _, pn := range paraNames {
		value, err := ioutil.ReadFile(tunablesPath + "/" + pn)
		if err != nil {
			log.Fatal("Couldn't read", tunablesPath+pn)

		}
		m[pn] = strings.TrimSpace(string(value))
	}
}

// Get the description of each tunable parameter and format it
func getTunableDesc(keys []string, m map[string]string) {

	cmd := exec.Command("/sbin/modinfo", "zfs", "-0")
	out, err := cmd.Output()
	if err != nil {
		log.Fatal("Couldn't get tunable descriptions:", err)
	}

	outstring := strings.Split(string(out), "\000")

	for _, l := range outstring {

		if !strings.HasPrefix(l, "parm:") {
			continue
		}

		// Get rid of "parm:" at beginning and any whitespace
		l = strings.TrimSpace(l[5:len(l)])
		descs := strings.Split(l, ":")

		if len(descs) < 2 {
			m[descs[0]] = "(No description available)"
			continue
		}

		key := strings.TrimSpace(descs[0])
		description := strings.TrimSpace(descs[1])

		m[key] = description
	}
}

// isLegalSection tests to see if string is a legal sections name
func isLegalSection(sec string) bool {
	result := false

	for _, s := range sections {

		if sec == s {
			result = true
		}
	}

	return result
}

// printGraphic prints a small graphic respresentation of the most important ARC
// data and then quits
func printGraphic() {
	fmt.Println("TODO print graphic")
}

// printHeader prints a title strings with the date
func printHeader() {
	line := strings.Repeat("-", lineLen)
	t := time.Now()
	ts := t.Format(dateFormat)
	fmt.Printf("\n%s\nZFS Subsystem Report\t\t\t\t%s\n", line, ts)
}

// printRawData displays the output of all parameters without any formatting or
// further information, but sorted alphabetically
func printRawData() {

	var paths []string

	for _, sp := range sectionPaths {
		paths = append(paths, sp)
	}

	sort.Strings(paths)

	for _, p := range paths {
		fmt.Printf("\n%s:\n", p)

		for _, l := range kstats[p] {
			fmt.Printf("\t%s\n", l)
		}
	}
}

// printTunables displays a list of tunables with the option of adding the
// descriptions and/or using a more compact display.
func printTunables() {

	var printFormat string
	var keys []string

	getTunables(tunables)

	if *OptPrintAlt {
		printFormat = "\t%s=%s\n"
	} else {
		printFormat = "\t%-50s%s\n"
	}

	for k, _ := range tunables {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	if *OptPrintDesc {
		getTunableDesc(keys, tunableDescs)
	}

	for _, k := range keys {

		if *OptPrintDesc {
			fmt.Printf("\t# %s\n", tunableDescs[k])
		}

		fmt.Printf(printFormat, k, tunables[k])
	}

}

func main() {

	flag.Parse()
	getKstats(kstats)
	printHeader()

	if *OptPrintGraphic {
		printGraphic()
		os.Exit(0)
	}

	if *OptPrintRaw {
		fmt.Println("\nPrinting RAW DATA:")
		printRawData()
		fmt.Println("\nTunables:")
		printTunables()
		os.Exit(0)
	}

	if *OptPrintSection != "" {

		if !isLegalSection(*OptPrintSection) {
			log.Fatal("Can't print unknown section '", *OptPrintSection, "'")
		}

		fmt.Printf("\n%s:\n", strings.ToUpper(*OptPrintSection))

		if *OptPrintSection == "tunables" {
			printTunables()
			os.Exit(0)
		}

		os.Exit(0)
	}
}
