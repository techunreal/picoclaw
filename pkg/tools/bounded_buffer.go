package tools

import "bytes"

const maxForegroundOutputBytes = 1 * 1024 * 1024 // 1 MB per stream

// boundedBuffer keeps at most limit bytes in memory. It is used for foreground
// exec output so a chatty command cannot grow the process without bound.
type boundedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newBoundedBuffer(limit int) *boundedBuffer {
	if limit <= 0 {
		limit = maxForegroundOutputBytes
	}
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	written := len(p)
	if b.buf.Len() < b.limit {
		remaining := b.limit - b.buf.Len()
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.buf.Write(p)
	}
	if b.buf.Len() >= b.limit {
		b.truncated = true
	}
	return written, nil
}

func (b *boundedBuffer) Len() int {
	return b.buf.Len()
}

func (b *boundedBuffer) String() string {
	output := b.buf.String()
	if b.truncated {
		output += outputTruncateMarker
	}
	return output
}
