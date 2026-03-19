// Package transport handles compressing and sending aggregated batches from
// the agent to the cloud server. Batches are JSON-encoded, gzip-compressed,
// and POSTed with Content-Encoding: gzip.
//
// Typical compression ratios: 200 KB JSON → ~30 KB gzipped.
package transport

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"
)

// CompressResult holds the compressed payload and metadata about the
// compression for logging and rate-limiting decisions.
type CompressResult struct {
	// Data is the gzip-compressed JSON payload.
	Data []byte

	// OriginalSize is the JSON payload size before compression.
	OriginalSize int

	// CompressedSize is the size after gzip compression.
	CompressedSize int
}

// Ratio returns the compression ratio (0.0–1.0). Lower is better.
// Returns 1.0 if original size is zero.
func (r CompressResult) Ratio() float64 {
	if r.OriginalSize == 0 {
		return 1.0
	}
	return float64(r.CompressedSize) / float64(r.OriginalSize)
}

// CompressBatch serializes an AggregatedBatch to JSON and compresses it with
// gzip at the default compression level. Returns the compressed bytes and
// size metadata.
func CompressBatch(batch *aggregator.AggregatedBatch) (CompressResult, error) {
	return CompressBatchLevel(batch, gzip.DefaultCompression)
}

// CompressBatchLevel is like CompressBatch but accepts a custom gzip
// compression level (gzip.NoCompression through gzip.BestCompression, or
// gzip.DefaultCompression).
func CompressBatchLevel(batch *aggregator.AggregatedBatch, level int) (CompressResult, error) {
	data, err := json.Marshal(batch)
	if err != nil {
		return CompressResult{}, fmt.Errorf("marshal batch: %w", err)
	}

	compressed, err := gzipBytes(data, level)
	if err != nil {
		return CompressResult{}, fmt.Errorf("gzip batch: %w", err)
	}

	return CompressResult{
		Data:           compressed,
		OriginalSize:   len(data),
		CompressedSize: len(compressed),
	}, nil
}

// CompressJSON gzip-compresses an already-serialized JSON byte slice.
// Useful for non-batch payloads (error batches, schema snapshots, etc.).
func CompressJSON(data []byte) ([]byte, error) {
	return gzipBytes(data, gzip.DefaultCompression)
}

// gzipBytes compresses raw bytes at the given gzip level.
func gzipBytes(data []byte, level int) ([]byte, error) {
	var buf bytes.Buffer
	// Pre-size the buffer — compressed output is usually smaller, but
	// we allocate roughly half to avoid early resizing.
	buf.Grow(len(data) / 2)

	gz, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, fmt.Errorf("gzip writer: %w", err)
	}
	if _, err := gz.Write(data); err != nil {
		return nil, fmt.Errorf("gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}

	return buf.Bytes(), nil
}
