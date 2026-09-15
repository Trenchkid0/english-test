package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	authCookieName = "ruang_kata_session"
	passwordRounds = 210000
)

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type authUser struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type authContextKey struct{}

func userFromContext(ctx context.Context) (authUser, bool) {
	user, ok := ctx.Value(authContextKey{}).(authUser)
	return user, ok
}

func mustUser(r *http.Request) authUser {
	user, _ := userFromContext(r.Context())
	return user
}

func userID(ctx context.Context) int64 {
	user, _ := userFromContext(ctx)
	return user.ID
}

func (a *app) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !a.allow(r) {
		writeError(w, http.StatusTooManyRequests, "Terlalu banyak percobaan. Tunggu satu menit lalu coba lagi.")
		return
	}
	var body struct{ Name, Email, Password string }
	if decodeJSON(w, r, &body) != nil {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	if len(body.Name) < 2 || len(body.Name) > 80 {
		writeError(w, 422, "Nama harus berisi 2–80 karakter.")
		return
	}
	if len(body.Email) > 190 || !emailPattern.MatchString(body.Email) {
		writeError(w, 422, "Alamat email tidak valid. Periksa format lalu coba lagi.")
		return
	}
	if message := validatePassword(body.Password); message != "" {
		writeError(w, 422, message)
		return
	}
	hash, err := hashPassword(body.Password)
	if err != nil {
		writeError(w, 500, "Akun belum dapat dibuat.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "Akun belum dapat dibuat.")
		return
	}
	defer tx.Rollback()
	var firstID int64
	err = tx.QueryRowContext(r.Context(), `SELECT id FROM users ORDER BY id LIMIT 1 FOR UPDATE`).Scan(&firstID)
	firstUser := errors.Is(err, sql.ErrNoRows)
	if err != nil && !firstUser {
		writeError(w, 500, "Akun belum dapat dibuat.")
		return
	}
	role := "learner"
	if firstUser {
		role = "admin"
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(r.Context(), `INSERT INTO users (name,email,password_hash,role,created_at,updated_at) VALUES (?,?,?,?,?,?)`, body.Name, body.Email, hash, role, now, now)
	if err != nil {
		writeError(w, 409, "Email tersebut sudah terdaftar. Masuk dengan akun yang sudah ada.")
		return
	}
	id, _ := result.LastInsertId()
	if firstUser {
		for _, table := range []string{"practice_sessions", "review_cards", "vocabulary_cards", "writing_submissions", "user_question_history"} {
			if _, err = tx.ExecContext(r.Context(), "UPDATE "+table+" SET user_id=? WHERE user_id=0", id); err != nil {
				writeError(w, 500, "Data lama belum dapat dipindahkan ke akun pertama.")
				return
			}
		}
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, "Akun belum dapat dibuat.")
		return
	}
	user := authUser{ID: id, Name: body.Name, Email: body.Email, Role: role}
	if err := a.startAuthSession(w, r, user); err != nil {
		writeError(w, 500, "Akun dibuat, tetapi sesi login belum dapat dimulai.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !a.allow(r) {
		writeError(w, http.StatusTooManyRequests, "Terlalu banyak percobaan. Tunggu satu menit lalu coba lagi.")
		return
	}
	var body struct{ Email, Password string }
	if decodeJSON(w, r, &body) != nil {
		return
	}
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	var user authUser
	var passwordHash string
	err := a.db.QueryRowContext(r.Context(), `SELECT id,name,email,password_hash,role FROM users WHERE email=?`, body.Email).Scan(&user.ID, &user.Name, &user.Email, &passwordHash, &user.Role)
	if err != nil || !verifyPassword(body.Password, passwordHash) {
		writeError(w, http.StatusUnauthorized, "Email atau password tidak cocok. Periksa keduanya lalu coba lagi.")
		return
	}
	if err := a.startAuthSession(w, r, user); err != nil {
		writeError(w, 500, "Sesi login belum dapat dibuat.")
		return
	}
	writeJSON(w, 200, map[string]any{"user": user})
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if cookie, err := r.Cookie(authCookieName); err == nil {
		_, _ = a.db.ExecContext(r.Context(), `DELETE FROM auth_sessions WHERE token_hash=?`, tokenHash(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: authCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: requestIsHTTPS(r)})
	writeJSON(w, 200, map[string]bool{"loggedOut": true})
}

func (a *app) me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, 200, map[string]any{"user": mustUser(r)})
}

func (a *app) startAuthSession(w http.ResponseWriter, r *http.Request, user authUser) error {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	now := time.Now().UTC()
	expires := now.Add(30 * 24 * time.Hour)
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO auth_sessions (token_hash,user_id,expires_at,created_at) VALUES (?,?,?,?)`, tokenHash(token), user.ID, expires, now)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: authCookieName, Value: token, Path: "/", Expires: expires, MaxAge: 30 * 24 * 60 * 60, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: requestIsHTTPS(r)})
	return nil
}

func (a *app) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(authCookieName)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "Silakan masuk untuk melanjutkan.")
			return
		}
		var user authUser
		err = a.db.QueryRowContext(r.Context(), `SELECT u.id,u.name,u.email,u.role FROM auth_sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>?`, tokenHash(cookie.Value), time.Now().UTC()).Scan(&user.ID, &user.Name, &user.Email, &user.Role)
		if err != nil {
			http.SetCookie(w, &http.Cookie{Name: authCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: requestIsHTTPS(r)})
			writeError(w, http.StatusUnauthorized, "Sesi login berakhir. Silakan masuk kembali.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey{}, user)))
	})
}

func (a *app) requireAdmin(next http.Handler) http.Handler {
	return a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mustUser(r).Role != "admin" {
			writeError(w, http.StatusForbidden, "Fitur ini hanya tersedia untuk admin.")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func validatePassword(password string) string {
	if len(password) < 10 || len(password) > 128 {
		return "Password harus berisi 10–128 karakter."
	}
	var letter, number bool
	for _, r := range password {
		if r >= '0' && r <= '9' {
			number = true
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letter = true
		}
	}
	if !letter || !number {
		return "Password harus memuat minimal satu huruf dan satu angka."
	}
	return ""
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	derived := pbkdf2SHA256([]byte(password), salt, passwordRounds, 32)
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", passwordRounds, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(derived)), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	rounds, err := strconv.Atoi(parts[1])
	if err != nil || rounds < 100000 || rounds > 1000000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	expected, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(expected) != 32 {
		return false
	}
	actual := pbkdf2SHA256([]byte(password), salt, rounds, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func pbkdf2SHA256(password, salt []byte, rounds, length int) []byte {
	hashLength := sha256.Size
	blocks := (length + hashLength - 1) / hashLength
	result := make([]byte, 0, blocks*hashLength)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < rounds; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		result = append(result, t...)
	}
	return result[:length]
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func ensureOwnershipSchema(ctx context.Context, db *sql.DB) error {
	for _, item := range []struct{ table, definition string }{
		{"practice_sessions", "user_id BIGINT NOT NULL DEFAULT 0"},
		{"review_cards", "user_id BIGINT NOT NULL DEFAULT 0"},
		{"vocabulary_cards", "user_id BIGINT NOT NULL DEFAULT 0"},
		{"writing_submissions", "user_id BIGINT NOT NULL DEFAULT 0"},
	} {
		if _, err := db.ExecContext(ctx, "ALTER TABLE "+item.table+" ADD COLUMN IF NOT EXISTS "+item.definition); err != nil {
			return fmt.Errorf("migration ownership %s: %w", item.table, err)
		}
	}
	if exists, err := indexExists(ctx, db, "vocabulary_cards", "uq_vocabulary_word"); err != nil {
		return err
	} else if exists {
		if _, err := db.ExecContext(ctx, `ALTER TABLE vocabulary_cards DROP INDEX uq_vocabulary_word`); err != nil {
			return fmt.Errorf("migration vocabulary index: %w", err)
		}
	}
	for _, item := range []struct{ table, name, columns string }{
		{"practice_sessions", "idx_sessions_user", "user_id, created_at"},
		{"review_cards", "idx_review_user_due", "user_id, next_review_at"},
		{"vocabulary_cards", "uq_vocabulary_user_word", "user_id, word"},
		{"writing_submissions", "idx_writing_user_created", "user_id, created_at"},
	} {
		exists, err := indexExists(ctx, db, item.table, item.name)
		if err != nil {
			return err
		}
		if !exists {
			kind := "INDEX"
			if strings.HasPrefix(item.name, "uq_") {
				kind = "UNIQUE KEY"
			}
			if _, err := db.ExecContext(ctx, "ALTER TABLE "+item.table+" ADD "+kind+" "+item.name+" ("+item.columns+")"); err != nil {
				return fmt.Errorf("migration index %s: %w", item.name, err)
			}
		}
	}
	return nil
}

func indexExists(ctx context.Context, db *sql.DB, table, name string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=?`, table, name).Scan(&count)
	return count > 0, err
}
