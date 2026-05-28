package tool

import (
	"context"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
)

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
	return "Returns the current date and time. Optional input may be an IANA timezone such as Asia/Tokyo or UTC."
}

func (t *CurrentTimeTool) Call(ctx context.Context, input string) (string, error) {
	_ = ctx

	location := time.Local
	timezone := strings.TrimSpace(input)
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
