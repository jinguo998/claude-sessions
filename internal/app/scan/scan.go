package scan

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jinguo998/claude-sessions/internal/app/model"
	"github.com/jinguo998/claude-sessions/internal/source"
	"github.com/jinguo998/claude-sessions/internal/storage"
)

type WarningKind string

const (
	WarningSourceUnavailable WarningKind = "source_unavailable"
	WarningPartialScan       WarningKind = "partial_scan"
	WarningCache             WarningKind = "cache"
	WarningParse             WarningKind = "parse_warning"
	WarningArchiveConflict   WarningKind = "archive_restore_conflict"
)

type Warning struct {
	Kind    WarningKind
	Source  string
	Message string
	Err     error
}

type Result struct {
	Sessions []model.Session
	Warnings []Warning
	Err      error
}

type Repository struct {
	scanners []source.Scanner
	cache    storage.MetadataCache
}

type sourceResult struct {
	sessions []model.Session
	warnings []Warning
}

func NewRepository(scanners []source.Scanner, cache storage.MetadataCache) *Repository {
	return &Repository{scanners: scanners, cache: cache}
}

func (r *Repository) Scan(ctx context.Context) Result {
	ch := make(chan sourceResult, len(r.scanners))
	for _, scanner := range r.scanners {
		scanner := scanner
		go func() {
			ch <- r.scanSource(ctx, scanner)
		}()
	}

	var result Result
	for range r.scanners {
		sr := <-ch
		result.Sessions = append(result.Sessions, sr.sessions...)
		result.Warnings = append(result.Warnings, sr.warnings...)
	}
	if r.cache != nil {
		r.cache.Save()
	}
	return result
}

func (r *Repository) scanSource(ctx context.Context, scanner source.Scanner) sourceResult {
	candidates, err := scanner.Discover(ctx)
	if err != nil {
		warning := Warning{
			Kind:    WarningSourceUnavailable,
			Source:  string(scanner.Source()),
			Message: err.Error(),
			Err:     err,
		}
		if len(candidates) == 0 {
			return sourceResult{warnings: []Warning{warning}}
		}
	}
	var discoverWarning *Warning
	if err != nil && len(candidates) > 0 {
		discoverWarning = &Warning{
			Kind:    WarningPartialScan,
			Source:  string(scanner.Source()),
			Message: err.Error(),
			Err:     err,
		}
	}

	type itemResult struct {
		session model.Session
		warning *Warning
	}
	const maxWorkers = 16
	var wg sync.WaitGroup
	ch := make(chan itemResult, 100)
	sem := make(chan struct{}, maxWorkers)
	var cachedSessions []model.Session
	for _, candidate := range candidates {
		candidate := candidate
		if cached, ok := r.cacheGet(candidate); ok {
			if cached.Session.MsgCount > 0 {
				cachedSessions = append(cachedSessions, model.FromDomain(cached.Session, cached.SearchText))
			}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			scanned, err := scanner.ScanFile(ctx, candidate)
			if err != nil {
				ch <- itemResult{warning: &Warning{
					Kind:    WarningPartialScan,
					Source:  string(scanner.Source()),
					Message: fmt.Sprintf("%s: %v", candidate.Path, err),
					Err:     err,
				}}
				return
			}
			searchText := buildSearchText(scanned.SearchParts)
			cached := storage.CachedSession{
				Session:     scanned.Session,
				SearchText:  searchText,
				MetadataKey: candidate.MetadataKey,
			}
			if r.cache != nil {
				r.cache.Put(candidate.Path, candidate.FileSize, candidate.ModTime, cached)
			}
			if scanned.Session.MsgCount > 0 {
				ch <- itemResult{session: model.FromDomain(scanned.Session, searchText)}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	var result sourceResult
	if discoverWarning != nil {
		result.warnings = append(result.warnings, *discoverWarning)
	}
	result.sessions = append(result.sessions, cachedSessions...)
	for item := range ch {
		if item.warning != nil {
			result.warnings = append(result.warnings, *item.warning)
			continue
		}
		result.sessions = append(result.sessions, item.session)
	}
	return result
}

func (r *Repository) cacheGet(candidate source.Candidate) (storage.CachedSession, bool) {
	if r.cache == nil {
		return storage.CachedSession{}, false
	}
	cached, ok := r.cache.Get(candidate.Path, candidate.FileSize, candidate.ModTime)
	if !ok || cached.MetadataKey != candidate.MetadataKey {
		return storage.CachedSession{}, false
	}
	return cached, true
}

func buildSearchText(parts []string) string {
	var cleaned []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			cleaned = append(cleaned, strings.Join(strings.Fields(part), " "))
		}
	}
	return strings.ToLower(strings.Join(cleaned, " "))
}
