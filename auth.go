package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

func hashPassword(pw string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyPassword(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func newID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

// hasSpace 报告字符串是否含任意空白字符（空格、制表、换行、Unicode 空白）。
func hasSpace(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func (s *session) doLogin(username, password string) error {
	username = strings.TrimSpace(username)
	if password == "" || hasSpace(password) {
		// 不区分"格式不对"和"密码错误"，避免信息泄露
		return fmt.Errorf("error.invalid_login")
	}
	var id, hash, status string
	err := db.QueryRow(
		`SELECT id, password_hash, status FROM users WHERE username = ?`, username,
	).Scan(&id, &hash, &status)
	if err != nil || status != "active" || !verifyPassword(password, hash) {
		return fmt.Errorf("error.invalid_login")
	}
	s.userID, s.username, s.nickname = id, username, username
	return nil
}

func (s *session) doRegister(username, password string) error {
	username = strings.TrimSpace(username)
	if len(username) < 2 || len(username) > 32 {
		return fmt.Errorf("error.bad_username")
	}
	if hasSpace(password) {
		return fmt.Errorf("error.password_has_space")
	}
	if len(password) < 6 {
		return fmt.Errorf("error.weak_password")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	id := newID("u")
	_, err = db.Exec(
		`INSERT INTO users(id, username, password_hash, created_at, status) VALUES (?, ?, ?, ?, 'active')`,
		id, username, hash, time.Now().Unix(),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return fmt.Errorf("error.username_taken")
		}
		return fmt.Errorf("error.register_failed")
	}
	s.userID, s.username, s.nickname = id, username, username
	return nil
}