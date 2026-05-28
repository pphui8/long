package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
)

const defaultTimezone = "Asia/Tokyo"

type CurrentTimeTool struct {
	now func() time.Time
}

func NewCurrentTimeTool() *CurrentTimeTool {
	return &CurrentTimeTool{now: time.Now}
}

func (t *CurrentTimeTool) Name() string {
	return "current_time"
}

func (t *CurrentTimeTool) Description() string {
	return "Returns the current date and time. Input should be an IANA timezone such as Asia/Tokyo or UTC; defaults to Asia/Tokyo if omitted."
}

func (t *CurrentTimeTool) Call(ctx context.Context, input string) (string, error) {
	_ = ctx

	location := time.Local
	timezone := normalizeTimezoneInput(input)
	if timezone != "" && !strings.EqualFold(timezone, "local") && !strings.EqualFold(timezone, "now") {
		loaded, err := time.LoadLocation(timezone)
		if err != nil {
			return "", fmt.Errorf("invalid timezone %q: %w", timezone, err)
		}
		location = loaded
	}

	current := t.now().In(location)
	return fmt.Sprintf("Current time: %s\nTimezone: %s", current.Format(time.RFC3339), current.Location()), nil
}

func normalizeTimezoneInput(input string) string {
	timezone := strings.TrimSpace(input)
	if timezone == "" || timezone == "{}" || strings.EqualFold(timezone, "null") {
		return defaultTimezone
	}

	var quoted string
	if err := json.Unmarshal([]byte(timezone), &quoted); err == nil {
		timezone = strings.TrimSpace(quoted)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(input), &payload); err == nil {
		for _, key := range []string{"timezone", "time_zone", "tz", "location"} {
			if value := strings.TrimSpace(payload[key]); value != "" {
				timezone = value
				break
			}
		}
	}

	switch {
	case timezone == "":
		return defaultTimezone
	case strings.EqualFold(timezone, "tokyo"), strings.EqualFold(timezone, "japan"), strings.EqualFold(timezone, "jst"):
		return "Asia/Tokyo"
	default:
		return timezone
	}
}
