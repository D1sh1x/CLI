package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func itoa(n int) string { return strconv.Itoa(n) }
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func splitPeers(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			if !strings.HasPrefix(p, "http://") && !strings.HasPrefix(p, "https://") {
				p = "http://" + p
			}
			out = append(out, p)
		}
	}
	return out
}

func shardLines(lines []string, parts int) [][]string {
	if parts <= 1 {
		return [][]string{lines}
	}
	res := make([][]string, parts)
	for i := range res {
		res[i] = make([]string, 0, len(lines)/parts+1)
	}
	for i, s := range lines {
		res[i%parts] = append(res[i%parts], s)
	}
	return res
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

type respWriter struct {
	http.ResponseWriter
	status int
}

func (w *respWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &respWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)
		fmt.Fprintf(os.Stderr, "%s %s %d %v\n", r.Method, r.URL.Path, ww.status, time.Since(start))
	})
}
