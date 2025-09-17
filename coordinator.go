package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type coordinatorConfig struct {
	pattern  string
	flags    GrepFlags
	file     string
	peers    []string
	workers  int
	timeout  time.Duration
	quorum   int
	jsonOut  bool
	showNode bool
}

const defaultTimeout = 10 * time.Second

func runCoordinator(cfg coordinatorConfig) (int, error) {
	lines, err := readAllLines(cfg.file)
	if err != nil {
		return 2, err
	}

	N := len(cfg.peers) + 1
	required := cfg.quorum
	if required <= 0 {
		required = N/2 + 1
	}
	if required > N {
		return 2, fmt.Errorf("invalid quorum %d > total nodes %d", required, N)
	}

	shards := shardLines(lines, N)

	type result struct {
		resp GrepResponse
		ok   bool
		err  error
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	resultsCh := make(chan result, N)

	go func(data []string) {
		m, cnt, e := grepLines(cfg.pattern, cfg.flags, data, cfg.workers)
		if e != nil {
			resultsCh <- result{resp: GrepResponse{Node: "self"}, ok: false, err: e}
			return
		}
		resultsCh <- result{resp: GrepResponse{JobID: "local", Node: "self", Matches: m, Count: cnt}, ok: true}
	}(shards[0])

	for i, addr := range cfg.peers {
		addr = strings.TrimRight(addr, "/")
		shard := shards[i+1]
		go func(node string, data []string) {
			req := GrepRequest{
				JobID:   fmt.Sprintf("job-%d-%d", time.Now().UnixNano(), i+1),
				Pattern: cfg.pattern,
				Flags:   cfg.flags,
				Lines:   data,
				Workers: cfg.workers,
			}
			b, _ := json.Marshal(req)
			httpClient := &http.Client{Timeout: cfg.timeout / 2}
			url := node + apiPathGrep

			httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
			httpReq.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(httpReq)
			if err != nil {
				resultsCh <- result{resp: GrepResponse{Node: node}, ok: false, err: err}
				return
			}
			defer resp.Body.Close()

			var r GrepResponse
			if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
				resultsCh <- result{resp: GrepResponse{Node: node}, ok: false, err: err}
				return
			}
			if r.Error != "" {
				resultsCh <- result{resp: r, ok: false, err: errors.New(r.Error)}
				return
			}
			if r.Node == "" {
				r.Node = node
			}
			resultsCh <- result{resp: r, ok: true}
		}(addr, shard)
	}

	var (
		okCount    int
		totalCount int
		allLines   []string
		collected  int
	)

	for collected < N {
		select {
		case <-ctx.Done():
			return 2, fmt.Errorf("timeout before reaching quorum: %d/%d", okCount, required)
		case res := <-resultsCh:
			collected++
			if res.ok {
				okCount++
				totalCount += res.resp.Count
				if !cfg.flags.CountOnly {
					if cfg.showNode {
						for _, s := range res.resp.Matches {
							allLines = append(allLines, fmt.Sprintf("%s\t%s", res.resp.Node, s))
						}
					} else {
						allLines = append(allLines, res.resp.Matches...)
					}
				}
			} else {
				fmt.Fprintf(os.Stderr, "node %s error: %v\n", res.resp.Node, res.err)
			}
			if okCount >= required {
				if cfg.flags.CountOnly {
					if cfg.jsonOut {
						_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
							"nodes_ok": okCount, "nodes_total": N, "count": totalCount,
						})
					} else {
						fmt.Println(totalCount)
					}
				} else {
					if cfg.jsonOut {
						_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
							"nodes_ok": okCount, "nodes_total": N, "lines": allLines,
						})
					} else {
						for _, s := range allLines {
							fmt.Println(s)
						}
					}
				}
				return 0, nil
			}
		}
	}
	return 2, fmt.Errorf("quorum not reached: %d/%d", okCount, required)
}
