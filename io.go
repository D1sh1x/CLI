package main

import (
	"bufio"
	"io"
	"os"
)

func readAllLines(file string) ([]string, error) {
	if file == "" || file == "-" {
		return readAllLinesFromReader(os.Stdin)
	}
	return readAllLinesFromFile(file)
}

func readAllLinesFromFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readAllLinesFromReader(f)
}

func readAllLinesFromReader(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(r)
	buf := make([]byte, defaultScanBuf)
	sc.Buffer(buf, defaultScanBuf)
	out := make([]string, 0, 1024)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
