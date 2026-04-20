package config

import (
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	get := func(string) string { return "" }
	c, err := Parse(get)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 8888 {
		t.Fatalf("Port = %d, want 8888", c.Port)
	}
	if c.VectorDimension != 100 {
		t.Fatalf("dim = %d, want 100", c.VectorDimension)
	}
	if c.LogLevel != "info" {
		t.Fatalf("log = %q, want info", c.LogLevel)
	}
	if c.ChunkSize != 16384 {
		t.Fatalf("chunk = %d, want 16384", c.ChunkSize)
	}
	if c.SnapshotInterval != 300*time.Second {
		t.Fatalf("interval = %v, want 5m", c.SnapshotInterval)
	}
	if c.MaxBatchSize != 10000 {
		t.Fatalf("MaxBatchSize = %d, want 10000", c.MaxBatchSize)
	}
	if c.MaxRequestBytes != 64<<20 {
		t.Fatalf("MaxRequestBytes = %d, want 64MiB", c.MaxRequestBytes)
	}
}

func TestOverrides(t *testing.T) {
	env := map[string]string{
		"PORT":              "9999",
		"VECTOR_DIMENSION":  "100",
		"LOG_LEVEL":         "debug",
		"CHUNK_SIZE":        "4096",
		"SNAPSHOT_PATH":     "/tmp/x.bin",
		"SNAPSHOT_INTERVAL": "10s",
		"INITIAL_CAPACITY":  "1200000",
		"MAX_BATCH_SIZE":    "5000",
	}
	get := func(k string) string { return env[k] }
	c, err := Parse(get)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 9999 || c.ChunkSize != 4096 || c.SnapshotPath != "/tmp/x.bin" ||
		c.SnapshotInterval != 10*time.Second || c.InitialCapacity != 1200000 ||
		c.MaxBatchSize != 5000 {
		t.Fatalf("override parse wrong: %+v", c)
	}
}

func TestInvalidPort(t *testing.T) {
	get := func(k string) string {
		if k == "PORT" {
			return "not-a-number"
		}
		return ""
	}
	if _, err := Parse(get); err == nil {
		t.Fatal("expected error")
	}
}

func TestInvalidDim(t *testing.T) {
	get := func(k string) string {
		if k == "VECTOR_DIMENSION" {
			return "101"
		}
		return ""
	}
	if _, err := Parse(get); err == nil {
		t.Fatal("dim != 100 must reject")
	}
}
