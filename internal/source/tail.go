package source

import (
	"io"
	"os"
	"strings"

	"github.com/jinguo998/claude-sessions/internal/domain"
)

const adaptiveTailMaxBytes int64 = 16 * 1024 * 1024

type AppendLineTurnsFunc func([]domain.ConversationTurn, []byte, bool) []domain.ConversationTurn

func ParseTailMessages(path string, verbose bool, maxMessages int, initialBytes int64, appendLineMessages AppendLineTurnsFunc) ([]domain.ConversationTurn, error) {
	if maxMessages <= 0 {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, nil
	}
	readSize := initialBytes
	if readSize <= 0 {
		readSize = 1024 * 1024
	}
	if readSize > size {
		readSize = size
	}
	maxRead := adaptiveTailMaxBytes
	if maxRead < readSize {
		maxRead = readSize
	}
	if maxRead > size {
		maxRead = size
	}

	for {
		lines, err := tailJSONLines(path, readSize)
		if err != nil {
			return nil, err
		}
		var messages []domain.ConversationTurn
		for _, line := range lines {
			messages = appendLineMessages(messages, []byte(line), verbose)
		}
		if len(messages) >= maxMessages || readSize >= size || readSize >= maxRead {
			return trimMessages(messages, maxMessages), nil
		}
		nextReadSize := readSize * 2
		if nextReadSize < readSize || nextReadSize > maxRead {
			nextReadSize = maxRead
		}
		if nextReadSize > size {
			nextReadSize = size
		}
		if nextReadSize == readSize {
			return trimMessages(messages, maxMessages), nil
		}
		readSize = nextReadSize
	}
}

func tailJSONLines(path string, maxBytes int64) ([]string, error) {
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	readSize := maxBytes
	if size < readSize {
		readSize = size
	}
	offset := size - readSize
	buf := make([]byte, readSize)
	if _, err := f.ReadAt(buf, offset); err != nil && err != io.EOF {
		return nil, err
	}
	if offset > 0 {
		if idx := strings.IndexByte(string(buf), '\n'); idx >= 0 {
			buf = buf[idx+1:]
		} else {
			return nil, nil
		}
	}
	text := strings.TrimRight(string(buf), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func trimMessages(messages []domain.ConversationTurn, maxMessages int) []domain.ConversationTurn {
	if maxMessages <= 0 || len(messages) <= maxMessages {
		return messages
	}
	return messages[len(messages)-maxMessages:]
}
