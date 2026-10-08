package iam_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/iam"
)

func (e *authEnv) loginFrom(t *testing.T, username string, client iam.ClientType, ua string) iam.Tokens {
	t.Helper()
	tok, err := e.auth.Login(context.Background(), iam.LoginInput{
		Username: username, Password: pw, Client: client, IP: "198.51.100.4", UserAgent: ua,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestSessions(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "22290050", pw, e.hasher)

	laptop := e.loginFrom(t, "22290050", iam.ClientWeb, "Mozilla/5.0 (Macintosh) Chrome/140.0 Safari/537.36")
	e.clock.Advance(time.Minute)
	phone := e.loginFrom(t, "22290050", iam.ClientMobile, "Agora/1.0 (iPhone)")
	e.clock.Advance(time.Minute)
	old := e.loginFrom(t, "22290050", iam.ClientWeb, "eski tarayıcı")

	// Eski oturum boşta kalma süresini aşsın: diğer ikisi refresh ile canlı kalır.
	e.clock.Advance(90 * time.Minute)
	if _, err := e.auth.Refresh(ctx, laptop.RefreshToken); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Minute)
	if _, err := e.auth.Refresh(ctx, phone.RefreshToken); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(40 * time.Minute) // eski oturumun refresh token'ı (2 sa) doldu

	sessions, err := e.auth.Sessions(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("%d oturum listelendi, 2 bekleniyordu (boşta kalma süresi dolan hariç): %+v", len(sessions), sessions)
	}
	if sessions[0].ID != phone.SessionID || sessions[1].ID != laptop.SessionID {
		t.Errorf("son görülmeye göre sıralı değil: %+v", sessions)
	}
	if sessions[0].ClientType != iam.ClientMobile || sessions[0].IP != "198.51.100.4" || sessions[0].UserAgent != "Agora/1.0 (iPhone)" {
		t.Errorf("oturum = %+v", sessions[0])
	}

	t.Run("tek oturumu kapatma", func(t *testing.T) {
		if err := e.auth.EndSession(ctx, userID, phone.SessionID); err != nil {
			t.Fatal(err)
		}
		if !e.revoker.has(phone.SessionID) {
			t.Error("kapatılan oturum iptal listesine eklenmedi")
		}
		if _, err := e.auth.Refresh(ctx, phone.RefreshToken); !errors.Is(err, iam.ErrInvalidRefreshToken) {
			t.Errorf("kapatılan oturumla refresh: %v", err)
		}
		// Tekrar kapatmak hata değil, olay yazmaz.
		if err := e.auth.EndSession(ctx, userID, phone.SessionID); err != nil {
			t.Errorf("ikinci kapatma: %v", err)
		}
		if got := e.eventCounts(t, userID)[audit.EventSessionRevoked]; got != 1 {
			t.Errorf("SESSION_REVOKED %d kez yazıldı", got)
		}
	})

	t.Run("başkasının oturumu kapatılamaz", func(t *testing.T) {
		otherID := e.addUser(t, "22290051", pw, e.hasher)
		if err := e.auth.EndSession(ctx, otherID, laptop.SessionID); !errors.Is(err, iam.ErrNotFound) {
			t.Errorf("err = %v, ErrNotFound bekleniyordu", err)
		}
		if reason := e.sessionRevokeReason(t, laptop.SessionID); reason != "" {
			t.Error("başkasının oturumu kapandı")
		}
	})

	t.Run("diğer oturumları kapatma", func(t *testing.T) {
		current := e.loginFrom(t, "22290050", iam.ClientWeb, "yeni")
		n, err := e.auth.EndOtherSessions(ctx, userID, current.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		// laptop ve boşta kalma süresi dolan eski oturum (iptal edilmemişti) kapanır.
		if n != 2 {
			t.Errorf("%d oturum kapatıldı, 2 bekleniyordu", n)
		}
		if reason := e.sessionRevokeReason(t, old.SessionID); reason != "LOGOUT_ALL" {
			t.Errorf("eski oturum sebebi = %q", reason)
		}
		remaining, _ := e.auth.Sessions(ctx, userID)
		if len(remaining) != 1 || remaining[0].ID != current.SessionID {
			t.Errorf("kalan oturumlar = %+v", remaining)
		}
	})
}
