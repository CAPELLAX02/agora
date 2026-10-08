package httpx

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Sayfalama sınırları.
const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100
)

// ParseLimit, "limit" sorgu parametresini okur. Yoksa DefaultPageLimit döner.
// Geçersizse alan hatası döner.
func ParseLimit(q url.Values) (int, *FieldError) {
	v := q.Get("limit")
	if v == "" {
		return DefaultPageLimit, nil
	}

	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > MaxPageLimit {
		return 0, &FieldError{
			Field:   "limit",
			Message: fmt.Sprintf("1 ile %d arasında bir tamsayı olmalı", MaxPageLimit),
		}
	}

	return n, nil
}

// EncodeCursor, sayfalama konumunu istemciye verilecek opak bir metne çevirir.
// İstemci içeriği yorumlamamalı, sadece bir sonraki istekte geri göndermelidir.
//
// URL-güvenli base64 kullanılır: standart alfabedeki '+' sorgu parametresinde
// boşluğa dönüşür ve cursor bozulur.
func EncodeCursor(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("httpx: cursor kodlanamadı: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// DecodeCursor, EncodeCursor ile üretilmiş metni T tipine geri çevirir.
func DecodeCursor[T any](s string) (T, error) {
	var v T

	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return v, fmt.Errorf("httpx: cursor çözümlenemedi: %w", err)
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return v, fmt.Errorf("httpx: cursor çözümlenemedi: %w", err)
	}

	return v, nil
}
