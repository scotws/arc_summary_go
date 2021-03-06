// Print basic data on ZFS Adjustable Replacement Cache (ARC) on Linux systems
// Copyright (c) 2017 Scot W. Stevenson <scot.stevenson@gmail.com>
//
// Based on arc_summary.py by Ben Rockwood, Martin Matushka, Jason Hellenthal
// and others. Number of hits and byte sizes limited to 64 bit (float64/uint64)
//
// 	*** THIS CODE IS CURRENTLY INCOMPLETE ***
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
	"strconv"
	"strings"
	"time"
)

const (
	procPath     = "/proc/spl/kstat/zfs/"
	tunablesPath = "/sys/module/zfs/parameters"
	dateFormat   = "Mon Jan 1 03:04:00 2006"
	indent       = "\t"
	lineLen      = 72
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

	sectionPaths = map[string]string{
		"arc":    "arcstats",
		"dmu":    "dmu_tx",
		"vdev":   "vdev_cache_stats",
		"xuio":   "xuio_stats",
		"zfetch": "zfetchstats",
		"zil":    "zil",
	}

	sectionCalls = map[string]func(){
		"arc":      printARC,
		"dmu":      printDMU,
		"l2arc":    printL2ARC,
		"tunables": printTunables,
		"vdev":     printVDEV,
		"xuio":     printXuio,
		"zfetch":   printZfetch,
		"zil":      printZIL,
	}
)

// cleanProcLine takes a raw line of the data from /proc and isolates the name and
// value contained, eg "arc_no_grow   4    0" The "4" in the middle is the type
// factor that can be ignored
// TODO deal with errors
func cleanProcLine(s string) (string, string) {
	fields := strings.Fields(s)
	return strings.TrimSpace(fields[0]), strings.TrimSpace(fields[2])
}

// fBytes creates a human-readable version of the number of bytes in SI
// units. This works for 64 bit values (16 EiB); for details see
// https://blogs.oracle.com/bonwick/you-say-zeta,-i-say-zetta
func fBytes(s string) string {

	// First element "Bytes" is dummy value to aid indexing
	units := []string{"Bytes", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}

	var limit, value float64
	var result, unit string

	b := stringToUint64(s)

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

// fHits returns a human-readable version of the number of hits with SI
// units to describe the size. This works up to a 64 bit number (18.4 EB for
// unsigned int64)
func fHits(s string) string {

	// First element " " is dummy value to aid indexing
	units := []string{" ", "k", "M", "G", "T", "P", "E"}

	var limit, value float64
	var result, unit string

	hits := stringToUint64(s)

	// Keep this separate so we give back smaller numbers of hits without
	// decimal points. Leave spaces to align with unit output
	if hits < 1000 {
		result = fmt.Sprintf("%d    ", hits)
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

// fPerc calculates a precentage and returns the number in a human-readable
// format. If percentage cannot be calculated (because of a zero in the lower
// value) a blank string is returned)
func fPerc(upper, lower string) string {

	u, err := strconv.ParseFloat(upper, 64)
	if err != nil {
		log.Fatal("Error converting string ", upper, "to float")
	}

	l, err := strconv.ParseFloat(lower, 64)
	if err != nil {
		log.Fatal("Error converting string ", lower, "to float")
	}

	result := " "

	if l > 0 {
		result = fmt.Sprintf("%0.1f %%", (100 * u / l))
	}

	return result
}

// getKstats collects information on the ZFS subsystem from the /proc virtual
// file system. Fun fact: The name "kstat" is a holdover from the Solaris utility
// of the same name
func getKstats(m map[string][]string) {

	for _, s := range sectionPaths {

		fullPath := procPath + s

		f, err := os.Open(fullPath)

		if err != nil {
			log.Fatal("Could not open ", fullPath, " for reading")
		}
		defer f.Close()

		var parameters []string
		input := bufio.NewScanner(f)

		for input.Scan() {
			parameters = append(parameters, input.Text())
		}

		// We use a short version of the section path as the key, eg
		// "arcstats" instead of "/proc/spl/kstat/zfs/arcstats"
		w := strings.Split(s, "/")
		key := w[len(w)-1]

		// The first two lines of output are header stuff we don't need
		parameters = parameters[2:len(parameters)]
		sort.Strings(parameters)
		m[key] = parameters
	}
}

// getTunables collects information on the tunable parameters of the ZFS
// subsystem and returns them in a map
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

// Get the description of each tunable parameter and format it. For more
// information on what each parameter does on a Linux system, see
// "man 5 zfs-module-parameters"
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

		key := strings.TrimSpace(descs[0])

		if len(descs) < 2 {
			m[key] = "(No description available)"
			continue
		}

		// Drop useless information on internal format (eg "(uint)"). Some
		// of the descriptions have comments within paras so we can't
		// just split on "("
		description := descs[1]
		idx := strings.LastIndex(description, "(")

		if idx != -1 {
			description = description[0:idx]
		}

		m[key] = strings.TrimSpace(description)
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

	const (
		graphIndent = "      "
		graphWidth  = 70 // may not be smaller then length of status line // may not be smaller then length of status line
		graphNote   = "('F': MFU size  'R': MRU size  'O': Other)\n\n"
	)

	var (
		arcStats = make(map[string]string)
		line     = graphIndent + "+" + strings.Repeat("-", graphWidth-2) + "+"
		bar      = graphIndent + "|%s%s%s%s|\n"
		infoLine = graphIndent + "ARC: %s/%s (%s)  MFU: %s  MRU: %s"
	)

	procSection("arcstats", arcStats)

	arcSize := arcStats["size"]
	arcMaxSize := arcStats["c_max"]
	mfuSize := arcStats["mfu_size"]
	mruSize := arcStats["mru_size"]
	arcPerc := fPerc(arcStats["size"], arcStats["c_max"])

	mfuBytes := stringToUint64(mfuSize)
	mruBytes := stringToUint64(mruSize)
	arcMaxBytes := stringToUint64(arcMaxSize)
	otherBytes := stringToUint64(arcSize) - (mfuBytes + mruBytes)

	mfuChars := strings.Repeat("F", int((mfuBytes*graphWidth)/arcMaxBytes))
	mruChars := strings.Repeat("R", int((mruBytes*graphWidth)/arcMaxBytes))
	otherChars := strings.Repeat("O", int((otherBytes*graphWidth)/arcMaxBytes))

	whiteSpace := graphWidth - 2 - (len(mfuChars) + len(mruChars) + len(otherChars))

	statusLine := fmt.Sprintf(infoLine, fBytes(arcSize), fBytes(arcMaxSize), arcPerc,
		fBytes(mfuSize), fBytes(mruSize))
	offsetStatusLine := (graphWidth - (len(statusLine)) + len(graphIndent)) / 2
	paddingStatusLine := strings.Repeat(" ", offsetStatusLine)

	offsetInfoLine := (graphWidth - (len(infoLine)) + len(graphIndent)) / 2
	paddingInfoLine := strings.Repeat(" ", offsetInfoLine)

	fmt.Printf("\n%s%s", paddingStatusLine, statusLine)
	fmt.Printf("\n%s\n", line)
	fmt.Printf(bar, mfuChars, mruChars, otherChars, strings.Repeat(" ", whiteSpace))
	fmt.Println(line)
	fmt.Printf("%s%s", paddingInfoLine, graphNote)

}

// printHeader prints the title with the date and time
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
		fmt.Printf("\n%s:\n", strings.ToUpper(p))

		for _, l := range kstats[p] {
			name, value := cleanProcLine(l)
			fmt.Printf("\t%-50s%s\n", name, value)
		}
	}
}

// prtL* are formatting functions to print formatted output. All of these assume
// a width of 72 characters for the output

// prtL1 prints primary level format without percentage
func prtL1(msg, value string) {
	var l1 = "\n%-61s%11s\n"
	fmt.Printf(l1, msg, value)
}

// prtL2 prints secondary level format without percentage
func prtL2(msg, value string) {
	var l2 = indent + "%-53s%11s\n"
	fmt.Printf(l2, msg, value)
}

// prtL1p prints first level format with percentage
func prtL1p(msg, perc, value string) {
	var l1p = "\n%-55s%6s%11s\n"
	fmt.Printf(l1p, msg, perc, value)
}

// prtL2p prints second level format with percentage
func prtL2p(msg, perc, value string) {
	var l2p = indent + "%-47s%6s%11s\n"
	fmt.Printf(l2p, msg, perc, value)
}

// printARC displays formatted information on the most important ARC
// parameters in human-readable format. This excludes the L2ARC, which is
// printed in its own section. The layout follows the original arc_summary.py to
// make switching easier.
func printARC() {

	var arcStats = make(map[string]string)
	procSection("arcstats", arcStats)

	throttle := arcStats["memory_throttle_count"]
	health := "HEALTHY"

	if throttle != "0" {
		health = "THROTTLED"
	}

	prtL1("ARC summary:", health)
	prtL2("Memory throttle count:", fHits(throttle))

	arcSize := fBytes(arcStats["size"])
	arcPerc := fPerc(arcStats["size"], arcStats["c_max"])
	prtL1p("ARC size:", arcPerc, arcSize)
	prtL2p("Target size (adaptive):", "FEHLT", fBytes(arcStats["c"]))

	maxSize := arcStats["c_max"]
	minSize := arcStats["c_min"]
	prtL2p("Min size (hard limit):", "FEHLT", fBytes(minSize))
	prtL2p("Max size (high water):", "FEHLT", fBytes(maxSize))

	fmt.Println("\nARC size breakdown:")
	mfuSize := arcStats["mfu_size"]
	mruSize := arcStats["mru_size"]
	cacheTotal := stringToUint64(mfuSize) + stringToUint64(mruSize)
	cacheTotalString := strconv.FormatUint(cacheTotal, 10)
	mfuPerc := fPerc(mfuSize, cacheTotalString)
	mruPerc := fPerc(mruSize, cacheTotalString)
	prtL2p("Most Frequently Used (MFU) cache size:", mfuPerc, fBytes(mfuSize))
	prtL2p("Most Recently Used (MRU) cache size:", mruPerc, fBytes(mruSize))

}

// printDMU displays the statistics related to the DMU
// TODO - figure out some of these statistics are from ZFETCH
func printDMU() {

	var dmuStats = make(map[string]string)
	procSection("dmu_tx", dmuStats)

	dmuEfficiency := dmuStats["efficiency"]

	fmt.Println("TODO Print DMU statistics")
	fmt.Println("TEST (efficiency)", dmuEfficiency)
}

// printL2ARC displays the statistics related to the L2ARC if one is
// installed
func printL2ARC() {
	fmt.Println("TODO Print L2ARC statistics")
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

// printVDEV displays statistics related to the Virtual Devices
func printVDEV() {
	var vdevStats = make(map[string]string)
	procSection("vdev_cache_stats", vdevStats)

	delegations := vdevStats["delegations"]
	misses := vdevStats["misses"]
	hits := vdevStats["hits"]

	t := stringToUint64(delegations) + stringToUint64(misses) + stringToUint64(hits)
	total := strconv.FormatUint(t, 10)

	hitRatio := fPerc(hits, total)
	missRatio := fPerc(misses, total)
	delegationsRatio := fPerc(delegations, total)

	prtL1("VDEV summary: ", " ") // Could list total here as "events"
	prtL2p("Cache hits:", hitRatio, fHits(hits))
	prtL2p("Cache misses:", missRatio, fHits(hits))
	prtL2p("Cache delegations:", delegationsRatio, fHits(hits))
}

// printXuio displays the statistics related to the Virtual Devices
func printXuio() {
	fmt.Println("TODO Print Xuio statistics")
}

// printZfetch displays the statistics related to zfetch
func printZfetch() {
	fmt.Println("TODO Print zfetch stuff")
}

// printZIL displays the statistics related to the ZIL
func printZIL() {
	fmt.Println("TODO Print ZIL stuff")
}

// procSection splits up the statistics on a given section which are first
// only bundled up in kstats. This gives us the option to only sort the
// individual statistics when we actually need them
func procSection(s string, m map[string]string) {

	arcstats, ok := kstats[s]
	if !ok {
		log.Fatal("Internal error: Can't access data on section", s)
	}

	for _, l := range arcstats {
		name, value := cleanProcLine(l)
		m[name] = value
	}
}

// stringToUint64 takes a string with a number and converts it to feed into one
// of the conversion processes for ftBytes or formatHints
func stringToUint64(s string) uint64 {

	i, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		log.Fatal("Error converting ", s, " to uint64: ", err)
	}

	return uint64(i)
}

func main() {

	flag.Parse()
	getKstats(kstats)

	if *OptPrintGraphic {
		printGraphic()
		os.Exit(0)
	}

	printHeader()

	if *OptPrintRaw {
		printRawData()
		fmt.Println("\nTUNABLES:")
		printTunables()
		os.Exit(0)
	}

	if *OptPrintSection != "" {

		if !isLegalSection(*OptPrintSection) {
			log.Fatal("Can't print unknown section '", *OptPrintSection, "'")
		}

		fmt.Printf("\n--- %s ---\n", strings.ToUpper(*OptPrintSection))
		sectionCalls[*OptPrintSection]()
		os.Exit(0)
	}

	// If no parameter given, just print everything except the graphic
	for _, s := range sections {
		fmt.Printf("\n--- %s ---\n", strings.ToUpper(s))
		sectionCalls[s]()
	}
	os.Exit(0)
}
