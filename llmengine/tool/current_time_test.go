package tool

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCurrentTimeToolExecuteUsesTimezoneInput(t *testing.T) {
	fixed := time.Date(2026, 5, 27, 12, 30, 0, 0, time.UTC)
	tool := &CurrentTimeTool{now: func() time.Time { return fixed }}

	result, err := tool.Execute(context.Background(), "Asia/Tokyo")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
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

	if !tool.AllowsEmptyInput() {
		t.Fatal("AllowsEmptyInput = false, want true")
	}
}
