package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

const gloveURL = "https://nlp.stanford.edu/data/glove.2024.wikigiga.100d.zip"

// ensureFile downloads to path if it doesn't exist.
func ensureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return downloadTo(gloveURL, path)
}

func downloadTo(url, path string) error {
	resp, err := http.Get(url) //nolint:gosec // url is caller-supplied via CLI flag
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	tmp := path + ".partial"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	pw := newProgressWriter("downloading", resp.ContentLength)
	if _, err := io.Copy(f, teeToProgress(resp.Body, pw)); err != nil {
		pw.Done()
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	pw.Done()
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// openFirstZipEntry opens the first file inside a zip archive.
func openFirstZipEntry(path string) (io.ReadCloser, func(), error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, err
	}
	if len(zr.File) == 0 {
		_ = zr.Close()
		return nil, nil, fmt.Errorf("zip is empty")
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		_ = zr.Close()
		return nil, nil, err
	}
	return rc, func() { _ = rc.Close(); _ = zr.Close() }, nil
}
