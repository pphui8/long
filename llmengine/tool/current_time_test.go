package tool

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCurrentTimeToolCallUsesTimezoneInput(t *testing.T) {
	fixed := time.Date(2026, 5, 27, 12, 30, 0, 0, time.UTC)
	tool := &CurrentTimeTool{now: func() time.Time { return fixed }}

	result, err := tool.Call(context.Background(), "Asia/Tokyo")
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}

	if !strings.Contains(result, "Current time: 2026-05-27T21:30:00+09:00") {
		t.Fatalf("result = %q, want Tokyo time", result)
	}
	if !strings.Contains(result, "Timezone: Asia/Tokyo") {
		t.Fatalf("result = %q, want timezone", result)
	}
}

func TestCurrentTimeToolAllowsEmptyInput(t *testing.T) {
	tool := NewCurrentTimeTool()

	if _, err := tool.Call(context.Background(), ""); err != nil {
		t.Fatalf("Call with empty input returned error: %v", err)
	}
}

func TestCurrentTimeToolDefaultsEmptyJSONObjectInput(t *testing.T) {
	fixed := time.Date(2026, 5, 27, 12, 30, 0, 0, time.UTC)
	tool := &CurrentTimeTool{now: func() time.Time { return fixed }}

	result, err := tool.Call(context.Background(), "{}")
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}

	if !strings.Contains(result, "Current time: 2026-05-27T21:30:00+09:00") {
		t.Fatalf("result = %q, want default Tokyo time", result)
	}
	if !strings.Contains(result, "Timezone: Asia/Tokyo") {
		t.Fatalf("result = %q, want default timezone", result)
	}
}

func TestCurrentTimeToolAcceptsJSONTimezoneInput(t *testing.T) {
	fixed := time.Date(2026, 5, 27, 12, 30, 0, 0, time.UTC)
	tool := &CurrentTimeTool{now: func() time.Time { return fixed }}

	result, err := tool.Call(context.Background(), `{"timezone":"UTC"}`)
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}

	if !strings.Contains(result, "Current time: 2026-05-27T12:30:00Z") {
		t.Fatalf("result = %q, want UTC time", result)
	}
	if !strings.Contains(result, "Timezone: UTC") {
		t.Fatalf("result = %q, want UTC timezone", result)
	}
}
