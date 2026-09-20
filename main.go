package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
)

//go:embed web/*
var webFiles embed.FS

var safeDBName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

const schemaVersion = "schema_20260920_question_bank_performance_v2"

type config struct {
	Addr, DBHost, DBPort, DBUser, DBPassword, DBName string
	DeepSeekKey, DeepSeekBaseURL, DeepSeekModel      string
}

type app struct {
	db                 *sql.DB
	cfg                config
	client             *http.Client
	skipReviewSchedule bool
	mu                 sync.Mutex
	hits               map[string][]time.Time
}

type generateInput struct {
	Level           string   `json:"level"`
	IELTSTarget     float64  `json:"ieltsTarget"`
	Count           int      `json:"count"`
	DurationMinutes int      `json:"durationMinutes"`
	Types           []string `json:"types"`
	QuestionMode    string   `json:"questionMode,omitempty"`
}

type question struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Context      string   `json:"context,omitempty"`
	Evidence     string   `json:"evidence,omitempty"`
	ErrorTag     string   `json:"errorTag,omitempty"`
	ReviewKey    string   `json:"reviewKey,omitempty"`
	Prompt       string   `json:"prompt"`
	Choices      []string `json:"choices"`
	CorrectIndex int      `json:"correctIndex"`
	Explanation  string   `json:"explanation"`
	LearningTip  string   `json:"learningTip"`
	IELTSSkill   string   `json:"ieltsSkill"`
}

type publicQuestion struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"`
	Context    string   `json:"context,omitempty"`
	Prompt     string   `json:"prompt"`
	Choices    []string `json:"choices"`
	IELTSSkill string   `json:"ieltsSkill"`
}

type session struct {
	ID              string           `json:"id"`
	CreatedAt       time.Time        `json:"createdAt"`
	UpdatedAt       time.Time        `json:"updatedAt"`
	CompletedAt     *time.Time       `json:"completedAt,omitempty"`
	Level           string           `json:"level"`
	IELTSTarget     float64          `json:"ieltsTarget"`
	DurationMinutes int              `json:"durationMinutes"`
	Types           []string         `json:"types"`
	Source          string           `json:"source"`
	Status          string           `json:"status"`
	Score           *int             `json:"score,omitempty"`
	Questions       []question       `json:"-"`
	Answers         map[int]int      `json:"answers"`
	PublicQuestions []publicQuestion `json:"questions"`
}

type sessionSummary struct {
	ID            string     `json:"id"`
	Level         string     `json:"level"`
	Status        string     `json:"status"`
	Source        string     `json:"source"`
	CreatedAt     time.Time  `json:"createdAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	IELTSTarget   float64    `json:"ieltsTarget"`
	QuestionCount int        `json:"questionCount"`
	AnsweredCount int        `json:"answeredCount"`
	Score         *int       `json:"score,omitempty"`
}

type reviewItem struct {
	Question      publicQuestion `json:"question"`
	SelectedIndex *int           `json:"selectedIndex,omitempty"`
	CorrectIndex  int            `json:"correctIndex"`
	IsCorrect     bool           `json:"isCorrect"`
	Explanation   string         `json:"explanation"`
	LearningTip   string         `json:"learningTip"`
	Evidence      string         `json:"evidence,omitempty"`
	ErrorTag      string         `json:"errorTag,omitempty"`
}

type sessionResponse struct {
	Session session      `json:"session"`
	Review  []reviewItem `json:"review,omitempty"`
	Notice  string       `json:"notice,omitempty"`
}

func main() {
	loadDotEnv(".env")
	cfg := loadConfig()
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	var err error
	switch command {
	case "serve":
		err = runServer(cfg)
	case "migrate":
		err = runMigration(cfg)
	case "seed":
		if len(os.Args) > 2 {
			if os.Args[2] != "--fresh" || len(os.Args) > 3 {
				err = errors.New("opsi seed tidak dikenal; gunakan `seed` atau `seed --fresh`")
				break
			}
			_ = os.Setenv("RESEED_LOCAL_BANK", "1")
		}
		err = runSeed(cfg)
	case "seed-cefr":
		err = runCEFRSeed(cfg)
	case "audit-cefr":
		err = runCEFRAudit(cfg)
	case "seed-ielts":
		err = runIELTSTargetedSeed(cfg)
	case "audit-ielts":
		err = runIELTSTargetedAudit(cfg)
	case "pilot-ai-ielts":
		err = runAIOriginalSeed(cfg, true)
	case "seed-ai-ielts":
		err = runAIOriginalSeed(cfg, false)
	case "audit-ai-ielts":
		err = runAIOriginalAudit(cfg)
	default:
		err = fmt.Errorf("perintah %q tidak dikenal; gunakan serve, migrate, seed, seed-cefr, audit-cefr, seed-ielts, audit-ielts, pilot-ai-ielts, seed-ai-ielts, atau audit-ai-ielts", command)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func runServer(cfg config) error {
	db, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	checkCtx, cancelCheck := context.WithTimeout(context.Background(), 5*time.Second)
	err = ensureSchemaCurrent(checkCtx, db)
	cancelCheck()
	if err != nil {
		return err
	}

	a := &app{db: db, cfg: cfg, client: &http.Client{Timeout: 65 * time.Second}, hits: map[string][]time.Time{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", a.health)
	mux.HandleFunc("/api/auth/register", a.register)
	mux.HandleFunc("/api/auth/login", a.login)
	mux.HandleFunc("/api/auth/logout", a.logout)
	mux.Handle("/api/auth/me", a.requireAuth(http.HandlerFunc(a.me)))
	mux.Handle("/api/dashboard", a.requireAuth(http.HandlerFunc(a.dashboard)))
	mux.Handle("/api/reviews", a.requireAuth(http.HandlerFunc(a.reviews)))
	mux.Handle("/api/reviews/session", a.requireAuth(http.HandlerFunc(a.reviewSession)))
	mux.Handle("/api/vocabulary", a.requireAuth(http.HandlerFunc(a.vocabulary)))
	mux.Handle("/api/vocabulary/", a.requireAuth(http.HandlerFunc(a.vocabularyByID)))
	mux.Handle("/api/writing/prompt", a.requireAuth(http.HandlerFunc(a.writingPrompt)))
	mux.Handle("/api/writing/prompts", a.requireAuth(http.HandlerFunc(a.writingPromptsList)))
	mux.Handle("/api/writing/revision", a.requireAuth(http.HandlerFunc(a.submitWritingRevision)))
	mux.Handle("/api/writing", a.requireAuth(http.HandlerFunc(a.writing)))
	mux.Handle("/api/mock-tests", a.requireAuth(http.HandlerFunc(a.mockTests)))
	mux.Handle("/api/mock-tests/", a.requireAuth(http.HandlerFunc(a.mockTestByID)))
	mux.Handle("/api/diagnostic", a.requireAuth(http.HandlerFunc(a.diagnostic)))
	mux.Handle("/api/reflections/", a.requireAuth(http.HandlerFunc(a.saveReflection)))
	mux.Handle("/api/sessions", a.requireAuth(http.HandlerFunc(a.sessions)))
	mux.Handle("/api/sessions/", a.requireAuth(http.HandlerFunc(a.sessionByID)))
	mux.Handle("/api/question-bank", a.requireAdmin(http.HandlerFunc(a.questionBank)))
	mux.Handle("/api/question-bank/generate", a.requireAdmin(http.HandlerFunc(a.generateQuestionBank)))
	mux.Handle("/api/admin/questions", a.requireAdmin(http.HandlerFunc(a.adminQuestions)))
	adminPage := a.requireAdmin(http.HandlerFunc(a.serveAdminPage))
	mux.Handle("/admin", adminPage)
	mux.Handle("/admin.html", adminPage)
	mux.HandleFunc("/error", a.serveGenericErrorPage)
	mux.HandleFunc("/error.html", a.serveGenericErrorPage)
	assets, _ := fs.Sub(webFiles, "web")
	mux.Handle("/", http.FileServer(http.FS(assets)))

	srv := &http.Server{Addr: cfg.Addr, Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 75 * time.Second, WriteTimeout: 75 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("IELTS practice listening on %s", cfg.Addr)
	return srv.ListenAndServe()
}

func runMigration(cfg config) error {
	db, err := openDBForMigration(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := migrate(ctx, db); err != nil {
		return err
	}
	log.Printf("database migration completed: %s", schemaVersion)
	return nil
}

func runSeed(cfg config) error {
	db, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	if err := ensureSchemaCurrent(ctx, db); err != nil {
		return err
	}
	if err := seedCodexQuestionBank(ctx, db); err != nil {
		return err
	}
	cells, total, minimum, maximum, err := localQuestionBankSummary(ctx, db)
	if err != nil {
		return err
	}
	log.Printf("question bank seed completed: cells=%d total=%d min_per_cell=%d max_per_cell=%d", cells, total, minimum, maximum)
	return nil
}

func runCEFRSeed(cfg config) error {
	db, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	if err := ensureSchemaCurrent(ctx, db); err != nil {
		return err
	}
	dataDir := env("SEED_DATA_DIR", filepath.Join("data", "questions"))
	replacements, err := seedCEFRQuestionBank(ctx, db, dataDir)
	if err != nil {
		return err
	}
	cells, total, minimum, maximum, err := cefrQuestionBankSummary(ctx, db)
	if err != nil {
		return err
	}
	log.Printf("CEFR seed completed: cells=%d total=%d min_per_cell=%d max_per_cell=%d replacements_for_existing=%d new_questions_added=12000 normalized_context_duplicates=0", cells, total, minimum, maximum, replacements)
	return nil
}

func runCEFRAudit(cfg config) error {
	db, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := ensureSchemaCurrent(ctx, db); err != nil {
		return err
	}
	if err := verifyCEFRSeed(ctx, db); err != nil {
		return err
	}
	cells, total, minimum, maximum, err := cefrQuestionBankSummary(ctx, db)
	if err != nil {
		return err
	}
	log.Printf("CEFR audit passed: cells=%d total=%d min_per_cell=%d max_per_cell=%d missing_explanations=0 missing_learning_tips=0 normalized_context_duplicates=0", cells, total, minimum, maximum)
	return nil
}

func runIELTSTargetedSeed(cfg config) error {
	db, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	if err := ensureSchemaCurrent(ctx, db); err != nil {
		return err
	}
	if err := seedIELTSTargetedQuestionBank(ctx, db); err != nil {
		return err
	}
	log.Printf("IELTS targeted seed completed: levels=A1-C2 targets=5.0-9.0 types=6 cells=324 total=324000 per_cell=1000 normalized_context_duplicates=0")
	return nil
}

func runIELTSTargetedAudit(cfg config) error {
	db, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := ensureSchemaCurrent(ctx, db); err != nil {
		return err
	}
	if err := verifyIELTSTargetedQuestionBank(ctx, db); err != nil {
		return err
	}
	log.Printf("IELTS targeted audit passed: cells=324 total=324000 per_cell=1000 missing_explanations=0 normalized_context_duplicates=0")
	return nil
}

func runAIOriginalSeed(cfg config, pilot bool) error {
	db, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	timeout := 7 * 24 * time.Hour
	if pilot {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := ensureSchemaCurrent(ctx, db); err != nil {
		return err
	}
	a := &app{db: db, cfg: cfg, client: &http.Client{Timeout: 120 * time.Second}, hits: map[string][]time.Time{}}
	if err := seedAIOriginalQuestions(ctx, a, pilot); err != nil {
		return err
	}
	if pilot {
		log.Printf("AI original pilot completed: B1 band 5.0, one independently generated question per type")
		return nil
	}
	log.Printf("Codex local original seed completed: cells=216 total=216000 per_cell=1000 source=authentic_curated")
	return nil
}

func runAIOriginalAudit(cfg config) error {
	db, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := ensureSchemaCurrent(ctx, db); err != nil {
		return err
	}
	if err := verifyAIOriginalQuestions(ctx, db); err != nil {
		return err
	}
	log.Printf("Codex local original audit passed: cells=216 total=216000 per_cell=1000 source=authentic_curated normalized_context_duplicates=0")
	return nil
}

func (a *app) serveAdminPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	page, err := webFiles.ReadFile("web/admin.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Halaman admin belum dapat dibuka.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "admin.html", time.Time{}, bytes.NewReader(page))
}

func (a *app) serveGenericErrorPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	status := http.StatusForbidden
	if s, err := strconv.Atoi(r.URL.Query().Get("status")); err == nil && s >= 400 && s <= 599 {
		status = s
	} else if s, err := strconv.Atoi(r.URL.Query().Get("code")); err == nil && s >= 400 && s <= 599 {
		status = s
	}
	message := r.URL.Query().Get("message")
	if message == "" {
		message = r.URL.Query().Get("error")
	}
	if message == "" {
		message = "Fitur ini hanya tersedia untuk admin."
	}
	title := r.URL.Query().Get("title")
	if title == "" {
		if status == http.StatusForbidden {
			title = "Fitur Khusus Admin"
		} else if status == http.StatusUnauthorized {
			title = "Sesi Belum Aktif"
		} else {
			title = "Terjadi Kendala"
		}
	}
	badge := r.URL.Query().Get("badge")
	if badge == "" {
		badge = fmt.Sprintf("%d · %s", status, http.StatusText(status))
	}
	detail := r.URL.Query().Get("detail")
	if detail == "" {
		if status == http.StatusForbidden {
			detail = "Halaman dan katalog bank soal ini hanya dapat diakses oleh akun dengan peran Administrator. Akun Anda saat ini berstatus Learner (Pelajar)."
		} else {
			detail = "Silakan kembali ke beranda atau masuk ulang ke akun Anda."
		}
	}
	a.serveErrorPage(w, r, status, badge, title, message, detail)
}

func (a *app) serveErrorPage(w http.ResponseWriter, r *http.Request, statusCode int, badge, title, message, detail string) {
	page, err := webFiles.ReadFile("web/error.html")
	if err != nil {
		writeError(w, statusCode, message)
		return
	}
	html := string(page)
	html = strings.ReplaceAll(html, "{{STATUS_CODE}}", strconv.Itoa(statusCode))
	html = strings.ReplaceAll(html, "{{STATUS_BADGE}}", badge)
	html = strings.ReplaceAll(html, "{{TITLE}}", title)
	html = strings.ReplaceAll(html, "{{MESSAGE}}", message)
	html = strings.ReplaceAll(html, "{{DETAIL}}", detail)
	reqPath := r.URL.Path
	if reqPath == "" || reqPath == "/error" || reqPath == "/error.html" {
		reqPath = "/admin"
	}
	html = strings.ReplaceAll(html, "{{REQUEST_PATH}}", reqPath)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(html))
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || os.Getenv(key) != "" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		_ = os.Setenv(key, value)
	}
}

func loadConfig() config {
	return config{
		Addr: env("APP_ADDR", ":8080"), DBHost: env("DB_HOST", "127.0.0.1"), DBPort: env("DB_PORT", "3306"),
		DBUser: os.Getenv("DB_USER"), DBPassword: os.Getenv("DB_PASSWORD"), DBName: env("DB_NAME", "english_practice"),
		DeepSeekKey: os.Getenv("DEEPSEEK_API_KEY"), DeepSeekBaseURL: strings.TrimRight(env("DEEPSEEK_BASE_URL", "https://api.deepseek.com"), "/"), DeepSeekModel: env("DEEPSEEK_MODEL", "deepseek-chat"),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func openDB(cfg config) (*sql.DB, error) {
	driverConfig, err := databaseDriverConfig(cfg)
	if err != nil {
		return nil, err
	}
	driverConfig.DBName = cfg.DBName
	db, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		return nil, err
	}
	configureDBPool(db)
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 20*time.Second)
	err = db.PingContext(pingCtx)
	cancelPing()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("menghubungkan MySQL database %s: %w; jalankan migrate jika database belum dibuat", cfg.DBName, err)
	}
	return db, nil
}

func databaseDriverConfig(cfg config) (*mysql.Config, error) {
	if cfg.DBUser == "" || cfg.DBPassword == "" {
		return nil, errors.New("DB_USER dan DB_PASSWORD wajib diisi")
	}
	if !safeDBName.MatchString(cfg.DBName) {
		return nil, errors.New("DB_NAME hanya boleh berisi huruf, angka, dan underscore")
	}
	driverConfig := mysql.NewConfig()
	driverConfig.User = cfg.DBUser
	driverConfig.Passwd = cfg.DBPassword
	driverConfig.Net = "tcp"
	driverConfig.Addr = net.JoinHostPort(cfg.DBHost, cfg.DBPort)
	driverConfig.ParseTime = true
	driverConfig.Collation = "utf8mb4_unicode_ci"
	driverConfig.Timeout = 30 * time.Second
	driverConfig.ReadTimeout = 300 * time.Second
	driverConfig.WriteTimeout = 300 * time.Second
	return driverConfig, nil
}

func openDBForMigration(cfg config) (*sql.DB, error) {
	driverConfig, err := databaseDriverConfig(cfg)
	if err != nil {
		return nil, err
	}
	admin, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		return nil, err
	}
	defer admin.Close()
	adminCtx, cancelAdmin := context.WithTimeout(context.Background(), 20*time.Second)
	_, err = admin.ExecContext(adminCtx, "CREATE DATABASE IF NOT EXISTS `"+cfg.DBName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	cancelAdmin()
	if err != nil {
		return nil, fmt.Errorf("membuat database %s: %w", cfg.DBName, err)
	}
	return openDB(cfg)
}

func configureDBPool(db *sql.DB) {
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)
}

func ensureSchemaCurrent(ctx context.Context, db *sql.DB) error {
	var current int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_data_migrations WHERE name=?`, schemaVersion).Scan(&current); err != nil || current != 1 {
		log.Printf("Menjalankan migrasi database otomatis untuk versi: %s", schemaVersion)
		if err := migrate(ctx, db); err != nil {
			return fmt.Errorf("migrasi database otomatis gagal: %w; jalankan `go run . migrate`", err)
		}
	}
	return nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(80) NOT NULL, email VARCHAR(190) NOT NULL,
			password_hash VARCHAR(255) NOT NULL, role VARCHAR(20) NOT NULL DEFAULT 'learner',
			created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL, UNIQUE KEY uq_users_email (email)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS auth_sessions (
			token_hash CHAR(64) PRIMARY KEY, user_id BIGINT NOT NULL, expires_at DATETIME(6) NOT NULL,
			created_at DATETIME(6) NOT NULL, INDEX idx_auth_user (user_id), INDEX idx_auth_expiry (expires_at),
			CONSTRAINT fk_auth_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS question_bank (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, content_hash CHAR(64) NOT NULL, level VARCHAR(4) NOT NULL,
			ielts_target DECIMAL(2,1) NOT NULL DEFAULT 6.5, type VARCHAR(30) NOT NULL, source VARCHAR(20) NOT NULL, question_json JSON NOT NULL,
			active BOOLEAN NOT NULL DEFAULT TRUE, created_at DATETIME(6) NOT NULL,
			UNIQUE KEY uq_question_content (content_hash), INDEX idx_question_pick (active,level,ielts_target,type)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_question_history (
			user_id BIGINT NOT NULL, question_key CHAR(64) NOT NULL, session_id VARCHAR(36) NOT NULL,
			first_seen_at DATETIME(6) NOT NULL,
			PRIMARY KEY (user_id,question_key), INDEX idx_question_history_session (session_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS app_data_migrations (
			name VARCHAR(100) PRIMARY KEY, completed_at DATETIME(6) NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS practice_sessions (
			id VARCHAR(36) PRIMARY KEY, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
			completed_at DATETIME(6) NULL, level VARCHAR(4) NOT NULL, ielts_target DECIMAL(2,1) NOT NULL,
			duration_minutes INT NOT NULL, question_types JSON NOT NULL, source VARCHAR(20) NOT NULL,
			status VARCHAR(20) NOT NULL, score INT NULL, questions_json JSON NOT NULL,
			INDEX idx_sessions_created (created_at), INDEX idx_sessions_status (status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS practice_answers (
			session_id VARCHAR(36) NOT NULL, question_index INT NOT NULL, selected_index INT NOT NULL,
			answered_at DATETIME(6) NOT NULL, PRIMARY KEY (session_id, question_index),
			CONSTRAINT fk_answer_session FOREIGN KEY (session_id) REFERENCES practice_sessions(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS review_cards (
			card_key CHAR(64) PRIMARY KEY, question_json JSON NOT NULL, mastery INT NOT NULL DEFAULT 0,
			interval_days INT NOT NULL DEFAULT 1, next_review_at DATETIME(6) NOT NULL,
			last_correct BOOLEAN NOT NULL DEFAULT FALSE, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
			INDEX idx_review_due (next_review_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS vocabulary_cards (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, word VARCHAR(160) NOT NULL, meaning TEXT NOT NULL,
			example TEXT NOT NULL, collocation VARCHAR(255) NOT NULL DEFAULT '', mastery INT NOT NULL DEFAULT 0,
			next_review_at DATETIME(6) NOT NULL, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
			UNIQUE KEY uq_vocabulary_word (word), INDEX idx_vocabulary_due (next_review_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS writing_submissions (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, task_type VARCHAR(20) NOT NULL, prompt TEXT NOT NULL,
			response_text MEDIUMTEXT NOT NULL, word_count INT NOT NULL, overall_band DECIMAL(2,1) NOT NULL,
			feedback_json JSON NOT NULL, source VARCHAR(20) NOT NULL, created_at DATETIME(6) NOT NULL,
			INDEX idx_writing_created (created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS reading_reflections (
			session_id VARCHAR(36) NOT NULL, question_index INT NOT NULL, reason VARCHAR(30) NOT NULL,
			created_at DATETIME(6) NOT NULL, PRIMARY KEY (session_id, question_index),
			CONSTRAINT fk_reflection_session FOREIGN KEY (session_id) REFERENCES practice_sessions(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS mock_test_sessions (
			id VARCHAR(36) PRIMARY KEY, user_id BIGINT NOT NULL, title VARCHAR(255) NOT NULL,
			module_type VARCHAR(20) NOT NULL DEFAULT 'academic', status VARCHAR(20) NOT NULL DEFAULT 'in_progress',
			current_section VARCHAR(20) NOT NULL DEFAULT 'listening', started_at DATETIME(6) NOT NULL,
			completed_at DATETIME(6) NULL, time_remaining_seconds INT NOT NULL DEFAULT 9600,
			config_json JSON NOT NULL, questions_json JSON NOT NULL, answers_json JSON NOT NULL,
			section_scores_json JSON NOT NULL, overall_band DECIMAL(2,1) NULL,
			created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
			INDEX idx_mock_user (user_id), INDEX idx_mock_status (status), INDEX idx_mock_created (created_at),
			CONSTRAINT fk_mock_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS writing_revisions (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, submission_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
			revision_number INT NOT NULL DEFAULT 1, revised_text MEDIUMTEXT NOT NULL, word_count INT NOT NULL,
			overall_band DECIMAL(2,1) NOT NULL, feedback_json JSON NOT NULL, diff_json JSON NOT NULL,
			created_at DATETIME(6) NOT NULL,
			INDEX idx_rev_sub (submission_id), INDEX idx_rev_user (user_id),
			CONSTRAINT fk_rev_sub FOREIGN KEY (submission_id) REFERENCES writing_submissions(id) ON DELETE CASCADE,
			CONSTRAINT fk_rev_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}
	var targetColumn int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='question_bank' AND column_name='ielts_target'`).Scan(&targetColumn); err != nil {
		return fmt.Errorf("question bank schema check: %w", err)
	}
	if targetColumn == 0 {
		if _, err := db.ExecContext(ctx, `ALTER TABLE question_bank ADD COLUMN ielts_target DECIMAL(2,1) NOT NULL DEFAULT 6.5 AFTER level`); err != nil {
			return fmt.Errorf("question bank target migration: %w", err)
		}
	}
	lookupIndexExists, err := indexExists(ctx, db, "question_bank", "idx_question_bank_lookup")
	if err != nil {
		return fmt.Errorf("question bank lookup index check: %w", err)
	}
	if !lookupIndexExists {
		if _, err := db.ExecContext(ctx, `ALTER TABLE question_bank ADD INDEX idx_question_bank_lookup (active,level,ielts_target,type)`); err != nil {
			return fmt.Errorf("question bank lookup index migration: %w", err)
		}
	}
	performanceIndexes := []struct {
		name    string
		columns string
	}{
		{name: "idx_question_admin_created", columns: "created_at,id"},
		{name: "idx_question_admin_active_created", columns: "active,created_at,id"},
		{name: "idx_question_source", columns: "source"},
		{name: "idx_question_admin_filter", columns: "active,level,ielts_target,type,source,created_at,id"},
		{name: "idx_question_admin_filter_all", columns: "level,ielts_target,type,source,created_at,id"},
	}
	for _, item := range performanceIndexes {
		exists, err := indexExists(ctx, db, "question_bank", item.name)
		if err != nil {
			return fmt.Errorf("question bank performance index %s check: %w", item.name, err)
		}
		if !exists {
			if _, err := db.ExecContext(ctx, `ALTER TABLE question_bank ADD INDEX `+item.name+` (`+item.columns+`)`); err != nil {
				return fmt.Errorf("question bank performance index %s migration: %w", item.name, err)
			}
		}
	}
	if err := ensureOwnershipSchema(ctx, db); err != nil {
		return err
	}
	if err := backfillUserQuestionHistory(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO app_data_migrations (name,completed_at) VALUES (?,?) ON DUPLICATE KEY UPDATE completed_at=VALUES(completed_at)`, schemaVersion, time.Now().UTC()); err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	return nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; script-src 'self'; connect-src 'self'; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}

func (a *app) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.PingContext(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "Database belum siap.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) sessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.listSessions(w, r)
	case http.MethodPost:
		a.createSession(w, r)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (a *app) sessionByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodGet {
		a.getSession(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "complete" && r.Method == http.MethodPost {
		a.completeSession(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "retry" && r.Method == http.MethodPost {
		a.retrySession(w, r, parts[0])
		return
	}
	if len(parts) == 3 && parts[1] == "answers" && r.Method == http.MethodPut {
		index, err := strconv.Atoi(parts[2])
		if err != nil {
			writeError(w, 400, "Nomor soal tidak valid.")
			return
		}
		a.saveAnswer(w, r, parts[0], index)
		return
	}
	http.NotFound(w, r)
}

func (a *app) createSession(w http.ResponseWriter, r *http.Request) {
	if !a.allow(r) {
		writeError(w, http.StatusTooManyRequests, "Terlalu banyak permintaan. Tunggu satu menit lalu coba lagi.")
		return
	}
	var input generateInput
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	if message := validateInput(input); message != "" {
		writeError(w, 422, message)
		return
	}

	var questions []question
	var source, notice string
	var err error
	var sessionTx *sql.Tx
	uid := userID(r.Context())
	switch normalizedQuestionMode(input.QuestionMode) {
	case "deepseek":
		if a.cfg.DeepSeekKey == "" {
			writeError(w, http.StatusServiceUnavailable, "DeepSeek belum dikonfigurasi. Pilih bank soal atau minta admin memasang API key.")
			return
		}
		questions, source, err = a.generateQuestions(r.Context(), input)
		if err != nil {
			log.Printf("generate learner session: %v", err)
			writeError(w, http.StatusBadGateway, "Soal baru belum dapat dibuat oleh DeepSeek. Coba lagi atau pilih bank soal.")
			return
		}
		added, insertErr := insertBankQuestions(r.Context(), a.db, input.Level, input.IELTSTarget, source, questions)
		if insertErr != nil {
			log.Printf("store generated learner questions: %v", insertErr)
			writeError(w, http.StatusInternalServerError, "Soal baru berhasil dibuat tetapi belum dapat disimpan ke bank.")
			return
		}
		if err := dedupeQuestionBank(r.Context(), a.db); err != nil {
			log.Printf("dedupe generated question bank: %v", err)
		}
		notice = fmt.Sprintf("%d soal dibuat dengan DeepSeek; %d soal baru disimpan ke bank.", len(questions), added)
	default:
		questions, err = a.selectBankQuestions(r.Context(), a.db, uid, input)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		source = "question_bank"
		notice = "Soal diacak dari bank soal tersimpan."
	}
	if source != "question_bank" {
		shuffleQuestions(questions)
	}
	assignReviewKeys(questions)
	now := time.Now().UTC()
	id := newID()
	qJSON, _ := json.Marshal(questions)
	typeJSON, _ := json.Marshal(input.Types)
	if sessionTx == nil {
		sessionTx, err = a.db.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, 500, "Sesi belum dapat disimpan.")
			return
		}
	}
	defer sessionTx.Rollback()
	_, err = sessionTx.ExecContext(r.Context(), `INSERT INTO practice_sessions
		(id, user_id, created_at, updated_at, level, ielts_target, duration_minutes, question_types, source, status, questions_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'in_progress', ?)`, id, uid, now, now, input.Level, input.IELTSTarget, input.DurationMinutes, typeJSON, source, qJSON)
	if err != nil {
		log.Printf("insert session: %v", err)
		writeError(w, 500, "Sesi belum dapat disimpan.")
		return
	}
	if err = recordUserQuestionHistory(r.Context(), sessionTx, uid, id, input.Level, input.IELTSTarget, questions, now); err != nil {
		log.Printf("record question history: %v", err)
		writeError(w, 500, "Riwayat soal belum dapat disimpan.")
		return
	}
	if err = sessionTx.Commit(); err != nil {
		log.Printf("commit session: %v", err)
		writeError(w, 500, "Sesi belum dapat disimpan.")
		return
	}
	s := session{ID: id, CreatedAt: now, UpdatedAt: now, Level: input.Level, IELTSTarget: input.IELTSTarget, DurationMinutes: input.DurationMinutes, Types: input.Types, Source: source, Status: "in_progress", Questions: questions, Answers: map[int]int{}}
	s.PublicQuestions = publicQuestions(questions)
	writeJSON(w, http.StatusCreated, sessionResponse{Session: s, Notice: notice})
}

func normalizedQuestionMode(mode string) string {
	if mode == "" {
		return "bank"
	}
	return mode
}

func validateInput(in generateInput) string {
	if !slices.Contains([]string{"A1", "A2", "B1", "B2", "C1", "C2"}, in.Level) {
		return "Pilih level A1 sampai C2."
	}
	if in.IELTSTarget < 5 || in.IELTSTarget > 9 || float64(int(in.IELTSTarget*2)) != in.IELTSTarget*2 {
		return "Target IELTS harus 5.0–9.0 dalam kelipatan 0.5."
	}
	if in.Count < 5 || in.Count > 500 {
		return "Jumlah soal harus 5–500."
	}
	if in.DurationMinutes < 5 || in.DurationMinutes > 60 {
		return "Waktu harus 5–60 menit."
	}
	if !slices.Contains([]string{"bank", "deepseek"}, normalizedQuestionMode(in.QuestionMode)) {
		return "Sumber soal tidak dikenal."
	}
	allowed := []string{"grammar", "vocabulary", "reading", "fill_blank", "listening", "error_identification"}
	if len(in.Types) == 0 {
		return "Pilih minimal satu tipe latihan."
	}
	seen := make(map[string]struct{}, len(in.Types))
	for _, typ := range in.Types {
		if !slices.Contains(allowed, typ) {
			return "Tipe latihan tidak dikenal."
		}
		if _, exists := seen[typ]; exists {
			return "Tipe latihan tidak boleh dipilih lebih dari sekali."
		}
		seen[typ] = struct{}{}
	}
	return ""
}

func (a *app) saveAnswer(w http.ResponseWriter, r *http.Request, id string, index int) {
	var body struct {
		SelectedIndex int `json:"selectedIndex"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	s, err := a.loadSession(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Sesi tidak ditemukan.")
		return
	}
	if err != nil {
		writeError(w, 500, "Sesi belum dapat dibuka.")
		return
	}
	if s.Status != "in_progress" {
		writeError(w, 409, "Sesi ini sudah selesai.")
		return
	}
	if time.Now().After(s.CreatedAt.Add(time.Duration(s.DurationMinutes) * time.Minute)) {
		writeError(w, 410, "Waktu sesi sudah habis. Nilai latihan untuk melihat hasil.")
		return
	}
	if index < 0 || index >= len(s.Questions) || body.SelectedIndex < 0 || body.SelectedIndex >= len(s.Questions[index].Choices) {
		writeError(w, 422, "Pilihan jawaban tidak valid.")
		return
	}
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO practice_answers (session_id, question_index, selected_index, answered_at)
		VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE selected_index=VALUES(selected_index), answered_at=VALUES(answered_at)`, id, index, body.SelectedIndex, time.Now().UTC())
	if err != nil {
		writeError(w, 500, "Jawaban belum tersimpan. Coba sekali lagi.")
		return
	}
	writeJSON(w, 200, map[string]any{"saved": true, "answeredCount": len(s.Answers) + boolIntMap(s.Answers, index)})
}

func boolIntMap(m map[int]int, key int) int {
	if _, ok := m[key]; ok {
		return 0
	}
	return 1
}

func (a *app) completeSession(w http.ResponseWriter, r *http.Request, id string) {
	s, err := a.loadSession(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Sesi tidak ditemukan.")
		return
	}
	if err != nil {
		writeError(w, 500, "Sesi belum dapat dibuka.")
		return
	}
	if s.Status == "completed" {
		writeJSON(w, 200, makeResponse(s))
		return
	}
	correct := 0
	for i, q := range s.Questions {
		if selected, ok := s.Answers[i]; ok && selected == q.CorrectIndex {
			correct++
		}
	}
	score := int(float64(correct)*100/float64(len(s.Questions)) + .5)
	now := time.Now().UTC()
	_, err = a.db.ExecContext(r.Context(), `UPDATE practice_sessions SET status='completed', score=?, completed_at=?, updated_at=? WHERE id=? AND user_id=?`, score, now, now, id, userID(r.Context()))
	if err != nil {
		writeError(w, 500, "Nilai belum dapat disimpan.")
		return
	}
	s.Status = "completed"
	s.Score = &score
	s.CompletedAt = &now
	if !a.skipReviewSchedule {
		if err := a.updateReviewSchedule(r.Context(), s, now); err != nil {
			log.Printf("update review schedule: %v", err)
		}
	}
	writeJSON(w, 200, makeResponse(s))
}

func (a *app) retrySession(w http.ResponseWriter, r *http.Request, id string) {
	if !a.allow(r) {
		writeError(w, http.StatusTooManyRequests, "Terlalu banyak permintaan. Tunggu satu menit lalu coba lagi.")
		return
	}
	original, err := a.loadSession(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Sesi tidak ditemukan.")
		return
	}
	if err != nil {
		writeError(w, 500, "Sesi belum dapat dibuka.")
		return
	}
	if original.Status != "completed" {
		writeError(w, 409, "Selesaikan sesi sebelum membuat latihan ulang.")
		return
	}

	questions := retryQuestions(original)
	if len(questions) == 0 {
		writeError(w, 409, "Semua jawaban sudah benar. Tidak ada soal yang perlu diulang.")
		return
	}
	types := questionTypes(questions)
	duration := max(5, min(60, len(questions)*2))
	now := time.Now().UTC()
	newSession := session{
		ID: newID(), CreatedAt: now, UpdatedAt: now, Level: original.Level,
		IELTSTarget: original.IELTSTarget, DurationMinutes: duration, Types: types,
		Source: "review", Status: "in_progress", Questions: questions, Answers: map[int]int{},
	}
	newSession.PublicQuestions = publicQuestions(questions)
	qJSON, _ := json.Marshal(questions)
	typeJSON, _ := json.Marshal(types)
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO practice_sessions
		(id, user_id, created_at, updated_at, level, ielts_target, duration_minutes, question_types, source, status, questions_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'review', 'in_progress', ?)`, newSession.ID, userID(r.Context()), now, now, newSession.Level, newSession.IELTSTarget, duration, typeJSON, qJSON)
	if err != nil {
		log.Printf("insert retry session: %v", err)
		writeError(w, 500, "Latihan ulang belum dapat dibuat.")
		return
	}
	writeJSON(w, http.StatusCreated, sessionResponse{
		Session: newSession,
		Notice:  fmt.Sprintf("Latihan ulang berisi %d soal yang belum dikuasai.", len(questions)),
	})
}

func retryQuestions(s session) []question {
	items := make([]question, 0, len(s.Questions))
	for index, q := range s.Questions {
		selected, answered := s.Answers[index]
		if !answered || selected != q.CorrectIndex {
			q.ID = fmt.Sprintf("retry-%d", len(items)+1)
			if q.ReviewKey == "" {
				q.ReviewKey = questionKey(q)
			}
			items = append(items, q)
		}
	}
	return items
}

func questionTypes(questions []question) []string {
	types := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, q := range questions {
		if _, exists := seen[q.Type]; exists {
			continue
		}
		seen[q.Type] = struct{}{}
		types = append(types, q.Type)
	}
	return types
}

func (a *app) listSessions(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT s.id, s.created_at, s.completed_at, s.level, s.ielts_target,
		JSON_LENGTH(s.questions_json), COUNT(a.question_index), s.status, s.source, s.score
		FROM practice_sessions s LEFT JOIN practice_answers a ON a.session_id=s.id
		WHERE s.user_id=? GROUP BY s.id ORDER BY s.created_at DESC LIMIT 50`, userID(r.Context()))
	if err != nil {
		writeError(w, 500, "Riwayat belum dapat dibuka.")
		return
	}
	defer rows.Close()
	items := []sessionSummary{}
	for rows.Next() {
		var x sessionSummary
		if err := rows.Scan(&x.ID, &x.CreatedAt, &x.CompletedAt, &x.Level, &x.IELTSTarget, &x.QuestionCount, &x.AnsweredCount, &x.Status, &x.Source, &x.Score); err != nil {
			continue
		}
		items = append(items, x)
	}
	writeJSON(w, 200, items)
}

func (a *app) getSession(w http.ResponseWriter, r *http.Request, id string) {
	s, err := a.loadSession(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Sesi tidak ditemukan.")
		return
	}
	if err != nil {
		writeError(w, 500, "Sesi belum dapat dibuka.")
		return
	}
	writeJSON(w, 200, makeResponse(s))
}

func (a *app) loadSession(ctx context.Context, id string) (session, error) {
	var s session
	var typesRaw, questionsRaw []byte
	err := a.db.QueryRowContext(ctx, `SELECT id,created_at,updated_at,completed_at,level,ielts_target,duration_minutes,question_types,source,status,score,questions_json FROM practice_sessions WHERE id=? AND user_id=?`, id, userID(ctx)).
		Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt, &s.CompletedAt, &s.Level, &s.IELTSTarget, &s.DurationMinutes, &typesRaw, &s.Source, &s.Status, &s.Score, &questionsRaw)
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(typesRaw, &s.Types); err != nil {
		return s, err
	}
	if err = json.Unmarshal(questionsRaw, &s.Questions); err != nil {
		return s, err
	}
	s.PublicQuestions = publicQuestions(s.Questions)
	s.Answers = map[int]int{}
	rows, err := a.db.QueryContext(ctx, `SELECT question_index,selected_index FROM practice_answers WHERE session_id=?`, id)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var i, v int
		if rows.Scan(&i, &v) == nil {
			s.Answers[i] = v
		}
	}
	return s, rows.Err()
}

func publicQuestions(items []question) []publicQuestion {
	out := make([]publicQuestion, len(items))
	for i, q := range items {
		out[i] = publicQuestion{ID: q.ID, Type: q.Type, Context: q.Context, Prompt: q.Prompt, Choices: q.Choices, IELTSSkill: q.IELTSSkill}
	}
	return out
}

func makeResponse(s session) sessionResponse {
	res := sessionResponse{Session: s}
	if s.Status != "completed" {
		return res
	}
	res.Review = make([]reviewItem, len(s.Questions))
	for i, q := range s.Questions {
		selected, ok := s.Answers[i]
		var selectedPtr *int
		if ok {
			value := selected
			selectedPtr = &value
		}
		res.Review[i] = reviewItem{Question: s.PublicQuestions[i], SelectedIndex: selectedPtr, CorrectIndex: q.CorrectIndex, IsCorrect: ok && selected == q.CorrectIndex, Explanation: q.Explanation, LearningTip: q.LearningTip, Evidence: q.Evidence, ErrorTag: q.ErrorTag}
	}
	return res
}

func (a *app) generateQuestions(ctx context.Context, in generateInput) ([]question, string, error) {
	if a.cfg.DeepSeekKey == "" {
		return demoQuestions(in), "demo", nil
	}
	allDomains := []string{
		"Marine Ecology and Coral Conservation", "Cognitive Neuroscience and Memory Retention",
		"Urban Planning and Public Infrastructure", "Renewable Energy and Microgrid Technology",
		"Behavioral Economics and Financial Psychology", "Computational Linguistics and AI Translation",
		"Archaeology and Material Cultural Preservation", "Biotechnology and Sustainable Agriculture",
		"Astrophysics and Planetary Climate Systems", "Public Health Epidemiology and Preventative Medicine",
		"Environmental Architecture and Passive Cooling", "Social Anthropology and Oral History Traditions",
	}
	rand.Shuffle(len(allDomains), func(i, j int) { allDomains[i], allDomains[j] = allDomains[j], allDomains[i] })
	selectedDomains := strings.Join(allDomains[:min(4, len(allDomains))], ", ")

	prompt := fmt.Sprintf(`Create exactly %d unique and original English-learning multiple-choice questions for an Indonesian learner targeting IELTS band %.1f (CEFR level %s).
Selected question types to balance: %s.
Incorporate varied topics from these academic domains: %s.

To prevent predictable patterns, adhere strictly to these diverse question archetypes:
1. Grammar: Vary across advanced syntactic patterns (e.g., negative inversion like 'Not only/Seldom', cleft sentences 'It was X that Y', mandative subjunctive, participle clauses 'Having + V3', inverted conditionals 'Had/Were/Should', modal deduction in the past 'must have/cannot have', double comparatives).
2. Vocabulary: Vary between contextual academic synonyms, formal antonyms, academic collocations (verb-noun, adj-noun), and register nuances. Avoid trivial single-word dictionary tests.
3. Reading: Provide a short original academic passage (80-140 words) in context. The prompt must test factual contradiction, indirect inference, cause-effect mechanisms, or author purpose. Crucially, the evidence field must contain an EXACT verbatim substring from context that proves the correct answer.
4. Fill-in-the-blank: Must contain _____ focusing on discourse connectors ('nevertheless', 'consequently', 'albeit'), dependent prepositions, or collocations.
5. Listening: Context must contain a complete, multi-turn dialogue (6–10 turns) with clear speaker labels (e.g. Student: and Professor:, or Researcher: and Lab Manager:). It must include realistic conversational self-repair or a timing/location/cost correction before revealing the final key detail. The prompt must test this detail without quoting the audio script in the question.

General rules:
- Provide 4 plausible choices (A, B, C, D) per question with realistic distractors.
- Distribute correctIndex across 0, 1, 2, and 3 (do not favor any single index).
- Explanation: In clear Indonesian, explaining why the correct choice is right and why common distractors are mistaken.
- LearningTip: Actionable, high-impact IELTS strategy tip in Indonesian.
- Do not copy official Cambridge IELTS exam questions.

Output ONLY valid JSON strictly matching:
{"questions":[{"id":"q1","type":"grammar|vocabulary|reading|fill_blank|listening|error_identification","context":"passage or dialogue if applicable","evidence":"exact supporting quote from context for reading, otherwise empty","errorTag":"grammar|vocabulary|detail|inference","prompt":"question text","choices":["A","B","C","D"],"correctIndex":0,"explanation":"Indonesian explanation","learningTip":"Indonesian tip","ieltsSkill":"Reading|Listening|Lexical Resource|Grammatical Range and Accuracy"}]}`, in.Count, in.IELTSTarget, in.Level, strings.Join(in.Types, ", "), selectedDomains)
	payload := map[string]any{"model": a.cfg.DeepSeekModel, "temperature": 0.75, "response_format": map[string]string{"type": "json_object"}, "messages": []map[string]string{{"role": "system", "content": "You are a master IELTS examiner and test creator creating diverse, high-quality, unpredictable practice exercises. Output valid JSON only."}, {"role": "user", "content": prompt}}}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.DeepSeekBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.DeepSeekKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("deepseek status=%d", resp.StatusCode)
		return nil, "", errors.New("deepseek request failed")
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &completion) != nil || len(completion.Choices) == 0 {
		return nil, "", errors.New("invalid completion")
	}
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	var result struct {
		Questions []question `json:"questions"`
	}
	if err = json.Unmarshal([]byte(strings.TrimSpace(content)), &result); err != nil {
		return nil, "", err
	}
	if err = validateQuestions(result.Questions, in.Count, in.Types); err != nil {
		return nil, "", err
	}
	return result.Questions, "deepseek", nil
}

func validateQuestions(items []question, count int, types []string) error {
	if len(items) != count {
		return errors.New("wrong question count")
	}
	seenPrompts := make(map[string]struct{}, len(items))
	for i, q := range items {
		if q.ID == "" {
			items[i].ID = fmt.Sprintf("q%d", i+1)
		}
		if !slices.Contains(types, q.Type) || q.Prompt == "" || len(q.Choices) != 4 || q.CorrectIndex < 0 || q.CorrectIndex > 3 || q.Explanation == "" || q.LearningTip == "" {
			return errors.New("invalid question")
		}
		if q.Type == "reading" && (q.Context == "" || q.Evidence == "" || !strings.Contains(strings.ToLower(q.Context), strings.ToLower(q.Evidence))) {
			return errors.New("reading question lacks exact evidence")
		}
		if q.Type == "listening" {
			turns := strings.Count(q.Context, ":")
			if len([]rune(strings.TrimSpace(q.Context))) < 60 || turns < 2 {
				return errors.New("listening question must contain a conversation")
			}
		}
		promptKey := strings.ToLower(strings.Join(strings.Fields(visiblePromptKey(q.Prompt)), " "))
		if q.Context != "" {
			promptKey += "|" + strings.ToLower(strings.Join(strings.Fields(q.Context), " "))
		}
		if _, exists := seenPrompts[promptKey]; exists {
			return errors.New("generated questions contain duplicate prompts")
		}
		seenPrompts[promptKey] = struct{}{}
	}
	return nil
}

func demoQuestions(in generateInput) []question {
	bank := []question{
		{ID: "g1", Type: "grammar", Prompt: "If I _____ more time, I would join an IELTS study group.", Choices: []string{"have", "had", "will have", "would have"}, CorrectIndex: 1, Explanation: "Kalimat ini memakai second conditional untuk situasi hipotetis: if + past simple, lalu would + verb.", LearningTip: "Kenali pola conditional dari pasangan tense di kedua klausa.", IELTSSkill: "Grammatical Range and Accuracy"},
		{ID: "g2", Type: "grammar", Prompt: "The report _____ by the research team before the deadline.", Choices: []string{"completed", "was completed", "has completing", "is complete"}, CorrectIndex: 1, Explanation: "Subjek ‘the report’ menerima tindakan, jadi bentuk passive voice past simple yang tepat adalah ‘was completed’.", LearningTip: "Cari siapa pelaku dan penerima tindakan sebelum memilih active atau passive voice.", IELTSSkill: "Grammatical Range and Accuracy"},
		{ID: "g3", Type: "grammar", Prompt: "Neither the lecturer nor the students _____ aware of the room change.", Choices: []string{"was", "were", "be", "has been"}, CorrectIndex: 1, Explanation: "Pada pola neither…nor, verb mengikuti subjek terdekat. ‘Students’ jamak sehingga memakai ‘were’.", LearningTip: "Untuk paired conjunction, cocokkan verb dengan noun terdekat.", IELTSSkill: "Grammatical Range and Accuracy"},
		{ID: "v1", Type: "vocabulary", Prompt: "Which word is closest in meaning to ‘substantial’ in an academic essay?", Choices: []string{"minor", "considerable", "temporary", "uncertain"}, CorrectIndex: 1, Explanation: "‘Substantial’ berarti cukup besar atau penting; ‘considerable’ memiliki makna akademik yang paling dekat.", LearningTip: "Pelajari sinonim bersama collocation, bukan sebagai kata tunggal.", IELTSSkill: "Lexical Resource"},
		{ID: "v2", Type: "vocabulary", Prompt: "Choose the most natural collocation: The policy could _____ a significant impact on public health.", Choices: []string{"do", "make", "have", "take"}, CorrectIndex: 2, Explanation: "Collocation bakunya adalah ‘have an impact’. Pilihan lain terdengar tidak alami dalam konteks ini.", LearningTip: "Catat academic collocations seperti ‘pose a risk’, ‘draw a conclusion’, dan ‘have an impact’.", IELTSSkill: "Lexical Resource"},
		{ID: "v3", Type: "vocabulary", Prompt: "In academic writing, ‘mitigate’ most nearly means…", Choices: []string{"make worse", "measure precisely", "make less severe", "remove completely"}, CorrectIndex: 2, Explanation: "‘Mitigate’ berarti mengurangi tingkat keparahan, bukan selalu menghilangkan masalah sepenuhnya.", LearningTip: "Perhatikan tingkat kekuatan kata; ‘mitigate’ lebih lemah daripada ‘eliminate’.", IELTSSkill: "Lexical Resource"},
		{ID: "r1", Type: "reading", Context: "A city introduced protected cycle lanes in 2022. Car journeys fell only slightly, but bicycle use doubled within a year. Local shops initially feared fewer customers; later surveys showed that foot traffic had increased.", Evidence: "later surveys showed that foot traffic had increased", ErrorTag: "detail", Prompt: "Which outcome was unexpected by local shops?", Choices: []string{"Car use disappeared", "Foot traffic increased", "Cycle lanes were removed", "Bicycle use declined"}, CorrectIndex: 1, Explanation: "Teks menyatakan toko awalnya khawatir pelanggan berkurang, tetapi survei justru menunjukkan foot traffic meningkat.", LearningTip: "Bedakan prediksi awal dan hasil akhir; kata ‘initially’ dan ‘later’ menandai kontras.", IELTSSkill: "Reading"},
		{ID: "r2", Type: "reading", Context: "A city introduced protected cycle lanes in 2022. Car journeys fell only slightly, but bicycle use doubled within a year. Local shops initially feared fewer customers; later surveys showed that foot traffic had increased.", Evidence: "Car journeys fell only slightly", ErrorTag: "vocabulary", Prompt: "The word ‘slightly’ indicates that the fall in car journeys was…", Choices: []string{"small", "sudden", "permanent", "unmeasured"}, CorrectIndex: 0, Explanation: "‘Slightly’ adalah adverb yang menunjukkan perubahan kecil, sehingga jawaban yang tepat adalah ‘small’.", LearningTip: "Pada vocabulary-in-context, ganti kata dengan pilihan lalu cek apakah makna kalimat tetap sama.", IELTSSkill: "Reading"},
		{ID: "r3", Type: "reading", Context: "Researchers studying sleep found that consistency mattered as much as duration. Participants with irregular bedtimes reported lower concentration even when their total weekly sleep matched that of regular sleepers.", Evidence: "Participants with irregular bedtimes reported lower concentration", ErrorTag: "inference", Prompt: "What does the study suggest?", Choices: []string{"Only sleep duration matters", "Regular timing supports concentration", "Weekly sleep cannot be measured", "Irregular sleepers sleep longer"}, CorrectIndex: 1, Explanation: "Gagasan utama menyebut konsistensi sama pentingnya dengan durasi dan jadwal tidak teratur terkait konsentrasi lebih rendah.", LearningTip: "Untuk main idea, pilih jawaban yang merangkum seluruh bukti, bukan satu detail.", IELTSSkill: "Reading"},
		{ID: "f1", Type: "fill_blank", Prompt: "Many researchers argue that access _____ education should be treated as a basic right.", Choices: []string{"at", "for", "to", "with"}, CorrectIndex: 2, Explanation: "Noun ‘access’ berpasangan dengan preposition ‘to’: access to education.", LearningTip: "Simpan noun–preposition pairs sebagai satu unit kosakata.", IELTSSkill: "Lexical Resource"},
		{ID: "f2", Type: "fill_blank", Prompt: "The number of remote workers has risen _____ over the past decade.", Choices: []string{"steadily", "steady", "steadiness", "steadier"}, CorrectIndex: 0, Explanation: "Kata yang menerangkan verb ‘has risen’ harus berupa adverb, yaitu ‘steadily’.", LearningTip: "Tentukan fungsi slot kosong sebelum melihat pilihan: noun, verb, adjective, atau adverb.", IELTSSkill: "Grammatical Range and Accuracy"},
		{ID: "f3", Type: "fill_blank", Prompt: "Although the proposal was costly, the committee decided to carry it _____.", Choices: []string{"on", "out", "over", "up"}, CorrectIndex: 1, Explanation: "Phrasal verb ‘carry out’ berarti melaksanakan suatu rencana atau tugas.", LearningTip: "Pelajari phrasal verbs di dalam kalimat formal agar tahu register penggunaannya.", IELTSSkill: "Lexical Resource"},
		{ID: "g4", Type: "grammar", Prompt: "By the time the lecture starts, we _____ our notes.", Choices: []string{"review", "reviewed", "will have reviewed", "are reviewing"}, CorrectIndex: 2, Explanation: "‘By the time’ dengan kejadian yang selesai sebelum titik waktu masa depan memakai future perfect: will have reviewed.", LearningTip: "Gunakan future perfect untuk menekankan tindakan yang sudah selesai sebelum waktu masa depan.", IELTSSkill: "Grammatical Range and Accuracy"},
		{ID: "g5", Type: "grammar", Prompt: "The new library, which _____ last year, now serves three neighbourhoods.", Choices: []string{"opens", "opened", "was opened", "has opening"}, CorrectIndex: 2, Explanation: "Library menerima tindakan dibuka, jadi passive voice past simple: was opened.", LearningTip: "Perhatikan relative clause dan apakah subjek melakukan atau menerima tindakan.", IELTSSkill: "Grammatical Range and Accuracy"},
		{ID: "g6", Type: "grammar", Prompt: "Students should submit the form _____ Friday afternoon.", Choices: []string{"at", "in", "by", "on"}, CorrectIndex: 2, Explanation: "‘By Friday afternoon’ berarti paling lambat sebelum atau pada waktu tersebut.", LearningTip: "Bedakan by (batas waktu) dan on (hari/tanggal tertentu).", IELTSSkill: "Grammatical Range and Accuracy"},
		{ID: "v4", Type: "vocabulary", Prompt: "The findings were considered _____ because they matched two independent studies.", Choices: []string{"reliable", "reluctant", "remote", "random"}, CorrectIndex: 0, Explanation: "Reliable berarti dapat dipercaya, sesuai dengan konteks temuan yang didukung studi lain.", LearningTip: "Gunakan petunjuk sebab-akibat dalam kalimat untuk menebak makna kata akademik.", IELTSSkill: "Lexical Resource"},
		{ID: "v5", Type: "vocabulary", Prompt: "Which phrase means ‘a gradual improvement’?", Choices: []string{"a sharp decline", "steady progress", "brief interruption", "limited access"}, CorrectIndex: 1, Explanation: "Steady progress berarti kemajuan yang berlangsung bertahap dan konsisten.", LearningTip: "Hafalkan pasangan adjective–noun yang sering muncul di laporan IELTS.", IELTSSkill: "Lexical Resource"},
		{ID: "r4", Type: "reading", Context: "A university introduced a ten-minute walking route between lectures. Attendance did not change, but students reported arriving less stressed and more prepared for the next class.", Evidence: "students reported arriving less stressed", ErrorTag: "detail", Prompt: "What benefit did students report?", Choices: []string{"More lectures", "Less stress", "Higher attendance", "Shorter classes"}, CorrectIndex: 1, Explanation: "Teks secara langsung menyebut mahasiswa tiba dengan stres yang lebih rendah.", LearningTip: "Cari klausa hasil yang menjawab pertanyaan, bukan asumsi di luar teks.", IELTSSkill: "Reading"},
		{ID: "r5", Type: "reading", Context: "A community garden began with twelve volunteers. After local schools joined the project, the number of weekly participants rose to forty, while the original volunteers continued to lead planning meetings.", Evidence: "the original volunteers continued to lead planning meetings", ErrorTag: "inference", Prompt: "What happened to the original volunteers?", Choices: []string{"They left the project", "They led planning meetings", "They joined a school", "They reduced the garden"}, CorrectIndex: 1, Explanation: "Bukti menyatakan relawan awal tetap memimpin rapat perencanaan.", LearningTip: "Untuk pertanyaan detail, cocokkan kata kerja utama pada bukti dengan pilihan jawaban.", IELTSSkill: "Reading"},
		{ID: "f4", Type: "fill_blank", Prompt: "The course aims to equip learners _____ strategies for independent study.", Choices: []string{"by", "with", "from", "at"}, CorrectIndex: 1, Explanation: "Collocation yang tepat adalah equip someone with something.", LearningTip: "Catat pola verb–preposition seperti equip with dan contribute to.", IELTSSkill: "Lexical Resource"},
		{ID: "f5", Type: "fill_blank", Prompt: "The committee reached a decision after _____ the evidence carefully.", Choices: []string{"consider", "considered", "considering", "to considering"}, CorrectIndex: 2, Explanation: "Setelah preposition after, gunakan gerund: after considering.", LearningTip: "Periksa kata sebelum blank untuk menentukan apakah yang dibutuhkan gerund atau infinitive.", IELTSSkill: "Grammatical Range and Accuracy"},
		{ID: "f6", Type: "fill_blank", Prompt: "There is growing concern _____ the effect of noise on concentration.", Choices: []string{"about", "at", "to", "with"}, CorrectIndex: 0, Explanation: "Noun concern berpasangan dengan about untuk menunjukkan hal yang dikhawatirkan.", LearningTip: "Pelajari noun–preposition collocations dalam contoh kalimat lengkap.", IELTSSkill: "Lexical Resource"},
		{ID: "l1", Type: "listening", Context: "Student: Excuse me, when will the community centre open its new study room?\nClerk: Next Monday, if the final inspection goes well.\nStudent: Will it have the same closing time as the old room?\nClerk: No. It will stay open until nine in the evening, one hour later.\nStudent: That will help students who work late.\nClerk: We hope so; the room will also have extra desks.", Prompt: "When will the new study room close?", Choices: []string{"Eight o'clock", "Nine o'clock", "Ten o'clock", "Next Sunday"}, CorrectIndex: 1, Explanation: "Clerk menyebut ruangan baru tutup pukul sembilan malam.", LearningTip: "Pada listening, catat angka dan perubahan waktu yang terdengar.", IELTSSkill: "Listening"},
		{ID: "l2", Type: "listening", Context: "Passenger: Could you tell me where the train leaves today?\nClerk: Please use platform three. Platform one is being repaired.\nPassenger: Has the departure time changed as well?\nClerk: No, it remains at ten twenty.\nPassenger: I usually wait near platform one.\nClerk: The signs will direct you across the concourse.", Prompt: "Why should passengers use platform three?", Choices: []string{"The train is faster", "Platform one is being repaired", "The departure is delayed", "Platform three is closer"}, CorrectIndex: 1, Explanation: "Clerk menjelaskan platform satu sedang diperbaiki.", LearningTip: "Dengarkan kata penghubung sebab seperti because dan since.", IELTSSkill: "Listening"},
		{ID: "l3", Type: "listening", Context: "Tutor: Are you ready for tomorrow's field trip?\nStudent: Almost. Should I bring a raincoat?\nTutor: The forecast is sunny, but bring a light jacket.\nStudent: Is it expected to rain after lunch?\nTutor: No, the temperature may fall after four o'clock.\nStudent: Then I will pack the jacket in my small bag.", Prompt: "Why is a jacket recommended?", Choices: []string{"Rain is expected", "The temperature may fall", "The trip starts at four", "The forecast is wrong"}, CorrectIndex: 1, Explanation: "Tutor mengatakan suhu mungkin turun setelah pukul empat.", LearningTip: "Bedakan alasan utama dari informasi latar dalam audio.", IELTSSkill: "Listening"},
		{ID: "e1", Type: "error_identification", Prompt: "Identify the error: If the policy [changes], market [prices] will [fell] drastically next [month].", Choices: []string{"changes", "prices", "fell", "month"}, CorrectIndex: 2, Explanation: "Setelah modal auxiliary 'will', kata kerja harus berupa bare infinitive (kata kerja bentuk pertama) yaitu 'fall', bukan past tense 'fell'.", LearningTip: "Modal auxiliary (will, can, must, should) selalu diikuti kata kerja bentuk dasar tanpa akhiran -ed atau bentuk lampau.", IELTSSkill: "Grammatical Range and Accuracy"},
		{ID: "e2", Type: "error_identification", Prompt: "Identify the error: The comprehensive report on urban transport [provide] concrete [guidelines] for regional [planners] under [supervision].", Choices: []string{"provide", "guidelines", "planners", "supervision"}, CorrectIndex: 0, Explanation: "Subjek utama 'The comprehensive report' adalah singular (tunggal), sehingga kata kerja dalam Present Simple wajib berakhiran -s ('provides').", LearningTip: "Cari subjek inti kalimat untuk memastikan kesesuaian subjek dan kata kerja (subject-verb agreement).", IELTSSkill: "Grammatical Range and Accuracy"},
		{ID: "e3", Type: "error_identification", Prompt: "Identify the error: In the preliminary study, the team [completed] the survey [remarkable] quickly despite [adverse] weather [conditions].", Choices: []string{"completed", "remarkable", "adverse", "conditions"}, CorrectIndex: 1, Explanation: "Kata 'quickly' adalah adverb, sehingga kata yang menerangkannya harus berupa adverb of degree yaitu 'remarkably', bukan adjective 'remarkable'.", LearningTip: "Gunakan adverb berakhiran -ly untuk menerangkan adjective atau adverb lainnya.", IELTSSkill: "Grammatical Range and Accuracy"},
	}
	byType := make(map[string][]question, len(in.Types))
	for _, q := range bank {
		if slices.Contains(in.Types, q.Type) {
			byType[q.Type] = append(byType[q.Type], q)
		}
	}
	out := make([]question, 0, in.Count)
	for i := 0; i < in.Count; i++ {
		typ := in.Types[i%len(in.Types)]
		questions := byType[typ]
		cycle := i / len(in.Types)
		q := questions[cycle%len(questions)]
		q.ID = fmt.Sprintf("demo-%d", i+1)
		out = append(out, q)
	}
	return out
}

func (a *app) allow(r *http.Request) bool {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		ip = host
	}
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	a.mu.Lock()
	defer a.mu.Unlock()
	recent := a.hits[ip][:0]
	for _, t := range a.hits[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 8 {
		a.hits[ip] = recent
		return false
	}
	a.hits[ip] = append(recent, now)
	return true
}
func newID() string {
	now := time.Now().UTC()
	return fmt.Sprintf("%x-%x", now.UnixNano(), now.UnixMilli()%0xffffff)
}
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "Data permintaan tidak valid.")
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, 400, "Data permintaan tidak valid.")
		if err == nil {
			return errors.New("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, 405, "Metode tidak didukung.")
}
