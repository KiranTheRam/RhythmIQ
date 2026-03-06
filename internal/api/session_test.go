package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSessionRoundTrip(t *testing.T) {
	manager := newSessionManager("test-secret", false)
	req := httptest.NewRequest("GET", "http://127.0.0.1:8080", nil)
	rec := httptest.NewRecorder()

	if err := manager.setUserID(rec, req, "spotify-user-123"); err != nil {
		t.Fatalf("setUserID() error = %v", err)
	}

	resp := rec.Result()
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one session cookie, got %d", len(cookies))
	}

	reqWithCookie := httptest.NewRequest("GET", "http://127.0.0.1:8080", nil)
	reqWithCookie.AddCookie(cookies[0])

	userID, err := manager.userID(reqWithCookie)
	if err != nil {
		t.Fatalf("userID() error = %v", err)
	}
	if userID != "spotify-user-123" {
		t.Fatalf("userID() = %q, want %q", userID, "spotify-user-123")
	}
}

func TestSessionTamperRejected(t *testing.T) {
	manager := newSessionManager("test-secret", false)
	token, err := manager.sign("spotify-user-123", time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("sign() error = %v", err)
	}

	tampered := "ZmFrZS11c2Vy." + strings.Split(token, ".")[1] + "." + strings.Split(token, ".")[2]
	_, err = manager.verify(tampered)
	if err == nil {
		t.Fatal("verify() expected error for tampered token, got nil")
	}
	if err != errInvalidSession {
		t.Fatalf("verify() error = %v, want %v", err, errInvalidSession)
	}
}

func TestSessionExpired(t *testing.T) {
	manager := newSessionManager("test-secret", false)
	token, err := manager.sign("spotify-user-123", time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("sign() error = %v", err)
	}

	_, err = manager.verify(token)
	if err == nil {
		t.Fatal("verify() expected expiration error, got nil")
	}
	if err != errNoSession {
		t.Fatalf("verify() error = %v, want %v", err, errNoSession)
	}
}
