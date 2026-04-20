// Command semcheck runs the semantic-plausibility spot-check against a
// running, loaded vecstore.
//
// For each line in the fixture, it calls /nearest?word=<query>&k=K and
// PASSes if any of the returned top-K labels is in the expected set,
// FAILs otherwise. Top-K (not top-1) matches the fixture's own premise
// that GloVe neighbourhoods are semantically plausible but the *exact*
// closest word is often an artifact (e.g. high-frequency co-occurrence
// or morphological variants outside the handwritten expected set).
// Prints a per-query table, a summary, and exits non-zero if any query
// failed — so it fits into CI or an acceptance gate.
//
// Fixture format (blank lines and `#`-comments ignored):
//
//	query<TAB or spaces>plausible1,plausible2,plausible3,...
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type nearestResp struct {
	Query   string `json:"query"`
	Results []struct {
		Label string `json:"label"`
	} `json:"results"`
}

type query struct {
	word     string
	expected []string // lowercased
}

type outcome struct {
	query string
	got   []string // top-K labels, best first
	hit   string   // first label that matched the expected set (empty on FAIL)
	pass  bool
	note  string
}

func main() {
	var (
		url     = flag.String("url", "http://localhost:8888", "vecstore base URL")
		fixture = flag.String("fixture", "testdata/semantic_queries.txt", "path to semantic_queries.txt")
		timeout = flag.Duration("timeout", 5*time.Second, "per-request timeout")
		k       = flag.Int("k", 5, "top-K neighbours to accept as a match")
	)
	flag.Parse()
	if *k < 1 {
		die("k must be >= 1")
	}

	queries, err := loadFixture(*fixture)
	if err != nil {
		die("load fixture: %v", err)
	}

	client := &http.Client{Timeout: *timeout}
	results := make([]outcome, 0, len(queries))

	topCol := fmt.Sprintf("TOP-%d", *k)
	fmt.Printf("%-15s  %-40s  %s\n", "QUERY", topCol, "RESULT")
	fmt.Println(strings.Repeat("-", 75))

	var passes int
	for _, q := range queries {
		out := runOne(client, *url, q, *k)
		results = append(results, out)
		verdict := "PASS"
		if out.pass {
			passes++
			if out.hit != "" && (len(out.got) == 0 || out.hit != out.got[0]) {
				verdict = "PASS (via " + out.hit + ")"
			}
		} else {
			verdict = "FAIL"
		}
		note := out.note
		if note != "" {
			note = " (" + note + ")"
		}
		fmt.Printf("%-15s  %-40s  %s%s\n", out.query, strings.Join(out.got, ","), verdict, note)
	}

	fmt.Println(strings.Repeat("-", 75))
	fmt.Printf("Summary: %d / %d passed (%.1f%%) at k=%d\n",
		passes, len(results), 100*float64(passes)/float64(len(results)), *k)

	if passes != len(results) {
		os.Exit(1)
	}
}

func loadFixture(path string) ([]query, error) {
	f, err := os.Open(path) //nolint:gosec // path is caller-supplied via CLI flag
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []query
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("bad fixture line: %q", line)
		}
		expected := strings.Split(fields[1], ",")
		for i, s := range expected {
			expected[i] = strings.ToLower(strings.TrimSpace(s))
		}
		out = append(out, query{word: fields[0], expected: expected})
	}
	return out, sc.Err()
}

func runOne(client *http.Client, baseURL string, q query, k int) outcome {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/nearest?word=%s&k=%d", baseURL, q.word, k), nil)
	if err != nil {
		return outcome{query: q.word, note: "build request: " + err.Error()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return outcome{query: q.word, note: "request: " + err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return outcome{query: q.word, note: "query word not in store"}
	}
	if resp.StatusCode != http.StatusOK {
		return outcome{query: q.word, note: fmt.Sprintf("status %d", resp.StatusCode)}
	}
	var body nearestResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return outcome{query: q.word, note: "decode: " + err.Error()}
	}
	if len(body.Results) == 0 {
		return outcome{query: q.word, note: "no results"}
	}
	got := make([]string, len(body.Results))
	for i, r := range body.Results {
		got[i] = strings.ToLower(r.Label)
	}
	want := make(map[string]struct{}, len(q.expected))
	for _, w := range q.expected {
		want[w] = struct{}{}
	}
	for _, g := range got {
		if _, ok := want[g]; ok {
			return outcome{query: q.word, got: got, hit: g, pass: true}
		}
	}
	return outcome{query: q.word, got: got, note: "none of top-K in expected set"}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
