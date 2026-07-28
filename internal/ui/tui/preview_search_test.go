package tui

import (
	"strings"
	"testing"
)

func TestNewPreviewSearchInitialState(t *testing.T) {
	ps := newPreviewSearch()

	if ps.active {
		t.Fatal("newPreviewSearch() should start inactive")
	}
	if len(ps.hits) != 0 {
		t.Fatalf("newPreviewSearch() hits = %d, want 0", len(ps.hits))
	}
	if ps.current != 0 {
		t.Fatalf("newPreviewSearch() current = %d, want 0", ps.current)
	}
}

func TestPreviewSearchExecuteFindsCaseInsensitiveHits(t *testing.T) {
	ps := newPreviewSearch()
	ps.SetContent("alpha\nGO here\ngo again\nmiddle\nfinal go")
	ps.input.SetValue("go")

	ps.Execute()

	want := []int{1, 2, 4}
	if len(ps.hits) != len(want) {
		t.Fatalf("hits length = %d, want %d (%v)", len(ps.hits), len(want), ps.hits)
	}
	for i := range want {
		if ps.hits[i] != want[i] {
			t.Fatalf("hits = %v, want %v", ps.hits, want)
		}
	}
	if ps.current != 0 {
		t.Fatalf("current = %d, want 0", ps.current)
	}
	if ps.query != "go" {
		t.Fatalf("query = %q, want %q", ps.query, "go")
	}
}

func TestPreviewSearchNextPrevWrap(t *testing.T) {
	ps := newPreviewSearch()
	ps.SetContent("go\nx\ngo\nx\ngo")
	ps.input.SetValue("go")
	ps.Execute()

	if line := ps.Next(); line != 2 {
		t.Fatalf("first Next() = %d, want 2", line)
	}
	if line := ps.Next(); line != 4 {
		t.Fatalf("second Next() = %d, want 4", line)
	}
	if line := ps.Next(); line != 0 {
		t.Fatalf("wrap Next() = %d, want 0", line)
	}
	if line := ps.Prev(); line != 4 {
		t.Fatalf("wrap Prev() = %d, want 4", line)
	}
}

func TestPreviewSearchHighlightContent(t *testing.T) {
	ps := newPreviewSearch()
	ps.SetContent("Hello\nhello again")
	ps.input.SetValue("hello")
	ps.Execute()

	got := ps.HighlightContent()
	if stripANSI(got) != "Hello\nhello again" {
		t.Fatalf("HighlightContent stripped text = %q, want original", stripANSI(got))
	}
	if got == ps.content {
		t.Fatalf("HighlightContent() did not apply styling")
	}
	if count := strings.Count(strings.ToLower(stripANSI(got)), "hello"); count != 2 {
		t.Fatalf("HighlightContent() preserved %d matches, want 2", count)
	}
}

func TestPreviewSearchStatusText(t *testing.T) {
	ps := newPreviewSearch()
	if got := ps.StatusText(); got != "" {
		t.Fatalf("StatusText() without query = %q, want empty", got)
	}

	ps.SetContent("go\nx\ngo")
	ps.input.SetValue("go")
	ps.Execute()
	if got := ps.StatusText(); got != "1/2" {
		t.Fatalf("StatusText() = %q, want %q", got, "1/2")
	}
}

func TestPreviewSearchOpenClose(t *testing.T) {
	ps := newPreviewSearch()
	ps.SetContent("go")
	ps.input.SetValue("go")
	ps.Execute()

	ps.Open()
	if !ps.active {
		t.Fatal("Open() should activate search")
	}
	if ps.input.Value() != "" {
		t.Fatalf("Open() should clear input, got %q", ps.input.Value())
	}

	ps.Close()
	if ps.active {
		t.Fatal("Close() should deactivate search")
	}
	if ps.query != "" {
		t.Fatalf("Close() should clear query, got %q", ps.query)
	}
	if len(ps.hits) != 0 {
		t.Fatalf("Close() should clear hits, got %v", ps.hits)
	}
	if ps.current != 0 {
		t.Fatalf("Close() should reset current, got %d", ps.current)
	}
}
