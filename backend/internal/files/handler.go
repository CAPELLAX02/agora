package files

import (
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// uploadTimeout, yükleme isteğinin okunup yanıtlanması için tanınan süredir: sunucunun genel
// okuma zaman aşımı (birkaç saniye) büyük dosyalar için yetmez, bu uçta istek başına uzatılır.
const uploadTimeout = 15 * time.Minute

// Handler, dosya uçlarını sunar.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler, bir Handler oluşturur.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register, route'ları kaydeder. Her oturum açmış kullanıcı dosya yükleyebilir; dosya bir
// varlığa (ödev teslimi, ders içeriği) bağlanana kadar sadece yükleyen görür ve silebilir.
// Bağlandıktan sonra erişimi o varlığın uçları verir.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("POST /api/v1/files", authz.Authenticated, h.upload)
	rt.HandleFunc("GET /api/v1/files/{id}", authz.Authenticated, h.get)
	rt.HandleFunc("GET /api/v1/files/{id}/download-url", authz.Authenticated, h.downloadURL)
	rt.HandleFunc("DELETE /api/v1/files/{id}", authz.Authenticated, h.delete)
}

// FileResponse, bir dosyanın API biçimidir. Dosyayı kullanan varlıkların yanıtları da kullanır.
type FileResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	MimeType   string    `json:"mime_type"`
	Size       int64     `json:"size"`
	SHA256     string    `json:"sha256"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// ToResponse, dosyayı API biçimine çevirir.
func ToResponse(f File) FileResponse {
	return FileResponse{ID: f.ID, Name: f.Name, MimeType: f.MimeType, Size: f.Size,
		SHA256: hex.EncodeToString(f.SHA256), UploadedAt: f.UploadedAt}
}

// URLResponse, imzalı indirme URL'sidir.
type URLResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func userID(r *http.Request) string {
	p, _ := authn.PrincipalFrom(r.Context())
	return p.UserID
}

// upload, multipart/form-data gövdesindeki "file" parçasını akışla depoya yazar. Gövde
// belleğe alınmaz (ParseMultipartForm değil, parça parça okuyucu).
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(uploadTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(uploadTimeout))

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusUnsupportedMediaType, Code: "UNSUPPORTED_MEDIA_TYPE",
			Detail: "Dosya multipart/form-data gövdesinin \"file\" alanında gönderilmeli."})
		return
	}
	// Sınır, dosya boyutu artı multipart başlıkları için pay.
	r.Body = http.MaxBytesReader(w, r.Body, h.svc.MaxBytes()+64<<10)
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.failed(w, r, &RejectError{Code: "FILE_TOO_LARGE", Detail: "Dosya çok büyük."})
			return
		}
		if err != nil {
			_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusBadRequest, Code: "INVALID_BODY", Detail: "Gövde okunamadı."})
			return
		}
		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			continue
		}
		f, err := h.svc.Upload(r.Context(), userID(r), part.FileName(), part)
		_ = part.Close()
		if errors.As(err, &maxErr) {
			err = &RejectError{Code: "FILE_TOO_LARGE", Detail: "Dosya çok büyük."}
		}
		if h.failed(w, r, err) {
			return
		}
		w.Header().Set("Location", "/api/v1/files/"+f.ID)
		h.writeJSON(w, r, http.StatusCreated, ToResponse(f))
		return
	}
	_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusBadRequest, Code: "NO_FILE", Detail: "Gövdede \"file\" alanında bir dosya yok."})
}

// own, path'teki dosyayı okur: sadece yükleyen görür, başkasına 404.
func (h *Handler) own(w http.ResponseWriter, r *http.Request) (File, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return File{}, false
	}
	f, err := h.svc.Get(r.Context(), h.svc.pool, id)
	if err == nil && f.UploadedBy != userID(r) {
		err = ErrNotFound
	}
	if h.failed(w, r, err) {
		return File{}, false
	}
	return f, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if f, ok := h.own(w, r); ok {
		h.writeJSON(w, r, http.StatusOK, ToResponse(f))
	}
}

func (h *Handler) downloadURL(w http.ResponseWriter, r *http.Request) {
	f, ok := h.own(w, r)
	if !ok {
		return
	}
	u, exp, err := h.svc.DownloadURL(r.Context(), f)
	if h.failed(w, r, err) {
		return
	}
	h.writeJSON(w, r, http.StatusOK, URLResponse{URL: u, ExpiresAt: exp})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	if h.failed(w, r, h.svc.Delete(r.Context(), userID(r), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Failed, dosya hatalarını problem yanıtına çevirir. Dosyayı kullanan paketler de kullanır.
func Failed(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) bool {
	var (
		reject  *RejectError
		storage *StorageError
	)
	switch {
	case err == nil:
		return false
	case errors.As(err, &reject):
		status := http.StatusUnsupportedMediaType
		switch reject.Code {
		case "FILE_TOO_LARGE":
			status = http.StatusRequestEntityTooLarge
		case "FILE_EMPTY":
			status = http.StatusBadRequest
		}
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: status, Code: reject.Code, Detail: reject.Detail})
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrAttached):
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusConflict, Code: "FILE_IN_USE",
			Detail: "Dosya bir kayda bağlı; buradan silinemez."})
	case errors.As(err, &storage):
		logger.Error("dosya deposuna ulaşılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusServiceUnavailable, Code: "STORAGE_UNAVAILABLE",
			Detail: "Dosya deposuna şu an ulaşılamıyor; biraz sonra tekrar deneyin."})
	default:
		logger.Error("beklenmeyen hata", "err", err, "path", r.URL.Path, "request_id", httpx.RequestIDFrom(r.Context()))
		httpx.InternalServerError(w, r)
	}
	return true
}

func (h *Handler) failed(w http.ResponseWriter, r *http.Request, err error) bool {
	return Failed(w, r, h.logger, err)
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}
