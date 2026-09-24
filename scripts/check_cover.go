// Command check_cover fails when statement coverage is below -fail-under.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func main() {
	profile := flag.String("profile", "cov.txt", "coverage profile path")
	failUnder := flag.Float64("fail-under", 100, "minimum total %% coverage")
	flag.Parse()

	out, err := exec.Command("go", "tool", "cover", "-func="+*profile).Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "go tool cover: %v\n%s\n", err, out)
		os.Exit(1)
	}
	var total float64
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "total:") {
			continue
		}
		// total: (statements) 100.0%
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pct := strings.TrimSuffix(fields[len(fields)-1], "%")
		total, err = strconv.ParseFloat(pct, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse total %q: %v\n", pct, err)
			os.Exit(1)
		}
	}
	fmt.Printf("coverage total: %.1f%% (fail-under %.1f%%)\n", total, *failUnder)
	if total+1e-9 < *failUnder {
		fmt.Fprintf(os.Stderr, "coverage %.1f%% is below fail-under %.1f%%\n", total, *failUnder)
		os.Exit(1)
	}
}
