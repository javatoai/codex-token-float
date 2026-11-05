package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"time"
)

const (
	sessionMeta   = "session_meta"
	responseUsage = "token_usage_record"
	tokenCount    = "token_count"
)

type logRecord struct {
	kind       string
	at         time.Time
	threadID   string
	forked     bool
	responseID string
	usage      tokenUsage
}

func (r logRecord) day() string {
	return r.at.In(beijing).Format(time.DateOnly)
}

func parseRecord(line []byte) (logRecord, bool) {
	if !bytes.Contains(line, []byte(`"session_meta"`)) &&
		!bytes.Contains(line, []byte(`"token_usage_record"`)) &&
		!bytes.Contains(line, []byte(`"token_count"`)) {
		return logRecord{}, false
	}
	var entry struct {
		Type      string    `json:"type"`
		Timestamp time.Time `json:"timestamp"`
		Payload   struct {
			Type       string     `json:"type"`
			ID         string     `json:"id"`
			ForkedFrom string     `json:"forked_from_id"`
			ThreadID   string     `json:"thread_id"`
			ResponseID string     `json:"response_id"`
			Usage      tokenUsage `json:"usage"`
			Info       *struct {
				Cumulative tokenUsage `json:"total_token_usage"`
			} `json:"info"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(line, &entry); err != nil {
		return logRecord{}, false
	}
	p := entry.Payload
	if entry.Type == sessionMeta {
		return logRecord{kind: sessionMeta, threadID: p.ID, forked: p.ForkedFrom != ""}, true
	}
	if entry.Timestamp.IsZero() {
		return logRecord{}, false
	}
	switch {
	case entry.Type == responseUsage:
		return logRecord{kind: responseUsage, at: entry.Timestamp, threadID: p.ThreadID, responseID: p.ResponseID, usage: p.Usage}, true
	case entry.Type == "event_msg" && p.Type == tokenCount && p.Info != nil:
		return logRecord{kind: tokenCount, at: entry.Timestamp, usage: p.Info.Cumulative}, true
	default:
		return logRecord{}, false
	}
}

type checkpoint struct {
	offset  int64
	records []logRecord
}

type session struct {
	info    fs.FileInfo
	tail    []byte
	cursor  checkpoint
	summary *fileSummary
	retry   bool
}

func scanFile(path string, previous session) (session, error) {
	file, err := os.Open(path)
	if err != nil {
		return session{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return session{}, err
	}
	if !info.Mode().IsRegular() {
		return session{}, fs.ErrInvalid
	}
	if sameFileVersion(previous.info, info) {
		return previous, nil
	}
	size := info.Size()
	var cursor checkpoint
	if previous.info != nil && size > previous.info.Size() && os.SameFile(previous.info, info) {
		// A truncate-and-rewrite can grow past the last observed size.
		oldTail, err := readTail(file, previous.info.Size())
		if err != nil {
			return session{}, err
		}
		if bytes.Equal(oldTail, previous.tail) {
			cursor = previous.cursor
		}
	}
	tail, err := readTail(file, size)
	if err != nil {
		return session{}, err
	}
	cursor, summary, err := readRecords(file, size, cursor)
	if err != nil {
		return session{}, err
	}
	return session{info: info, tail: tail, cursor: cursor, summary: summary}, nil
}

func readRecords(file io.ReaderAt, size int64, cursor checkpoint) (checkpoint, *fileSummary, error) {
	reader := bufio.NewReader(io.NewSectionReader(file, cursor.offset, size-cursor.offset))
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			if cursor.offset+int64(len(line)) != size {
				return checkpoint{}, nil, io.ErrUnexpectedEOF
			}
			// Show a valid final line immediately, but keep its start as the
			// checkpoint. The next append can complete or replace that line.
			visible := cursor.records
			if record, ok := parseRecord(line); ok {
				visible = append(visible[:len(visible):len(visible)], record)
			}
			return cursor, summarizeSession(visible), nil
		}
		if err != nil {
			return checkpoint{}, nil, err
		}
		if record, ok := parseRecord(line); ok {
			cursor.records = append(cursor.records, record)
		}
		cursor.offset += int64(len(line))
	}
}

func readTail(file *os.File, end int64) ([]byte, error) {
	data := make([]byte, min(end, 128))
	_, err := file.ReadAt(data, end-int64(len(data)))
	return data, err
}
