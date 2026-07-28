package query

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jinguo998/claude-sessions/internal/app/model"
	"github.com/jinguo998/claude-sessions/internal/domain"
)

type IndexedSession struct {
	Session    model.Session
	SearchText string
}

type Filter struct {
	Query       string
	ProjectPath string
	Source      domain.Source
}

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Index(sessions []model.Session) []IndexedSession {
	indexed := make([]IndexedSession, len(sessions))
	for i, sess := range sessions {
		indexed[i] = IndexedSession{Session: sess, SearchText: buildCorpus(sess)}
	}
	return indexed
}

func (s *Service) Filter(sessions []model.Session, filter Filter) []model.Session {
	indexed := s.Search(sessions, filter)
	var out []model.Session
	for _, item := range indexed {
		out = append(out, item.Session)
	}
	return out
}

func (s *Service) Search(sessions []model.Session, filter Filter) []IndexedSession {
	indexed := s.Index(sessions)
	tokens := Tokens(filter.Query)
	var out []IndexedSession
	for _, item := range indexed {
		sess := item.Session
		if filter.ProjectPath != "" && sess.ProjectPath != filter.ProjectPath {
			continue
		}
		if filter.Source != "" && sess.Source != filter.Source {
			continue
		}
		if len(tokens) > 0 && !MatchAllTokens(tokens, item.SearchText) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func SortRecent(sessions []model.Session) {
	sort.Sort(model.SortByLastTime(sessions))
}

func Tokens(query string) []string {
	return strings.Fields(strings.ToLower(query))
}

func MatchAllTokens(tokens []string, text string) bool {
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			return false
		}
	}
	return true
}

func MatchedRuneIndexes(tokens []string, text string) []int {
	var allIndexes []int
	for _, token := range tokens {
		byteIdx := strings.Index(text, token)
		if byteIdx < 0 {
			continue
		}
		allIndexes = appendRuneIndexes(allIndexes, text, byteIdx, token)
	}
	return allIndexes
}

func appendRuneIndexes(dst []int, text string, byteIdx int, token string) []int {
	runeStart := utf8.RuneCountInString(text[:byteIdx])
	tokenRuneLen := utf8.RuneCountInString(token)
	for j := range tokenRuneLen {
		dst = append(dst, runeStart+j)
	}
	return dst
}

func buildCorpus(sess model.Session) string {
	parts := []string{
		sess.SearchText,
		sess.FirstMsg,
		sess.LastMsg,
		sess.Title,
		sess.ProjectPath,
		sess.ProjectShortName(),
		sess.Client,
		sess.Origin,
		sess.Model,
	}
	for _, label := range sess.Labels {
		parts = append(parts, label)
	}
	if sess.Attributes != nil {
		for k, v := range sess.Attributes {
			parts = append(parts, k, v)
		}
	}
	return strings.ToLower(strings.Join(parts, " "))
}
