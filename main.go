package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		serveCmd(os.Args[2:])
	case "run":
		runCmd(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		runCmd(os.Args[1:])
	}
}

func serveCmd(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", defaultAddr, "HTTP listen address")
	node := fs.String("node", "", "node id (default: <host:port>)")
	workers := fs.Int("workers", defaultWorkers, "worker goroutines per node")
	localFile := fs.String("data-file", "", "optional local file to grep if request doesn't provide lines")
	_ = fs.Parse(args)

	nodeID := *node
	if nodeID == "" {
		nodeID = *addr
	}
	if err := runServer(serverConfig{
		addr: *addr, nodeID: nodeID, workers: *workers, localFile: *localFile,
	}); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	pattern := fs.String("pattern", "", "pattern for matching (or positional arg #1)")
	file := fs.String("file", "", "read input from file (default: stdin)")
	peersStr := fs.String("peers", "", "comma-separated peer base URLs (e.g. http://host1:8080,http://host2:8081)")
	workers := fs.Int("workers", runtime.GOMAXPROCS(0), "workers per node")
	timeout := fs.Duration("timeout", defaultTimeout, "overall timeout")
	quorum := fs.Int("quorum", 0, "quorum size (default: majority)")
	jsonOut := fs.Bool("json", false, "output JSON")
	showNode := fs.Bool("show-node", false, "prefix lines with node id")

	useFixed := fs.Bool("F", false, "fixed string match")
	useRegex := fs.Bool("E", true, "regex (default)")
	ignoreCase := fs.Bool("i", false, "ignore case")
	invert := fs.Bool("v", false, "invert match")
	maxCount := fs.Int("m", 0, "stop after NUM matches (per node)")
	countOnly := fs.Bool("c", false, "print only count")

	_ = fs.Parse(args)

	if *pattern == "" && fs.NArg() > 0 {
		*pattern = fs.Arg(0)
	}

	if *pattern == "" {
		fmt.Fprintln(os.Stderr, "pattern is required")
		os.Exit(2)
	}

	gf := GrepFlags{
		Regex:      *useRegex || !*useFixed,
		Fixed:      *useFixed && !*useRegex,
		IgnoreCase: *ignoreCase,
		Invert:     *invert,
		MaxCount:   *maxCount,
		CountOnly:  *countOnly,
	}

	peers := splitPeers(*peersStr)
	if len(peers) == 0 {
		lines, err := readAllLines(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read error:", err)
			os.Exit(2)
		}
		m, cnt, err := grepLines(*pattern, gf, lines, *workers)
		if err != nil {
			fmt.Fprintln(os.Stderr, "grep error:", err)
			os.Exit(2)
		}
		if gf.CountOnly {
			if *jsonOut {
				_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"count": cnt})
			} else {
				fmt.Println(cnt)
			}
			return
		}
		if *jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"lines": m})
		} else {
			for _, s := range m {
				if *showNode {
					fmt.Printf("self\t%s\n", s)
				} else {
					fmt.Println(s)
				}
			}
		}
		return
	}

	code, err := runCoordinator(coordinatorConfig{
		pattern:  *pattern,
		flags:    gf,
		file:     *file,
		peers:    peers,
		workers:  *workers,
		timeout:  *timeout,
		quorum:   *quorum,
		jsonOut:  *jsonOut,
		showNode: *showNode,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "distributed error:", err)
	}
	os.Exit(code)
}

func usage() {
	name := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, `
%s %s — распределённый grep

ЛОКАЛЬНО:
  cat data.txt | %s -E -i -pattern "error"
  %s -F -pattern "needle" -file data.txt
  %s -c -pattern "WARN" -file logs.txt

СЕРВЕР:
  %s serve -addr :8080 -node nodeA
  %s serve -addr :8081 -node nodeB

РАСПРЕДЕЛЁННО:
  cat big.log | %s -pattern "timeout" -peers http://localhost:8080,http://localhost:8081 -show-node
  %s -c -pattern "ERROR" -file big.log -peers http://nodeA:8080,http://nodeB:8081 -timeout 15s

КЛЮЧИ:
  run (по умолчанию), serve
  -pattern STR (обязателен), -file PATH (или stdin)
  -peers URL1,URL2,...        список узлов
  -quorum N                   по умолчанию majority (N/2+1)
  -i, -F, -E, -v, -m NUM, -c
  -workers N, -timeout 10s, -json, -show-node
`, name, version, name, name, name, name, name, name, name)
}
