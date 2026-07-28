package trash

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/storage"
)

const archiveDirName = ".claude-sessions-trash"

type Store struct {
	home string
}

func New() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewAt(home), nil
}

func NewAt(home string) *Store {
	return &Store{home: home}
}

func (s *Store) Archive(sess domain.Session, sideDir string) (string, error) {
	return archiveSessionAt(sess, s.home, sideDir, time.Now().UTC())
}

func archiveSessionAt(sess domain.Session, home, sideDir string, now time.Time) (string, error) {
	root := filepath.Join(home, archiveDirName)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}

	destDir, err := nextArchiveDir(root, archiveDirBase(sess, now))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}

	destFile := filepath.Join(destDir, filepath.Base(sess.FilePath))
	if err := movePath(sess.FilePath, destFile); err != nil {
		_ = os.RemoveAll(destDir)
		return "", err
	}

	sideDirDest := ""
	sideMoved := false
	rollback := func() {
		if sideMoved {
			_ = movePath(sideDirDest, sideDir)
		}
		_ = movePath(destFile, sess.FilePath)
		_ = os.RemoveAll(destDir)
	}
	if info, err := os.Stat(sideDir); err == nil && info.IsDir() {
		sideDirDest = filepath.Join(destDir, filepath.Base(sideDir))
		if err := movePath(sideDir, sideDirDest); err != nil {
			rollback()
			return "", err
		}
		sideMoved = true
	}

	meta := storage.ArchivedSessionMetadata{
		ID:               sess.ID,
		Source:           sess.Source,
		ArchivedAt:       now.Format(time.RFC3339),
		OriginalFilePath: sess.FilePath,
		OriginalSideDir:  sideDir,
		ProjectPath:      sess.ProjectPath,
		Title:            sess.Title,
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		rollback()
		return "", err
	}
	if err := os.WriteFile(filepath.Join(destDir, "metadata.json"), metaBytes, 0o644); err != nil {
		rollback()
		return "", err
	}

	return destDir, nil
}

func (s *Store) List() ([]storage.ArchivedSession, error) {
	return loadArchivedSessionsFromHome(s.home)
}

func loadArchivedSessionsFromHome(home string) ([]storage.ArchivedSession, error) {
	root := filepath.Join(home, archiveDirName)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var items []storage.ArchivedSession
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		metaBytes, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
		if err != nil {
			continue
		}
		var meta storage.ArchivedSessionMetadata
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			continue
		}
		if meta.Title == "" {
			meta.Title = meta.LegacyThreadName
		}

		item := storage.ArchivedSession{ArchiveDir: dir, Metadata: meta}
		matches, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		if len(matches) > 0 {
			item.SessionFile = matches[0]
		}
		if meta.OriginalSideDir != "" {
			sideBase := filepath.Base(meta.OriginalSideDir)
			sidePath := filepath.Join(dir, sideBase)
			if info, err := os.Stat(sidePath); err == nil && info.IsDir() {
				item.SideDir = sidePath
			}
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Metadata.ArchivedAt > items[j].Metadata.ArchivedAt
	})
	return items, nil
}

func (s *Store) Restore(item storage.ArchivedSession) error {
	return restoreArchivedSession(item)
}

func restoreArchivedSession(item storage.ArchivedSession) error {
	if item.Metadata.OriginalFilePath == "" || item.SessionFile == "" {
		return fmt.Errorf("archive metadata is incomplete")
	}
	if err := os.MkdirAll(filepath.Dir(item.Metadata.OriginalFilePath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(item.Metadata.OriginalFilePath); err == nil {
		return fmt.Errorf("destination exists: %s", item.Metadata.OriginalFilePath)
	}
	if item.SideDir != "" && item.Metadata.OriginalSideDir != "" {
		if err := os.MkdirAll(filepath.Dir(item.Metadata.OriginalSideDir), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(item.Metadata.OriginalSideDir); err == nil {
			return fmt.Errorf("side dir exists: %s", item.Metadata.OriginalSideDir)
		}
	}
	if err := movePath(item.SessionFile, item.Metadata.OriginalFilePath); err != nil {
		return err
	}

	if item.SideDir != "" && item.Metadata.OriginalSideDir != "" {
		if err := movePath(item.SideDir, item.Metadata.OriginalSideDir); err != nil {
			_ = movePath(item.Metadata.OriginalFilePath, item.SessionFile)
			return err
		}
	}

	_ = os.Remove(filepath.Join(item.ArchiveDir, "metadata.json"))
	_ = os.Remove(item.ArchiveDir)
	return nil
}

func (s *Store) Delete(item storage.ArchivedSession) error {
	return deleteArchivedSession(item)
}

func deleteArchivedSession(item storage.ArchivedSession) error {
	if item.ArchiveDir == "" {
		return fmt.Errorf("archive path is empty")
	}
	return os.RemoveAll(item.ArchiveDir)
}

func (s *Store) DeleteAll(items []storage.ArchivedSession) error {
	return deleteArchivedSessions(items)
}

func deleteArchivedSessions(items []storage.ArchivedSession) error {
	var failures []string
	for _, item := range items {
		if err := deleteArchivedSession(item); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

func movePath(src, dst string) error {
	if src == "" || dst == "" {
		return fmt.Errorf("source and destination are required")
	}
	if _, err := os.Stat(src); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("destination exists: %s", dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyPath(src, dst); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	if err := os.RemoveAll(src); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	return nil
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst, info.Mode().Perm())
	}
	return copyFile(src, dst, info.Mode().Perm())
}

func copyDir(src, dst string, perm os.FileMode) error {
	if err := os.Mkdir(dst, perm); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if err := copyPath(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return closeErr
	}
	return nil
}

func archiveDirBase(sess domain.Session, now time.Time) string {
	shortID := sess.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	return fmt.Sprintf("%s-%s-%s", now.Format("20060102T150405Z"), sess.Source, shortID)
}

func nextArchiveDir(root, base string) (string, error) {
	dest := filepath.Join(root, base)
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest, nil
	}
	for i := 2; i < 1000; i++ {
		candidate := filepath.Join(root, fmt.Sprintf("%s-%d", base, i))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unable to allocate archive dir for %s", base)
}
