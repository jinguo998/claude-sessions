package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type dbRunner interface {
	DBPath(context.Context) (string, error)
	Query(context.Context, string, any) error
}

type cliRunner struct{}

func (cliRunner) DBPath(ctx context.Context) (string, error) {
	_ = ctx
	return resolveDBPath()
}

func (cliRunner) Query(ctx context.Context, sql string, dest any) error {
	dbPath, err := resolveDBPath()
	if err != nil {
		return err
	}
	return queryWithSQLite(ctx, dbPath, sql, dest)
}

func queryWithSQLite(ctx context.Context, dbPath, sql string, dest any) error {
	cmd := exec.CommandContext(ctx, "sqlite3", "-readonly", "-cmd", ".timeout 5000", "-json", dbPath, sql)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("sqlite3 query: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := json.Unmarshal(out, dest); err != nil {
		return fmt.Errorf("decode sqlite3 query output: %w", err)
	}
	return nil
}

func resolveDBPath() (string, error) {
	dataDir, err := opencodeDataDir()
	if err != nil {
		return "", err
	}

	if env := strings.TrimSpace(os.Getenv("OPENCODE_DB")); env != "" {
		switch {
		case env == ":memory:":
			return "", fmt.Errorf("OPENCODE_DB=:memory: cannot be scanned")
		case filepath.IsAbs(env):
			return existingDBPath(env)
		default:
			return existingDBPath(filepath.Join(dataDir, env))
		}
	}

	candidates := []string{filepath.Join(dataDir, "opencode.db")}
	channelMatches, _ := filepath.Glob(filepath.Join(dataDir, "opencode-*.db"))
	sort.Strings(channelMatches)
	candidates = append(candidates, channelMatches...)
	for _, candidate := range candidates {
		if path, err := existingDBPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("opencode database not found under %s", dataDir)
}

func existingDBPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("opencode database path is a directory: %s", path)
	}
	return path, nil
}

func opencodeDataDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("OPENCODE_DATA_DIR")); dir != "" {
		return dir, nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "opencode"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "opencode"), nil
}

func sqlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
