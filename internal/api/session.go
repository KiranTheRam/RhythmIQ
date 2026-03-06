package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const sessionCookieName = "rhythmiq_session"

var (
	errNoSession      = errors.New("no active session")
	errInvalidSession = errors.New("invalid session")
)

type sessionManager struct {
	secret      []byte
	maxAge      time.Duration
	cookieName  string
	forceSecure bool
}

func newSessionManager(secret string, forceSecure bool) *sessionManager {
	cookieName := sessionCookieName
	if forceSecure {
		cookieName = "__Host-rhythmiq_session"
	}

	return &sessionManager{
		secret:      []byte(secret),
		maxAge:      14 * 24 * time.Hour,
		cookieName:  cookieName,
		forceSecure: forceSecure,
	}
}

func (m *sessionManager) setUserID(w http.ResponseWriter, r *http.Request, userID string) error {
	token, err := m.sign(userID, time.Now().Add(m.maxAge))
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.shouldSecure(r),
		MaxAge:   int(m.maxAge.Seconds()),
	})
	return nil
}

func (m *sessionManager) clear(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.shouldSecure(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func (m *sessionManager) userID(r *http.Request) (string, error) {
	cookie, err := r.Cookie(m.cookieName)
	if err != nil || cookie == nil || cookie.Value == "" {
		return "", errNoSession
	}

	userID, err := m.verify(cookie.Value)
	if err != nil {
		return "", err
	}
	return userID, nil
}

func (m *sessionManager) sign(userID string, expiresAt time.Time) (string, error) {
	if strings.TrimSpace(userID) == "" {
		return "", fmt.Errorf("user id cannot be empty")
	}

	encodedUserID := base64.RawURLEncoding.EncodeToString([]byte(userID))
	expiresPart := strconv.FormatInt(expiresAt.UTC().Unix(), 10)
	payload := encodedUserID + "." + expiresPart
	signature := base64.RawURLEncoding.EncodeToString(m.signPayload(payload))
	return payload + "." + signature, nil
}

func (m *sessionManager) verify(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errInvalidSession
	}

	payload := parts[0] + "." + parts[1]
	providedSignature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errInvalidSession
	}
	expectedSignature := m.signPayload(payload)
	if !hmac.Equal(providedSignature, expectedSignature) {
		return "", errInvalidSession
	}

	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", errInvalidSession
	}
	if time.Now().After(time.Unix(expiresUnix, 0)) {
		return "", errNoSession
	}

	decodedUserID, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", errInvalidSession
	}
	userID := strings.TrimSpace(string(decodedUserID))
	if userID == "" {
		return "", errInvalidSession
	}

	return userID, nil
}

func (m *sessionManager) signPayload(payload string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func isSecureRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (m *sessionManager) shouldSecure(r *http.Request) bool {
	if m.forceSecure {
		return true
	}
	return isSecureRequest(r)
}
