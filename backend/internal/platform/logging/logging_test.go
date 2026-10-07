package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewRespectsLevel(t *testing.T) {
	tests := []struct {
		level     string
		wantDebug bool
		wantInfo  bool
		wantWarn  bool
	}{
		{level: "debug", wantDebug: true, wantInfo: true, wantWarn: true},
		{level: "info", wantDebug: false, wantInfo: true, wantWarn: true},
		{level: "warn", wantDebug: false, wantInfo: false, wantWarn: true},
		{level: "error", wantDebug: false, wantInfo: false, wantWarn: false},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(&buf, "development", tt.level)

			logger.Debug("debug-mesajı")
			logger.Info("info-mesajı")
			logger.Warn("warn-mesajı")

			out := buf.String()
			checks := map[string]bool{
				"debug-mesajı": tt.wantDebug,
				"info-mesajı":  tt.wantInfo,
				"warn-mesajı":  tt.wantWarn,
			}
			for msg, want := range checks {
				if got := strings.Contains(out, msg); got != want {
					t.Errorf("seviye %q: %q görünüyor = %v, want %v", tt.level, msg, got, want)
				}
			}
		})
	}
}

func TestNewProductionWritesJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, "production", "info")

	logger.Info("merhaba", "anahtar", "değer")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("çıktı geçerli JSON değil: %v\n%s", err, buf.String())
	}
	if entry["msg"] != "merhaba" || entry["anahtar"] != "değer" {
		t.Errorf("beklenmeyen log kaydı: %v", entry)
	}
}
