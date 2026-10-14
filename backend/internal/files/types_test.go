package files

import (
	"strings"
	"testing"
)

func TestCleanName(t *testing.T) {
	for in, want := range map[string][2]string{
		"ödev.PDF":                    {"ödev.PDF", ".pdf"},
		`C:\Users\ali\rapor.docx`:     {"rapor.docx", ".docx"},
		"../../etc/passwd.txt":        {"passwd.txt", ".txt"},
		"  boşluklu adı .txt  ":       {"boşluklu adı .txt", ".txt"},
		"kontrol\x00\x1f\"tırnak.csv": {"kontroltırnak.csv", ".csv"},
		"README":                      {"README", ""},
		"/":                           {"", ""},
	} {
		got, ext := cleanName(in)
		if got != want[0] || ext != want[1] {
			t.Errorf("cleanName(%q) = %q %q, want %q %q", in, got, ext, want[0], want[1])
		}
	}
	long, ext := cleanName(strings.Repeat("ş", 300) + ".pdf")
	if len([]rune(long)) != 255 || ext != ".pdf" {
		t.Errorf("uzun ad %d karakter, uzantı %q", len([]rune(long)), ext)
	}
}

func TestKindMatches(t *testing.T) {
	zip := []byte("PK\x03\x04\x14\x00\x06\x00")
	ole := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0}
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	for _, tc := range []struct {
		ext  string
		head []byte
		ok   bool
	}{
		{".pdf", []byte("%PDF-1.4"), true},
		{".pdf", []byte("<html>"), false},
		{".docx", zip, true},
		{".xlsx", []byte("%PDF-1.4"), false},
		{".doc", ole, true},
		{".doc", zip, false},
		{".png", png, true},
		{".jpg", png, false},
		{".7z", []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C, 0}, true},
		{".txt", []byte("merhaba dünya\n"), true},
		{".html", []byte("<!DOCTYPE html><title>x</title>"), true}, // metin olarak saklanır
		{".py", []byte("print('x')\x00"), false},
		{".csv", []byte("no;puan\n1;70\n"), true},
	} {
		if got := kinds[tc.ext].matches(tc.head); got != tc.ok {
			t.Errorf("%s %q: %v, want %v", tc.ext, tc.head, got, tc.ok)
		}
	}
	if kinds[".html"].mime != "text/plain; charset=utf-8" {
		t.Error("HTML düz metin olarak saklanmalı")
	}
}
