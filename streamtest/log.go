package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Result struct {
	Timestamp time.Time `json:"ts"`
	Trigger   string    `json:"trigger"`
	Params    string    `json:"params,omitempty"`
	Status    string    `json:"status"`
	BotReply  string    `json:"bot_reply,omitempty"`
	Notes     string    `json:"notes,omitempty"`
}

type Log struct {
	f *os.File
}

func OpenLog(path string) (*Log, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Log{f: f}, nil
}

func (l *Log) Append(r Result) error {
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now().UTC()
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = l.f.Write(b)
	return err
}

func (l *Log) Close() error {
	return l.f.Close()
}

func LoadResults(path string) ([]Result, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Result
	br := bufio.NewReader(f)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 1 {
			var r Result
			if jerr := json.Unmarshal(line[:len(line)-1], &r); jerr == nil {
				out = append(out, r)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
