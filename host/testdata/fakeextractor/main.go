// fakeextractor is the smallest possible stand-in for a real extractor
// (docs/EXTRACTOR.md §1), built once by host/watch_test.go and reused across
// the tests that need runStore.start to actually run something. It ignores
// every command-line argument a real extractor would get (--root,
// --include, --exclude, --edges) since watch.go and runs.go build those the
// same way for any extractor and this test double does not need them: it
// just prints SEMAPS_FAKE_FACTS's content to stdout, or fails when
// SEMAPS_FAKE_FAIL=1 is set, exactly as EXTRACTOR.md §1 specifies for the
// two outcomes a host has to handle.
package main

import (
	"fmt"
	"os"
)

func main() {
	if os.Getenv("SEMAPS_FAKE_FAIL") == "1" {
		fmt.Fprintln(os.Stderr, "fake extractor: forced failure")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Getenv("SEMAPS_FAKE_FACTS"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Stdout.Write(data)
}
