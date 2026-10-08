package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestIfMatchVersion(t *testing.T) {
	tests := []struct {
		header string
		want   int
		ok     bool
	}{
		{`"3"`, 3, true},
		{`W/"12"`, 12, true},
		{` "7" `, 7, true},
		{"", 0, false},
		{"3", 0, false},
		{`"abc"`, 0, false},
		{`"0"`, 0, false},
		{`*`, 0, false},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("PUT", "/", nil)
		if tt.header != "" {
			req.Header.Set("If-Match", tt.header)
		}
		got, ok := IfMatchVersion(req)
		if got != tt.want || ok != tt.ok {
			t.Errorf("If-Match %q = %d, %v; want %d, %v", tt.header, got, ok, tt.want, tt.ok)
		}
	}
	if ETag(5) != `"5"` {
		t.Errorf("ETag(5) = %s", ETag(5))
	}
}
