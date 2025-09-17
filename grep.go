package main

import (
	"regexp"
	"strings"
	"sync"
)

const (
	defaultWorkers = 4
	defaultScanBuf = 1024 * 1024 // 1 MiB
)

// Параллельный греп по строкам с пулом воркеров.
func grepLines(pattern string, gf GrepFlags, lines []string, workers int) (matches []string, count int, err error) {
	if workers <= 0 {
		workers = defaultWorkers
	}

	// matcher
	var tester func(string) bool
	if gf.Fixed {
		target := pattern
		if gf.IgnoreCase {
			target = strings.ToLower(target)
		}
		tester = func(s string) bool {
			if gf.IgnoreCase {
				return strings.Contains(strings.ToLower(s), target)
			}
			return strings.Contains(s, target)
		}
	} else {
		reFlags := ""
		if gf.IgnoreCase {
			reFlags = "(?i)"
		}
		re, e := regexp.Compile(reFlags + pattern)
		if e != nil {
			return nil, 0, e
		}
		tester = func(s string) bool { return re.MatchString(s) }
	}

	in := make(chan string, 1024)
	out := make(chan string, 1024)

	var wg sync.WaitGroup
	const countPrefix = "\x00COUNT:"

	worker := func() {
		defer wg.Done()
		localCount := 0
		for s := range in {
			ok := tester(s)
			if gf.Invert {
				ok = !ok
			}
			if ok {
				if !gf.CountOnly {
					out <- s
				}
				localCount++
				if gf.MaxCount > 0 && localCount >= gf.MaxCount {
					break
				}
			}
		}
		if localCount > 0 {
			out <- countPrefix + itoa(localCount)
		}
	}

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go worker()
	}

	go func() {
		for _, s := range lines {
			in <- s
		}
		close(in)
	}()

	var collected []string
	totalCount := 0

	go func() {
		wg.Wait()
		close(out)
	}()

	for item := range out {
		if strings.HasPrefix(item, countPrefix) {
			n := atoi(strings.TrimPrefix(item, countPrefix))
			totalCount += n
			continue
		}
		collected = append(collected, item)
		// MaxCount ограничивает только строки, а не подсчёт
	}

	if gf.CountOnly {
		return nil, totalCount, nil
	}
	return collected, totalCount, nil
}
