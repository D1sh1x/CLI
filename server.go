package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultAddr = ":8080"
	apiPathGrep = "/v1/grep"
)

type serverConfig struct {
	addr      string
	nodeID    string
	workers   int
	localFile string
}

func runServer(cfg serverConfig) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc(apiPathGrep, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req GrepRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		var lines []string
		var err error

		if len(req.Lines) > 0 {
			lines = req.Lines
		} else if cfg.localFile != "" {
			lines, err = readAllLinesFromFile(cfg.localFile)
			if err != nil {
				writeJSON(w, http.StatusOK, GrepResponse{JobID: req.JobID, Node: cfg.nodeID, Error: err.Error()})
				return
			}
		} else {
			writeJSON(w, http.StatusOK, GrepResponse{JobID: req.JobID, Node: cfg.nodeID, Error: "no data (lines empty and no local file)"})
			return
		}

		workers := req.Workers
		if workers <= 0 {
			if cfg.workers > 0 {
				workers = cfg.workers
			} else {
				workers = defaultWorkers
			}
		}

		matches, count, e := grepLines(req.Pattern, req.Flags, lines, workers)
		if e != nil {
			writeJSON(w, http.StatusOK, GrepResponse{JobID: req.JobID, Node: cfg.nodeID, Error: e.Error()})
			return
		}
		writeJSON(w, http.StatusOK, GrepResponse{JobID: req.JobID, Node: cfg.nodeID, Matches: matches, Count: count})
	})

	srv := &http.Server{
		Addr:              cfg.addr,
		Handler:           logMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Printf("mygrep server %s listening on %s (node=%s)\n", version, cfg.addr, cfg.nodeID)

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
