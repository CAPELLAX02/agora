package notify

import (
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	tr, en := FormatTime(time.Date(2026, time.October, 21, 20, 59, 0, 0, time.UTC))
	if tr != "21 Ekim 2026 23:59" || en != "21 October 2026, 23:59" {
		t.Errorf("FormatTime = %q, %q", tr, en)
	}
}
