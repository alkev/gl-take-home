package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// asciiSpace reports whether r is an ASCII space or tab. GloVe files use
// these as the canonical field separator; some tokens legitimately contain
// other unicode-whitespace characters (e.g. U+00A0 in "2 1/2") and must not
// be split. Using strings.Fields would be wrong because it splits on those.
func asciiSpace(r rune) bool {
	return r == ' ' || r == '\t'
}

// parseLine parses one GloVe line: "word f1 f2 ... fN".
func parseLine(line string, dim int) (string, []float32, error) {
	// Trim trailing CR in case of \r\n line endings; bufio.ScanLines strips
	// \n but leaves \r inside the token.
	line = strings.TrimRight(line, "\r")
	fields := strings.FieldsFunc(line, asciiSpace)
	if len(fields) != dim+1 {
		return "", nil, fmt.Errorf("expected %d fields, got %d", dim+1, len(fields))
	}
	vec := make([]float32, dim)
	for i := 0; i < dim; i++ {
		f, err := strconv.ParseFloat(fields[i+1], 32)
		if err != nil {
			return "", nil, fmt.Errorf("field %d: %w", i+1, err)
		}
		vec[i] = float32(f)
	}
	return fields[0], vec, nil
}

// streamLines reads lines from r and invokes cb for each parsed line.
func streamLines(r io.Reader, dim int, cb func(string, []float32) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		label, vec, err := parseLine(line, dim)
		if err != nil {
			return err
		}
		if err := cb(label, vec); err != nil {
			return err
		}
	}
	return sc.Err()
}
