package mail

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// SMTPSender, e-postaları bir SMTP sunucusu üzerinden gönderir.
//
// net/smtp.SendMail'in zaman aşımı yoktur: yanıt vermeyen bir sunucu worker'ı
// sonsuza kadar bekletirdi. Bu yüzden bağlantı context'in süresine bağlanır.
// Sunucu destekliyorsa bağlantı STARTTLS ile şifrelenir. Kimlik doğrulama
// (PLAIN) şifresiz bir bağlantıda localhost dışına parolayı göndermez.
type SMTPSender struct {
	Addr     string // host:port
	From     string // "Ad <adres>"
	Username string
	Password string
	Timeout  time.Duration // context'te süre yoksa kullanılır
}

// Send, mesajı gönderir.
func (s *SMTPSender) Send(ctx context.Context, m Message) error {
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("mail: gönderen adresi geçersiz: %w", err)
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("mail: alıcı adresi geçersiz: %w", err)
	}
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return fmt.Errorf("mail: SMTP adresi geçersiz: %w", err)
	}

	data, err := m.Build(s.From, time.Now(), newMessageID(from.Address))
	if err != nil {
		return err
	}

	if _, ok := ctx.Deadline(); !ok && s.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("mail: SMTP sunucusuna bağlanılamadı: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: SMTP oturumu açılamadı: %w", err)
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("mail: STARTTLS başarısız: %w", err)
		}
	}
	if s.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, host)); err != nil {
			return fmt.Errorf("mail: SMTP kimlik doğrulaması başarısız: %w", err)
		}
	}

	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("mail: MAIL FROM reddedildi: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("mail: RCPT TO reddedildi: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA başarısız: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("mail: gövde yazılamadı: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: gövde kabul edilmedi: %w", err)
	}
	return c.Quit()
}

// newMessageID, gönderenin alan adıyla benzersiz bir Message-ID üretir.
func newMessageID(fromAddress string) string {
	_, domain, ok := strings.Cut(fromAddress, "@")
	if !ok || domain == "" {
		domain = "agora.local"
	}
	return strings.ToLower(rand.Text()) + "@" + domain
}
