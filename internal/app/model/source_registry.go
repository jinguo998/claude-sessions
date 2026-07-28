package model

import (
	"strings"

	"github.com/jinguo998/claude-sessions/internal/domain"
)

type PermissionMode = domain.PermissionMode

const (
	PermissionModeSafe = domain.PermissionModeSafe
	PermissionModeFast = domain.PermissionModeFast
)

// SourceInfo describes source presentation metadata and user-facing action
// capabilities. It deliberately lives outside domain so domain remains pure.
type SourceInfo struct {
	Source                   Source
	Label                    string
	Badge                    string
	LightColor               string
	DarkColor                string
	DefaultPermissionMode    PermissionMode
	SupportsSafeResumeAction bool
	SupportsFork             bool
	SupportsArchive          bool
}

type SourceRegistry struct {
	ordered  []SourceInfo
	bySource map[Source]SourceInfo
}

func DefaultSourceRegistry() SourceRegistry {
	return NewSourceRegistry([]SourceInfo{
		{
			Source:                   SourceClaude,
			Label:                    "Claude",
			Badge:                    "C",
			LightColor:               "27",
			DarkColor:                "75",
			DefaultPermissionMode:    PermissionModeFast,
			SupportsSafeResumeAction: true,
			SupportsFork:             true,
			SupportsArchive:          true,
		},
		{
			Source:                   SourceCodex,
			Label:                    "Codex",
			Badge:                    "X",
			LightColor:               "22",
			DarkColor:                "40",
			DefaultPermissionMode:    PermissionModeSafe,
			SupportsSafeResumeAction: false,
			SupportsFork:             true,
			SupportsArchive:          true,
		},
		{
			Source:                   SourceOpenCode,
			Label:                    "OpenCode",
			Badge:                    "O",
			LightColor:               "88",
			DarkColor:                "204",
			DefaultPermissionMode:    PermissionModeSafe,
			SupportsSafeResumeAction: false,
			SupportsFork:             true,
			SupportsArchive:          false,
		},
	})
}

func NewSourceRegistry(infos []SourceInfo) SourceRegistry {
	r := SourceRegistry{
		ordered:  make([]SourceInfo, 0, len(infos)),
		bySource: make(map[Source]SourceInfo, len(infos)),
	}
	for _, info := range infos {
		info = normalizeSourceInfo(info)
		r.ordered = append(r.ordered, info)
		r.bySource[info.Source] = info
	}
	return r
}

func (r SourceRegistry) All() []SourceInfo {
	r = r.withDefault()
	out := make([]SourceInfo, len(r.ordered))
	copy(out, r.ordered)
	return out
}

func (r SourceRegistry) Info(source Source) SourceInfo {
	r = r.withDefault()
	if info, ok := r.bySource[source]; ok {
		return info
	}
	return fallbackSourceInfo(source)
}

func (r SourceRegistry) DefaultPermissionMode(source Source) PermissionMode {
	return r.Info(source).DefaultPermissionMode
}

func (r SourceRegistry) SupportsSafeResumeAction(source Source) bool {
	return r.Info(source).SupportsSafeResumeAction
}

func (r SourceRegistry) SupportsFork(source Source) bool {
	return r.Info(source).SupportsFork
}

func (r SourceRegistry) SupportsArchive(source Source) bool {
	return r.Info(source).SupportsArchive
}

func (r SourceRegistry) withDefault() SourceRegistry {
	if len(r.ordered) > 0 || len(r.bySource) > 0 {
		return r
	}
	return DefaultSourceRegistry()
}

func normalizeSourceInfo(info SourceInfo) SourceInfo {
	fallback := fallbackSourceInfo(info.Source)
	if strings.TrimSpace(info.Label) == "" {
		info.Label = fallback.Label
	}
	if strings.TrimSpace(info.Badge) == "" {
		info.Badge = fallback.Badge
	}
	if strings.TrimSpace(info.LightColor) == "" {
		info.LightColor = fallback.LightColor
	}
	if strings.TrimSpace(info.DarkColor) == "" {
		info.DarkColor = fallback.DarkColor
	}
	if info.DefaultPermissionMode == "" {
		info.DefaultPermissionMode = PermissionModeSafe
	}
	return info
}

func fallbackSourceInfo(source Source) SourceInfo {
	label := string(source)
	if strings.TrimSpace(label) == "" {
		label = "unknown"
	}
	return SourceInfo{
		Source:                source,
		Label:                 label,
		Badge:                 "?",
		LightColor:            "242",
		DarkColor:             "241",
		DefaultPermissionMode: PermissionModeSafe,
	}
}
