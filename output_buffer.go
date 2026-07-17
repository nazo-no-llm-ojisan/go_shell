package main

import "bytes"

const defaultMaxCapturedOutputBytes = 16 << 20

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	if limit < 0 {
		limit = 0
	}
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	written := len(p)
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = b.truncated || len(p) > 0
		return written, nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.truncated = true
		return written, nil
	}
	_, _ = b.buf.Write(p)
	return written, nil
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

func (b *limitedBuffer) Truncated() bool {
	return b.truncated
}

func outputLimit(meta *metaConfig) int {
	if meta.maxOutputBytes > 0 {
		return meta.maxOutputBytes
	}
	return defaultMaxCapturedOutputBytes
}
