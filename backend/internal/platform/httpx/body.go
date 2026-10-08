package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// MaxBodyBytes, JSON istek gövdesinin kabul edilen en büyük boyutudur.
const MaxBodyBytes = 1 << 20 // 1 MiB

// BodyError, istek gövdesi okunamadığında dönen hatadır. Status ve Code
// istemciye dönülecek yanıtı belirler, Detail kullanıcıya gösterilebilir.
type BodyError struct {
	Status int
	Code   string
	Detail string
}

func (e *BodyError) Error() string {
	return "httpx: " + e.Detail
}

// ReadJSON, istek gövdesini dst'ye çözer. Gövde application/json olmalı,
// MaxBodyBytes'ı aşmamalı, tek bir JSON değeri içermeli ve dst'de karşılığı
// olmayan alan içermemeli. Hatalar *BodyError olarak döner ve InvalidBody ile
// istemciye yazılır.
func ReadJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	// Content-Type zorunlu: HTML formları application/json gönderemez, bu yüzden
	// başka bir siteden gelen form ile sahte istek (CSRF) bu uçlara ulaşamaz.
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return &BodyError{
			Status: http.StatusUnsupportedMediaType,
			Code:   "UNSUPPORTED_MEDIA_TYPE",
			Detail: "İstek gövdesi application/json olmalı.",
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}

	// İlk değerden sonra boşluk dışında bir şey kalmamalı: {"a":1}{"b":2} reddedilir.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return invalidBody("İstek gövdesi tek bir JSON değeri içermeli.")
	}
	return nil
}

func decodeError(err error) error {
	var (
		syntaxErr   *json.SyntaxError
		typeErr     *json.UnmarshalTypeError
		maxBytesErr *http.MaxBytesError
		invalidErr  *json.InvalidUnmarshalError
	)

	switch {
	case errors.Is(err, io.EOF):
		return invalidBody("İstek gövdesi boş.")
	case errors.Is(err, io.ErrUnexpectedEOF):
		return invalidBody("JSON yarım kalmış.")
	case errors.As(err, &syntaxErr):
		return invalidBody(fmt.Sprintf("Geçersiz JSON (%d. bayt).", syntaxErr.Offset))
	case errors.As(err, &typeErr):
		if typeErr.Field != "" {
			return invalidBody(fmt.Sprintf("%q alanının türü yanlış: %s bekleniyor.", typeErr.Field, typeErr.Type))
		}
		return invalidBody(fmt.Sprintf("JSON türü yanlış: %s bekleniyor.", typeErr.Type))
	case errors.As(err, &maxBytesErr):
		return &BodyError{
			Status: http.StatusRequestEntityTooLarge,
			Code:   "BODY_TOO_LARGE",
			Detail: fmt.Sprintf("İstek gövdesi en fazla %d bayt olabilir.", maxBytesErr.Limit),
		}
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		// encoding/json bu durum için tipli bir hata sunmuyor, mesajdan ayırt ediyoruz.
		field := strings.TrimPrefix(err.Error(), "json: unknown field ")
		return invalidBody(fmt.Sprintf("Bilinmeyen alan: %s.", field))
	case errors.As(err, &invalidErr):
		// dst pointer değil: istemcinin değil bizim hatamız. Recover middleware'i 500 döner.
		panic(err)
	default:
		return invalidBody("İstek gövdesi okunamadı.")
	}
}

func invalidBody(detail string) *BodyError {
	return &BodyError{Status: http.StatusBadRequest, Code: "INVALID_BODY", Detail: detail}
}

// InvalidBody, ReadJSON'dan dönen hatayı problem yanıtı olarak yazar.
func InvalidBody(w http.ResponseWriter, r *http.Request, err error) {
	be := invalidBody("İstek gövdesi okunamadı.")
	errors.As(err, &be)
	_ = WriteProblem(w, r, Problem{Status: be.Status, Code: be.Code, Detail: be.Detail})
}
