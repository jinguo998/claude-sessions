package model

import (
	"testing"
	"time"
)

func TestSessionFormatDuration(t *testing.T) {
	start := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		sess Session
		want string
	}{
		{name: "zero timestamps", sess: Session{}, want: "<1m"},
		{name: "under one minute", sess: Session{StartTime: start, LastTime: start.Add(30 * time.Second)}, want: "<1m"},
		{name: "minutes", sess: Session{StartTime: start, LastTime: start.Add(42 * time.Minute)}, want: "42m"},
		{name: "whole hours", sess: Session{StartTime: start, LastTime: start.Add(2 * time.Hour)}, want: "2h"},
		{name: "hours and minutes", sess: Session{StartTime: start, LastTime: start.Add(2*time.Hour + 5*time.Minute)}, want: "2h5m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sess.FormatDuration(); got != tt.want {
				t.Fatalf("FormatDuration() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionFormatSize(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{size: 999, want: "999B"},
		{size: 1024, want: "1KB"},
		{size: 1536, want: "2KB"},
		{size: 1024 * 1024, want: "1.0MB"},
		{size: 3*1024*1024 + 512*1024, want: "3.5MB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := (Session{FileSize: tt.size}).FormatSize(); got != tt.want {
				t.Fatalf("FormatSize() = %q, want %q", got, tt.want)
			}
		})
	}
}
