// Command semaps-extract-go prints code facts for Go sources on stdout, per
// docs/EXTRACTOR.md (output contract) and docs/extractors/go.md (the normative
// Go mapping: kinds, edges, member-relation features).
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// options are the command line of EXTRACTOR.md §1 plus --implements (ADR_20260927 §5).
type options struct {
	root       string
	includes   []string
	excludes   []string
	edges      map[string]bool // holds, uses, injects
	implements []string        // external interfaces by full name
}

const usage = "usage: semaps-extract-go [--root <dir>] [--include <path>]... [--exclude <glob>]... [--edges holds,uses,injects] [--implements <pkg.Name>,...]"

func parseArgs(args []string) (*options, error) {
	o := &options{root: ".", edges: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, hasVal := strings.Cut(a, "=")
		take := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", name)
			}
			i++
			return args[i], nil
		}
		switch name {
		case "--root", "--include", "--exclude", "--edges", "--implements":
			v, err := take()
			if err != nil {
				return nil, err
			}
			switch name {
			case "--root":
				if v == "" {
					return nil, fmt.Errorf("--root is empty")
				}
				o.root = v
			case "--include":
				o.includes = append(o.includes, v)
			case "--exclude":
				o.excludes = append(o.excludes, v)
			case "--edges":
				for _, k := range strings.Split(v, ",") {
					k = strings.TrimSpace(k)
					if k == "" {
						continue
					}
					if k != "holds" && k != "uses" && k != "injects" {
						return nil, fmt.Errorf("unknown edge kind: %s", k)
					}
					o.edges[k] = true
				}
			case "--implements":
				for _, n := range strings.Split(v, ",") {
					n = strings.TrimSpace(n)
					if n == "" {
						continue
					}
					if strings.ContainsAny(n, "*?[ ") {
						return nil, fmt.Errorf("--implements takes full names without masks: %s", n)
					}
					o.implements = append(o.implements, n)
				}
			}
		case "-h", "--help":
			return nil, fmt.Errorf("%s", usage)
		default:
			return nil, fmt.Errorf("unknown argument: %s", a)
		}
	}
	return o, nil
}

// run is main without os.Exit, so the tests call it in-process.
func run(args []string, stdout, stderr io.Writer) int {
	o, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if st, err := os.Stat(o.root); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "--root is not a directory: %s\n", o.root)
		return 2
	}
	facts, err := extract(o, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	out, err := marshal(facts)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if _, err := stdout.Write(out); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
