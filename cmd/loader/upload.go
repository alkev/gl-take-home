package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type uploadItem struct {
	Label string    `json:"label"`
	Data  []float32 `json:"data"`
}

type uploader struct {
	url       string
	batchSize int
	retries   int
	workers   int
	client    *http.Client
	jobs      chan []uploadItem
	wg        sync.WaitGroup
	total     int64
	totalMu   sync.Mutex
	errOnce   sync.Once
	firstErr  error
}

func newUploader(url string, batchSize, workers, retries int) *uploader {
	return &uploader{
		url: url, batchSize: batchSize, retries: retries, workers: workers,
		client: &http.Client{Timeout: 60 * time.Second},
		jobs:   make(chan []uploadItem, workers*2),
	}
}

func (u *uploader) start() {
	for i := 0; i < u.workers; i++ {
		u.wg.Add(1)
		go u.worker()
	}
}

func (u *uploader) worker() {
	defer u.wg.Done()
	for batch := range u.jobs {
		if err := u.send(batch); err != nil {
			u.errOnce.Do(func() { u.firstErr = err })
		} else {
			u.totalMu.Lock()
			u.total += int64(len(batch))
			u.totalMu.Unlock()
		}
	}
}

func (u *uploader) send(batch []uploadItem) error {
	body, err := json.Marshal(map[string]any{"embeddings": batch})
	if err != nil {
		return err
	}
	var attempt int
	for {
		attempt++
		req, err := http.NewRequest("POST", u.url+"/vectors", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := u.client.Do(req)
		if err == nil && resp.StatusCode == 201 {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if attempt > u.retries {
			if err != nil {
				return fmt.Errorf("upload failed after %d attempts: %w", attempt, err)
			}
			return fmt.Errorf("upload failed after %d attempts: status %d", attempt, resp.StatusCode)
		}
		time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
	}
}

func (u *uploader) submit(batch []uploadItem) {
	u.jobs <- batch
}

func (u *uploader) finish() (int64, error) {
	close(u.jobs)
	u.wg.Wait()
	return u.total, u.firstErr
}
