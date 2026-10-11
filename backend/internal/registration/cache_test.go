package registration

import (
	"errors"
	"testing"
	"time"
)

func TestTTLCache(t *testing.T) {
	c := newTTLCache[int](50 * time.Millisecond)
	loads := 0
	load := func() (int, error) { loads++; return loads, nil }

	if v, _ := c.get("a", load); v != 1 {
		t.Fatalf("ilk yükleme = %d", v)
	}
	if v, _ := c.get("a", load); v != 1 || loads != 1 {
		t.Errorf("önbellekten okunmalı: %d, %d yükleme", v, loads)
	}
	if v, _ := c.get("b", load); v != 2 {
		t.Errorf("başka anahtar ayrı yüklenir: %d", v)
	}
	time.Sleep(60 * time.Millisecond)
	if v, _ := c.get("a", load); v != 3 {
		t.Errorf("süresi dolan değer yeniden yüklenmeli: %d", v)
	}
	if _, err := c.get("c", func() (int, error) { return 0, errors.New("x") }); err == nil {
		t.Error("yükleme hatası dönmeli")
	}
	if v, _ := c.get("c", load); v != 4 {
		t.Errorf("hata önbelleğe alınmamalı: %d", v)
	}
}
