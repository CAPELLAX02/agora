package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// channel, yeni bildirimlerin duyurulduğu PostgreSQL kanalıdır. notify.Send, bildirimi yazdığı
// transaction'da alıcı başına pg_notify çağırır: duyuru ancak commit edilince gider, yani
// ayrı bir "commit sonrası" adımı gerekmez. API'nin her kopyası kanalı dinler.
const channel = "agora_notifications"

// messageChannel, yeni iletilerin duyurulduğu kanaldır: veri "kullanıcı:konuşma". Akış bunu
// "message" olayı olarak iletir; istemci konuşmayı tazeler (iletiler bildirim merkezine girmez).
const messageChannel = "agora_messages"

// MaxStreamsPerUser, bir kullanıcının aynı anda açabileceği canlı akış sayısıdır (sekmeler).
const MaxStreamsPerUser = 5

// ErrTooManyStreams: kullanıcının açık akış sınırı doldu.
var ErrTooManyStreams = errors.New("notify: açık canlı akış sınırı doldu")

// Hub, canlı bildirim akışlarının dağıtıcısıdır: PostgreSQL kanalını tek bir bağlantıyla dinler
// ve kullanıcının açık akışlarını uyandırır. Akış, uyanınca kaçırdıklarını veritabanından okur;
// sinyal içerik taşımaz, bu yüzden kaybolan ya da birleşen sinyal veri kaybettirmez.
type Hub struct {
	pool   *pgxpool.Pool
	logger *slog.Logger

	mu    sync.Mutex
	subs  map[string]map[*subscription]struct{} // kullanıcı → açık akışlar
	count int

	done      chan struct{}
	closeOnce sync.Once
}

// NewHub, bir Hub oluşturur; dinlemek için Run çağrılmalı.
func NewHub(pool *pgxpool.Pool, logger *slog.Logger) *Hub {
	return &Hub{pool: pool, logger: logger, subs: map[string]map[*subscription]struct{}{}, done: make(chan struct{})}
}

// Run, kanalı ctx iptal edilene ya da Close çağrılana kadar dinler. Bağlantı koparsa üstel
// geri çekilmeyle yeniden bağlanır.
func (h *Hub) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-h.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	backoff := time.Second
	for ctx.Err() == nil {
		err := h.listen(ctx)
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("bildirim kanalı dinlenemiyor, yeniden denenecek", "err", err, "after", backoff.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (h *Hub) listen(ctx context.Context) error {
	pooled, err := h.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	// Bağlantı LISTEN durumunda kalacağı için havuzdan alınır ve iş bitince kapatılır.
	conn := pooled.Hijack()
	defer conn.Close(context.Background())
	if _, err := conn.Exec(ctx, "LISTEN "+channel+"; LISTEN "+messageChannel); err != nil {
		return err
	}
	// Dinleme başlamadan (ya da bağlantı kopukken) gelen duyurular kaçmış olabilir: açık akışlar
	// kaçırdıklarını okusun.
	h.wakeAll()
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		if n.Channel == messageChannel {
			if user, conversation, ok := strings.Cut(n.Payload, ":"); ok {
				h.message(user, conversation)
			}
			continue
		}
		h.wake(n.Payload)
	}
}

// subscription, bir açık akıştır: bildirim sinyali (birleşir) ve yeni ileti olan konuşmalar.
type subscription struct {
	notify   chan struct{}
	messages chan string
}

// subscribe, kullanıcı için bir akış aboneliği açar; bırakmak için dönen fonksiyon çağrılır.
func (h *Hub) subscribe(userID string) (*subscription, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subs[userID]) >= MaxStreamsPerUser {
		return nil, nil, ErrTooManyStreams
	}
	sub := &subscription{notify: make(chan struct{}, 1), messages: make(chan string, 16)}
	if h.subs[userID] == nil {
		h.subs[userID] = map[*subscription]struct{}{}
	}
	h.subs[userID][sub] = struct{}{}
	h.count++
	return sub, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[userID][sub]; !ok {
			return
		}
		delete(h.subs[userID], sub)
		if len(h.subs[userID]) == 0 {
			delete(h.subs, userID)
		}
		h.count--
	}, nil
}

// wake, kullanıcının akışlarını uyandırır. Sinyal beklemedeyse yenisi eklenmez (birleşir).
func (h *Hub) wake(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs[userID] {
		select {
		case sub.notify <- struct{}{}:
		default:
		}
	}
}

// message, kullanıcının akışlarına konuşmada yeni ileti olduğunu iletir. Kuyruk doluysa düşer:
// istemci zaten konuşmaları tazeler.
func (h *Hub) message(userID, conversationID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs[userID] {
		select {
		case sub.messages <- conversationID:
		default:
		}
	}
}

func (h *Hub) wakeAll() {
	h.mu.Lock()
	users := make([]string, 0, len(h.subs))
	for u := range h.subs {
		users = append(users, u)
	}
	h.mu.Unlock()
	for _, u := range users {
		h.wake(u)
	}
}

// Streams, açık canlı akış sayısıdır (metrik).
func (h *Hub) Streams() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

// Done, Hub kapanınca kapanan kanaldır: akışlar kendini bitirir.
func (h *Hub) Done() <-chan struct{} {
	return h.done
}

// Close, dinlemeyi bırakır ve açık akışları kapatır (sunucu kapanırken). Tekrarı zararsızdır.
func (h *Hub) Close() {
	h.closeOnce.Do(func() { close(h.done) })
}

// AnnounceMessages, konuşmadaki yeni iletiyi alıcıların açık akışlarına duyurur; çağıranın
// transaction'ında çağrılır, PostgreSQL commit edilince iletir.
func AnnounceMessages(ctx context.Context, q db.Querier, userIDs []string, conversationID string) error {
	if len(userIDs) == 0 {
		return nil
	}
	if _, err := q.Exec(ctx, `SELECT pg_notify('`+messageChannel+`', u || ':' || $2) FROM unnest($1::text[]) AS u`,
		userIDs, conversationID); err != nil {
		return fmt.Errorf("notify: ileti duyurulamadı: %w", err)
	}
	return nil
}
