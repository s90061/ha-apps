package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("mySecretPass123")
	if err != nil {
		t.Fatalf("雜湊失敗: %v", err)
	}
	if !VerifyPassword("mySecretPass123", hash) {
		t.Fatalf("正確密碼驗證失敗")
	}
	if VerifyPassword("wrongPass", hash) {
		t.Fatalf("錯誤密碼不應通過驗證")
	}
	if _, err := HashPassword(strings.Repeat("a", MaxPasswordBytes+1)); err == nil {
		t.Fatalf("超過上限長度的密碼應被拒絕")
	}
	if VerifyPassword(strings.Repeat("a", MaxPasswordBytes+1), hash) {
		t.Fatalf("超過上限長度的密碼不應通過驗證")
	}
}

func TestSignAndVerifyToken(t *testing.T) {
	secret := "test-secret-key"
	token, err := SignToken(secret, 42, "alice", true, 3, time.Hour)
	if err != nil {
		t.Fatalf("簽發 Token 失敗: %v", err)
	}
	payload, err := VerifyToken(secret, token)
	if err != nil {
		t.Fatalf("驗證 Token 失敗: %v", err)
	}
	if payload.UserID != 42 || payload.Username != "alice" || !payload.IsAdmin || payload.TokenVersion != 3 {
		t.Fatalf("Token 內容不符: %+v", payload)
	}
}

func TestExtractTokenCookieOnly(t *testing.T) {
	// URL ?token= 與 Authorization: Bearer 一律不得被接受
	reqQuery := httptest.NewRequest(http.MethodGet, "/api/library/files?token=leaked-token", nil)
	reqQuery.Header.Set("Authorization", "Bearer leaked-bearer")
	if got := ExtractTokenFromRequest(reqQuery); got != "" {
		t.Fatalf("預期停用 Query/Bearer Token，但收到 %q", got)
	}

	reqCookie := httptest.NewRequest(http.MethodGet, "/api/library/files", nil)
	reqCookie.AddCookie(&http.Cookie{Name: CookieName, Value: "cookie-val"})
	if got := ExtractTokenFromRequest(reqCookie); got != "cookie-val" {
		t.Fatalf("預期從 HttpOnly Cookie 讀取憑證，實際為 %q", got)
	}
}
