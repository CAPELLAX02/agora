package richtext

import "testing"

func TestSanitize(t *testing.T) {
	got := Sanitize(`<p onclick="x()">Merhaba <script>alert(1)</script><a href="https://go.dev">Go</a></p>`)
	want := `<p>Merhaba <a href="https://go.dev" rel="nofollow noreferrer noopener" target="_blank">Go</a></p>`
	if got != want {
		t.Errorf("Sanitize = %s", got)
	}
}

func TestPlainTextAndEmpty(t *testing.T) {
	if got := PlainText("<p>Ara sınav <strong>12 Kasım</strong>'da.</p><ul><li>Bir</li><li>İki</li></ul>"); got != "Ara sınav 12 Kasım'da.\nBir\nİki" {
		t.Errorf("PlainText = %q", got)
	}
	if !Empty("<p>&nbsp; </p><p></p>") || Empty("<p>x</p>") {
		t.Error("Empty yanlış")
	}
}
