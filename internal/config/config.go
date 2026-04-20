package config

import (
	"fmt"
	"strconv"
	"time"
)

// Config holds service configuration parsed from environment variables.
type Config struct {
	Port             int
	VectorDimension  int
	LogLevel         string
	SnapshotPath     string
	SnapshotInterval time.Duration
	InitialCapacity  int
	ChunkSize        int
	MaxBatchSize     int
	MaxRequestBytes  int64
}

// Parse reads configuration from the given env-accessor function. Pass
// os.Getenv in production; tests pass stubs. VECTOR_DIMENSION is locked
// to 100 per the assignment brief.
func Parse(get func(string) string) (*Config, error) {
	c := &Config{
		Port:             8888,
		VectorDimension:  100,
		LogLevel:         "info",
		SnapshotInterval: 300 * time.Second,
		ChunkSize:        16384,
		MaxBatchSize:     10000,
		MaxRequestBytes:  64 << 20,
	}
	if err := intFromEnv(get, "PORT", &c.Port); err != nil {
		return nil, err
	}
	if err := intFromEnv(get, "VECTOR_DIMENSION", &c.VectorDimension); err != nil {
		return nil, err
	}
	if c.VectorDimension != 100 {
		return nil, fmt.Errorf("VECTOR_DIMENSION must be 100, got %d", c.VectorDimension)
	}
	if v := get("LOG_LEVEL"); v != "" {
		switch v {
		case "debug", "info", "warn", "error":
			c.LogLevel = v
		default:
			return nil, fmt.Errorf("invalid LOG_LEVEL %q", v)
		}
	}
	c.SnapshotPath = get("SNAPSHOT_PATH")
	if v := get("SNAPSHOT_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("SNAPSHOT_INTERVAL: %w", err)
		}
		c.SnapshotInterval = d
	}
	if err := intFromEnv(get, "INITIAL_CAPACITY", &c.InitialCapacity); err != nil {
		return nil, err
	}
	if err := intFromEnv(get, "CHUNK_SIZE", &c.ChunkSize); err != nil {
		return nil, err
	}
	if err := intFromEnv(get, "MAX_BATCH_SIZE", &c.MaxBatchSize); err != nil {
		return nil, err
	}
	if v := get("MAX_REQUEST_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("MAX_REQUEST_BYTES: %w", err)
		}
		c.MaxRequestBytes = n
	}
	return c, nil
}

func intFromEnv(get func(string) string, key string, out *int) error {
	v := get(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	*out = n
	return nil
}
