package usage

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type offsetReader struct {
	io.ReaderAt
	start int64
	t     *testing.T
}

func (r offsetReader) ReadAt(p []byte, off int64) (int, error) {
	r.t.Helper()
	if off < r.start {
		r.t.Fatalf("read old log content at %d, checkpoint is %d", off, r.start)
	}
	return r.ReaderAt.ReadAt(p, off)
}

func TestReadRecordsResumesFromCheckpoint(t *testing.T) {
	initial := `{"timestamp":"2026-11-05T00:01:00+08:00","type":"token_usage_record","payload":{"response_id":"first","usage":{"total_tokens":10}}}` + "\n"
	appended := initial + strings.Replace(initial, "first", "second", 1)
	cursor, _, err := readRecords(strings.NewReader(initial), int64(len(initial)), checkpoint{})
	if err != nil {
		t.Fatal(err)
	}
	reader := offsetReader{ReaderAt: strings.NewReader(appended), start: cursor.offset, t: t}
	_, summary, err := readRecords(reader, int64(len(appended)), cursor)
	if err != nil || len(summary.responses) != 2 {
		t.Fatalf("expected both responses after appending, got %v, %v", summary, err)
	}
}

func TestUnchangedFileReusesSummary(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := scanFile(path, session{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	reused, err := updateSession(path, info, file, false)
	if err != nil || reused.summary != file.summary {
		t.Fatalf("unchanged metadata must reuse the previous summary: %v", err)
	}
}
