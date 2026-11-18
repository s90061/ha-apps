package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const UserContextKey contextKey = "pistream_user"
const CookieName = "pistream_token"

type TokenPayload struct {
	UserID   int64  `json:"uid"`
	Username string `json:"usr"`
	IsAdmin  bool   `json:"adm"`
	Exp      int64  `json:"exp"`
}

// HashPassword 使用多次迭代 HMAC-SHA256 + 隨機 Salt 進行密碼雜湊 (零外部 CGO 依賴)
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := deriveKey([]byte(password), salt, 60000)
	return fmt.Sprintf("%s$%s", hex.EncodeToString(salt), hex.EncodeToString(dk)), nil
}

// VerifyPassword 驗證明文密碼是否與雜湊值相符
func VerifyPassword(password, storedHash string) bool {
	parts := strings.Split(storedHash, "$")
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	expectedDK, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	actualDK := deriveKey([]byte(password), salt, 60000)
	return subtle.ConstantTimeCompare(expectedDK, actualDK) == 1
}

func deriveKey(password, salt []byte, iterations int) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	digest := mac.Sum(nil)
	for i := 1; i < iterations; i++ {
		mac.Reset()
		mac.Write(digest)
		digest = mac.Sum(digest[:0])
	}
	return digest
}

// SignToken 簽發包含過期時間的無狀態 HMAC-SHA256 Token
func SignToken(secret string, userID int64, username string, isAdmin bool, ttl time.Duration) (string, error) {
	payload := TokenPayload{
		UserID:   userID,
		Username: username,
		IsAdmin:  isAdmin,
		Exp:      time.Now().Add(ttl).Unix(),
	}
	rawJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	b64Payload := base64.RawURLEncoding.EncodeToString(rawJSON)
	sig := computeHMAC(b64Payload, secret)
	return b64Payload + "." + sig, nil
}

// VerifyToken 驗證並解析 Token
func VerifyToken(secret, token string) (*TokenPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, errors.New("無效的憑證格式")
	}
	expectedSig := computeHMAC(parts[0], secret)
	if subtle.ConstantTimeCompare([]byte(expectedSig), []byte(parts[1])) != 1 {
		return nil, errors.New("憑證簽章驗證失敗")
	}
	rawJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("憑證內容解析失敗")
	}
	var payload TokenPayload
	if err := json.Unmarshal(rawJSON, &payload); err != nil {
		return nil, errors.New("憑證結構無效")
	}
	if time.Now().Unix() > payload.Exp {
		return nil, errors.New("登入憑證已過期，請重新登入")
	}
	return &payload, nil
}

func computeHMAC(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// ExtractTokenFromRequest 支援從 Cookie、Authorization Header 或 URL Query (?token=) 讀取憑證
func ExtractTokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		return c.Value
	}
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	if q := r.URL.Query().Get("token"); q != "" {
		return q
	}
	return ""
}

// WithUser 將解析後的使用者資訊注入 Request Context
func WithUser(ctx context.Context, user *TokenPayload) context.Context {
	return context.WithValue(ctx, UserContextKey, user)
}

// GetUser 從 Request Context 取得當前登入使用者
func GetUser(r *http.Request) *TokenPayload {
	if val, ok := r.Context().Value(UserContextKey).(*TokenPayload); ok {
		return val
	}
	return nil
}
