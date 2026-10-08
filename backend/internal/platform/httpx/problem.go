package httpx

import "net/http"

// Problem, RFC 9457 "Problem Details for HTTP APIs" yanıt gövdesidir.
// Code, RequestID ve Errors standarda eklenmiş uzantı alanlarıdır.
type Problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Instance  string       `json:"instance,omitempty"`
	Code      string       `json:"code,omitempty"`
	RequestID string       `json:"request_id,omitempty"`
	Errors    []FieldError `json:"errors,omitempty"`
}

// FieldError, doğrulama hatalarında tek bir alana ait hatayı anlatır.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// WriteProblem, p'yi application/problem+json olarak yazar.
// Boş bırakılan Type, Title, Instance ve RequestID alanlarını standart değerlerle doldurur.
func WriteProblem(w http.ResponseWriter, r *http.Request, p Problem) error {
	if p.Type == "" {
		p.Type = "about:blank"
	}
	if p.Title == "" {
		p.Title = http.StatusText(p.Status)
	}
	if p.Instance == "" {
		p.Instance = r.URL.Path
	}
	if p.RequestID == "" {
		p.RequestID = RequestIDFrom(r.Context())
	}
	return writeBody(w, p.Status, "application/problem+json", p)
}

// NotFound, 404 problem yanıtı yazar.
func NotFound(w http.ResponseWriter, r *http.Request) {
	_ = WriteProblem(
		w,
		r,
		Problem{
			Status: http.StatusNotFound,
			Code:   "NOT_FOUND",
			Detail: "İstenen kaynak bulunamadı.",
		},
	)
}

// ValidationFailed, istekteki alan hatalarını 400 problem yanıtı olarak yazar.
func ValidationFailed(w http.ResponseWriter, r *http.Request, errs []FieldError) {
	_ = WriteProblem(
		w,
		r,
		Problem{
			Status: http.StatusBadRequest,
			Code:   "VALIDATION_FAILED",
			Detail: "İstek geçersiz alanlar içeriyor.",
			Errors: errs,
		},
	)
}

// MethodNotAllowed, 405 problem yanıtı yazar. Allow başlığını çağıran taraf ayarlar.
func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	_ = WriteProblem(
		w,
		r,
		Problem{
			Status: http.StatusMethodNotAllowed,
			Code:   "METHOD_NOT_ALLOWED",
			Detail: "Bu kaynak için HTTP metodu desteklenmiyor.",
		},
	)
}

// InternalServerError, 500 problem yanıtını yazar. Ayrıntı istemciye sızdırılmaz.
func InternalServerError(w http.ResponseWriter, r *http.Request) {
	_ = WriteProblem(
		w,
		r,
		Problem{
			Status: http.StatusInternalServerError,
			Code:   "INTERNAL_ERROR",
			Detail: "Beklenmeyen bir hata oluştu.",
		},
	)
}
