package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

type Config struct {
	Port             string
	DBPath           string
	MediaDir         string
	CacheDir         string
	TranscodeTempDir string
	WebDir           string
	SegmentDuration  int
	BufferSeconds    int
	MaxKeepSegments  int
	AdminUser        string
	AdminPassword    string
	JWTSecret        string
	HwAccel          string
	VaapiDevice      string
}

func Load() *Config {
	defaultTemp := filepath.Join(os.TempDir(), "pistream_hls")
	if runtime.GOOS == "linux" {
		if info, err := os.Stat("/dev/shm"); err == nil && info.IsDir() {
			defaultTemp = "/dev/shm/pistream_hls"
		}
	}

	segDur := 4
	if v := os.Getenv("SEGMENT_DURATION"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 2 && parsed <= 10 {
			segDur = parsed
		}
	}

	bufferSec := DefaultBufferSeconds
	if v := os.Getenv("BUFFER_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= MinBufferSeconds && parsed <= MaxBufferSeconds {
			bufferSec = parsed
		}
	}

	maxKeep := 10
	if v := os.Getenv("MAX_KEEP_SEGMENTS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 4 {
			maxKeep = parsed
		}
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		secret = hex.EncodeToString(b)
	}

	return &Config{
		Port:             getEnv("PORT", "8080"),
		DBPath:           getEnv("DB_PATH", "./data/pistream.db"),
		MediaDir:         getEnv("MEDIA_DIR", "./media"),
		CacheDir:         getEnv("CACHE_DIR", "./data/cache"),
		TranscodeTempDir: getEnv("TRANSCODE_TEMP_DIR", defaultTemp),
		WebDir:           getEnv("WEB_DIR", "./web/dist"),
		SegmentDuration:  segDur,
		BufferSeconds:    bufferSec,
		MaxKeepSegments:  maxKeep,
		AdminUser:        getEnv("ADMIN_USER", "admin"),
		AdminPassword:    getEnv("ADMIN_PASSWORD", "admin123"),
		JWTSecret:        secret,
		HwAccel:          getEnv("HW_ACCEL", ""),
		VaapiDevice:      getEnv("VAAPI_DEVICE", "/dev/dri/renderD128"),
	}
}

const (
	DefaultBufferSeconds = 45
	MinBufferSeconds     = 16
	MaxBufferSeconds     = 120
)

func (c *Config) EffectiveBufferSeconds() int {
	if c == nil || c.BufferSeconds <= 0 {
		return DefaultBufferSeconds
	}
	if c.BufferSeconds < MinBufferSeconds {
		return MinBufferSeconds
	}
	if c.BufferSeconds > MaxBufferSeconds {
		return MaxBufferSeconds
	}
	return c.BufferSeconds
}

func (c *Config) EffectiveSegmentDuration() int {
	if c == nil || c.SegmentDuration <= 0 {
		return 4
	}
	return c.SegmentDuration
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
