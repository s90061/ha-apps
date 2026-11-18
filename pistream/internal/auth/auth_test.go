package auth

import (
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
}

func TestSignAndVerifyToken(t *testing.T) {
	secret := "test-secret-key"
	token, err := SignToken(secret, 42, "alice", true, time.Hour)
	if err != nil {
		t.Fatalf("簽發 Token 失敗: %v", err)
	}
	payload, err := VerifyToken(secret, token)
	if err != nil {
		t.Fatalf("驗證 Token 失敗: %v", err)
	}
	if payload.UserID != 42 || payload.Username != "alice" || !payload.IsAdmin {
		t.Fatalf("Token 內容不符: %+v", payload)
	}
}
