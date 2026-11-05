package usage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var beijing = time.FixedZone("UTC+8", 8*60*60)

type Status string

const (
	OK               Status = "ok"
	NoData           Status = "no_data"
	MissingDirectory Status = "missing_directory"
	PartialError     Status = "partial_error"
)

type Snapshot struct {
	Date           string  `json:"date"`
	TotalTokens    string  `json:"totalTokens"`
	FormattedTotal string  `json:"formattedTotal"`
	FileCount      int     `json:"fileCount"`
	EventCount     uint64  `json:"eventCount"`
	UpdatedAt      string  `json:"updatedAt"`
	Status         Status  `json:"status"`
	Message        *string `json:"message"`
}

func SessionsDir() string {
	if home, ok := os.LookupEnv("CODEX_HOME"); ok {
		return filepath.Join(home, "sessions")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "sessions")
}

type Index struct {
	root  string
	day   time.Time
	files map[string]session
}

func NewIndex(root string) *Index {
	return &Index{root: root}
}

// Scan updates the index and returns a snapshot. A single goroutine owns the index.
func (i *Index) Scan(now time.Time, rebuild bool) Snapshot {
	now = now.In(beijing)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, beijing)
	dayEnd := dayStart.AddDate(0, 0, 1)
	if !i.day.Equal(dayStart) {
		i.day, i.files = dayStart, nil
	}

	snapshot := Snapshot{Date: now.Format(time.DateOnly), UpdatedAt: now.Format(time.TimeOnly), Status: NoData}
	files := make(map[string]session, len(i.files))
	var failed int
	var unreadableDirs []string
	fs.WalkDir(os.DirFS(i.root), ".", func(relative string, entry fs.DirEntry, walkErr error) error {
		if relative == "." && (errors.Is(walkErr, fs.ErrNotExist) || (walkErr == nil && !entry.IsDir())) {
			message := fmt.Sprintf("未找到 Codex 会话目录：%s", i.root)
			snapshot.Status, snapshot.Message = MissingDirectory, &message
			return fs.SkipAll
		}
		if walkErr != nil {
			if !errors.Is(walkErr, fs.ErrNotExist) {
				failed++
				unreadableDirs = append(unreadableDirs, relative)
			}
			return nil
		}
		if !entry.Type().IsRegular() || filepath.Ext(relative) != ".jsonl" {
			return nil
		}
		previous := i.files[relative]
		info, err := entry.Info()
		if err == nil {
			if info.ModTime().Before(dayStart) {
				return nil
			}
			path := filepath.Join(i.root, filepath.FromSlash(relative))
			var updated session
			updated, err = updateSession(path, info, previous, rebuild, dayStart, dayEnd)
			if err == nil {
				files[relative] = updated
				return nil
			}
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		failed++
		previous.retry = true
		files[relative] = previous
		return nil
	})
	// Keep the last known contributions under a directory we could not inspect.
	for _, directory := range unreadableDirs {
		for path, previous := range i.files {
			if _, seen := files[path]; !seen && (directory == "." || strings.HasPrefix(path, directory+"/")) {
				previous.retry = previous.retry || rebuild
				files[path] = previous
			}
		}
	}
	i.files = files

	var usage tally
	for _, file := range files {
		usage.add(file.visible)
	}
	snapshot.FileCount = len(files)
	snapshot.TotalTokens = strconv.FormatUint(usage.tokens, 10)
	snapshot.FormattedTotal = groupDigits(snapshot.TotalTokens)
	snapshot.EventCount = usage.events
	switch {
	case failed > 0:
		message := fmt.Sprintf("%d 处日志暂时无法读取，将在下次刷新时重试", failed)
		snapshot.Status, snapshot.Message = PartialError, &message
	case usage.events > 0:
		snapshot.Status = OK
	}
	return snapshot
}

func updateSession(path string, info fs.FileInfo, previous session, rebuild bool, start, end time.Time) (session, error) {
	if rebuild || previous.retry {
		previous = session{}
	}
	if previous.info != nil && info.Size() == previous.info.Size() &&
		info.ModTime().Equal(previous.info.ModTime()) && os.SameFile(previous.info, info) {
		return previous, nil
	}
	return scanFile(path, previous, start, end)
}

func groupDigits(digits string) string {
	var result strings.Builder
	for index, digit := range digits {
		if index > 0 && (len(digits)-index)%4 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(digit)
	}
	return result.String()
}
