// Package httpx, HTTP katmanında ortak kullanılan yardımcıları içerir.
package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ListResponse, liste döndüren tüm uç noktaların ortak yanıt zarfıdır.
// Çıplak bir JSON dizisi yerine nesne döndürmek, sayfalama gibi alanları
// geriye dönük uyumluluğu bozmadan eklemeyi sağlar.
//
// NextCursor sadece sayfalanan listelerde ve sonraki sayfa varsa doludur.
// İstemci bir sonraki sayfa için onu ?cursor= parametresiyle geri gönderir.
type ListResponse[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// WriteJSON, v değerini JSON'a çevirir ve verilen durum koduyla yanıta yazar.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	return writeBody(w, status, "application/json; charset=utf-8", v)
}

func writeBody(w http.ResponseWriter, status int, contentType string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("httpx: json kodlanamadı: %w", err)
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("httpx: yanıt yazılamadı: %w", err)
	}

	return nil
}
