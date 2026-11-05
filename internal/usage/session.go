package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"math"
	"os"
	"time"
)

type tally struct {
	tokens uint64
	events uint64
}

func (t *tally) add(other tally) {
	t.tokens = saturatingAdd(t.tokens, other.tokens)
	t.events = saturatingAdd(t.events, other.events)
}

func saturatingAdd(a, b uint64) uint64 {
	if math.MaxUint64-a < b {
		return math.MaxUint64
	}
	return a + b
}

type tokenUsage struct {
	Total *uint64 `json:"total_tokens"`
}

type tokenEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Payload   struct {
		Type string `json:"type"`
		Info struct {
			Cumulative tokenUsage `json:"total_token_usage"`
			Last       tokenUsage `json:"last_token_usage"`
		} `json:"info"`
	} `json:"payload"`
}

type counter struct {
	previous    uint64
	hasPrevious bool
	tally       tally
}

func (c *counter) count(line []byte, start, end time.Time) {
	if !bytes.Contains(line, []byte(`"token_count"`)) {
		return
	}
	var event tokenEvent
	if err := json.Unmarshal(line, &event); err != nil || event.Timestamp.IsZero() ||
		event.Type != "event_msg" || event.Payload.Type != "token_count" {
		return
	}

	var increment uint64
	info := event.Payload.Info
	if info.Cumulative.Total != nil {
		total := *info.Cumulative.Total
		increment = total
		if c.hasPrevious && total >= c.previous {
			increment -= c.previous
		}
		c.previous, c.hasPrevious = total, true
	} else if info.Last.Total != nil {
		increment = *info.Last.Total
	}

	if increment > 0 && !event.Timestamp.Before(start) && event.Timestamp.Before(end) {
		c.tally.add(tally{tokens: increment, events: 1})
	}
}

type checkpoint struct {
	offset int64
	count  counter
}

type session struct {
	info    fs.FileInfo
	tail    []byte
	cursor  checkpoint
	visible tally
	retry   bool
}

func scanFile(path string, previous session, start, end time.Time) (session, error) {
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

	reader := bufio.NewReader(io.NewSectionReader(file, cursor.offset, size-cursor.offset))
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			if cursor.offset+int64(len(line)) != size {
				return session{}, io.ErrUnexpectedEOF
			}
			// A valid line without a newline is visible immediately. Keep its
			// start as the checkpoint so the next append can finish that line.
			visible := cursor.count
			visible.count(line, start, end)
			return session{info: info, tail: tail, cursor: cursor, visible: visible.tally}, nil
		}
		if err != nil {
			return session{}, err
		}
		cursor.count.count(line, start, end)
		cursor.offset += int64(len(line))
	}
}

func readTail(file *os.File, end int64) ([]byte, error) {
	data := make([]byte, min(end, 128))
	_, err := file.ReadAt(data, end-int64(len(data)))
	return data, err
}
