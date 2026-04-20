// Command loader downloads the GloVe Wikipedia+Gigaword 100d dataset and
// bulk-inserts all 1.2M embeddings into a running vecstore via POST /vectors.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	var (
		url          = flag.String("url", "http://localhost:8888", "vecstore base URL")
		file         = flag.String("file", "glove.2024.wikigiga.100d.zip", "path to GloVe zip (downloaded if missing)")
		dim          = flag.Int("dim", 100, "vector dimension")
		batchSize    = flag.Int("batch-size", 1000, "embeddings per request")
		workers      = flag.Int("workers", 8, "concurrent upload workers")
		retries      = flag.Int("retries", 3, "retries per batch")
		skipDownload = flag.Bool("skip-download", false, "do not try to download the zip")
	)
	flag.Parse()

	if !*skipDownload {
		if err := ensureFile(*file); err != nil {
			die("download: %v", err)
		}
	}

	rc, cleanup, err := openFirstZipEntry(*file)
	if err != nil {
		die("open zip: %v", err)
	}
	defer cleanup()

	up := newUploader(*url, *batchSize, *workers, *retries)
	up.start()

	start := time.Now()
	var lines int64
	var batch []uploadItem
	err = streamLines(rc, *dim, func(label string, vec []float32) error {
		lines++
		batch = append(batch, uploadItem{Label: label, Data: vec})
		if len(batch) >= *batchSize {
			up.submit(batch)
			batch = nil
		}
		if lines%50000 == 0 {
			elapsed := time.Since(start)
			fmt.Fprintf(os.Stderr, "%d lines parsed, %.0f lines/s\n",
				lines, float64(lines)/elapsed.Seconds())
		}
		return nil
	})
	if err != nil && err != io.EOF {
		die("parse: %v", err)
	}
	if len(batch) > 0 {
		up.submit(batch)
	}

	total, uploadErr := up.finish()
	if uploadErr != nil {
		die("upload: %v", uploadErr)
	}
	elapsed := time.Since(start)
	fmt.Printf("uploaded %d vectors in %s (%.0f vec/s)\n",
		total, elapsed.Round(time.Millisecond),
		float64(total)/elapsed.Seconds())
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
