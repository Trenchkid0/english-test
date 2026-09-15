package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type questionBankStat struct {
	Level       string  `json:"level"`
	IELTSTarget float64 `json:"ieltsTarget"`
	Type        string  `json:"type"`
	Source      string  `json:"source"`
	Count       int     `json:"count"`
}

type adminQuestionRecord struct {
	DatabaseID  int64     `json:"databaseId"`
	Level       string    `json:"level"`
	IELTSTarget float64   `json:"ieltsTarget"`
	Type        string    `json:"type"`
	Source      string    `json:"source"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"createdAt"`
	Question    question  `json:"question"`
}

type adminQuestionFilters struct {
	Search    string
	Level     string
	Target    string
	Type      string
	Source    string
	Status    string
	SortOrder string
	Page      int
	PageSize  int
}

type questionBankQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type questionHistoryExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func dedupeQuestionBank(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE newer FROM question_bank newer JOIN question_bank older
		ON older.id < newer.id
		AND older.level = newer.level
		AND older.ielts_target = newer.ielts_target
		AND older.type = newer.type
		AND JSON_UNQUOTE(JSON_EXTRACT(older.question_json, '$.prompt')) = JSON_UNQUOTE(JSON_EXTRACT(newer.question_json, '$.prompt'))`)
	return err
}

func seedSystemQuestionBank(ctx context.Context, db *sql.DB) error {
	var existing int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM question_bank WHERE source='system'`).Scan(&existing); err != nil {
		return err
	}
	if existing >= 165000 {
		return nil
	}
	for _, level := range []string{"A1", "A2", "B1", "B2", "C1", "C2"} {
		for _, target := range []float64{4, 4.5, 5, 5.5, 6, 6.5, 7, 7.5, 8, 8.5, 9} {
			if _, err := insertBankQuestions(ctx, db, level, target, "system", systemSeedQuestions(level, target)); err != nil {
				return fmt.Errorf("seed question bank %s %.1f: %w", level, target, err)
			}
		}
	}
	return nil
}

// systemSeedQuestions keeps the built-in bank useful at every level without
// requiring an external AI call during startup. Each type receives 500 stable
// entries; the variant marker makes each bank record independently selectable.
func systemSeedQuestions(level string, target float64) []question {
	types := []string{"grammar", "vocabulary", "reading", "fill_blank", "listening", "error_identification"}
	base := demoQuestions(generateInput{Level: level, IELTSTarget: 6.5, Count: 25, DurationMinutes: 20, Types: types})
	byType := map[string][]question{}
	for _, q := range base {
		byType[q.Type] = append(byType[q.Type], q)
	}
	out := make([]question, 0, 2500)
	for _, typ := range types {
		pool := byType[typ]
		for i := 0; i < 500; i++ {
			q := pool[i%len(pool)]
			q.ID = fmt.Sprintf("system-%s-%.1f-%s-%02d", level, target, typ, i+1)
			// Keep a stable internal variant so hashes remain unique; the UI strips it.
			q.Prompt = fmt.Sprintf("%s [Level %s · Band %.1f · Practice %02d]", q.Prompt, level, target, i+1)
			q.ReviewKey = ""
			out = append(out, q)
		}
	}
	return out
}

func bankQuestionHash(level string, q question) string {
	return bankQuestionHashForTarget(level, 6.5, q)
}

func bankQuestionHashForTarget(level string, target float64, q question) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%.1f\x00%s", level, target, questionKey(q))))
	return hex.EncodeToString(sum[:])
}

func insertBankQuestions(ctx context.Context, db *sql.DB, level string, target float64, source string, questions []question) (int64, error) {
	if len(questions) == 0 {
		return 0, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	totalAdded := int64(0)
	// A full 500-question cell normally fits comfortably below MySQL's default
	// packet limit and avoids five separate network round trips per cell.
	const chunkSize = 500
	now := time.Now().UTC()

	for i := 0; i < len(questions); i += chunkSize {
		end := min(i+chunkSize, len(questions))
		chunk := questions[i:end]

		placeholders := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*7)
		for _, q := range chunk {
			q.ReviewKey = questionKey(q)
			raw, err := json.Marshal(q)
			if err != nil {
				return 0, err
			}
			placeholders = append(placeholders, "(?,?,?,?,?,?,TRUE,?)")
			args = append(args, bankQuestionHashForTarget(level, target, q), level, target, q.Type, source, raw, now)
		}
		query := `INSERT INTO question_bank (content_hash,level,ielts_target,type,source,question_json,active,created_at) VALUES ` + strings.Join(placeholders, ",") +
			` ON DUPLICATE KEY UPDATE question_json=VALUES(question_json)`
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return 0, err
		}
		added, _ := result.RowsAffected()
		totalAdded += added
	}

	return totalAdded, tx.Commit()
}

func (a *app) selectBankQuestions(ctx context.Context, queryer questionBankQuerier, uid int64, input generateInput) ([]question, error) {
	byType := make(map[string][]question, len(input.Types))
	seenPrompts := map[string]struct{}{}
	seenKeys, err := seenQuestionKeys(ctx, queryer, uid)
	if err != nil {
		return nil, fmt.Errorf("membaca riwayat soal: %w", err)
	}
	for _, typ := range input.Types {
		rows, err := queryer.QueryContext(ctx, `SELECT question_json, source FROM question_bank WHERE active=TRUE AND source<>'system' AND level=? AND (ielts_target=? OR source='authentic_curated') AND (type=? OR (type IN ('fill_blank', 'fill_in_blank', 'fill_in_the_blank') AND ? IN ('fill_blank', 'fill_in_blank', 'fill_in_the_blank'))) ORDER BY CASE WHEN source='authentic_curated' THEN 0 ELSE 1 END, id ASC`, input.Level, input.IELTSTarget, typ, typ)
		if err != nil {
			return nil, err
		}
		var authenticQuestions []question
		var fallbackQuestions []question
		for rows.Next() {
			var raw []byte
			var src string
			var q question
			if rows.Scan(&raw, &src) == nil && json.Unmarshal(raw, &q) == nil {
				q.Prompt = visiblePromptKey(q.Prompt)
				if q.Type == "listening" && (len([]rune(strings.TrimSpace(q.Context))) < 180 || strings.Count(q.Context, ":") < 4 || strings.Count(q.Context, "\n") < 4) {
					continue
				}
				promptKey := visiblePromptKey(q.Prompt)
				if _, exists := seenPrompts[promptKey]; exists {
					continue
				}
				seenPrompts[promptKey] = struct{}{}
				if _, seen := seenKeys[bankHistoryKey(input.Level, input.IELTSTarget, q)]; seen {
					continue
				}
				if src == authenticQuestionSource {
					authenticQuestions = append(authenticQuestions, q)
				} else {
					fallbackQuestions = append(fallbackQuestions, q)
				}
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		shuffleQuestions(authenticQuestions)
		shuffleQuestions(fallbackQuestions)
		byType[typ] = append(authenticQuestions, fallbackQuestions...)
		if len(byType[typ]) == 0 {
			return nil, fmt.Errorf("stok soal baru %s untuk level %s dan target IELTS %.1f sudah habis", typ, input.Level, input.IELTSTarget)
		}
	}

	needed := make(map[string]int, len(input.Types))
	for i := 0; i < input.Count; i++ {
		needed[input.Types[i%len(input.Types)]]++
	}
	for typ, count := range needed {
		if len(byType[typ]) < count {
			return nil, fmt.Errorf("stok soal baru %s untuk level %s dan target IELTS %.1f tersisa %d, butuh %d", typ, input.Level, input.IELTSTarget, len(byType[typ]), count)
		}
	}
	cursors := make(map[string]int, len(input.Types))
	selected := make([]question, 0, input.Count)
	for i := 0; i < input.Count; i++ {
		typ := input.Types[i%len(input.Types)]
		pool := byType[typ]
		q := pool[cursors[typ]]
		cursors[typ]++
		q.ID = fmt.Sprintf("bank-%d", i+1)
		if q.ReviewKey == "" {
			q.ReviewKey = questionKey(q)
		}
		selected = append(selected, q)
	}
	shuffleQuestions(selected)
	return selected, nil
}

func shuffleQuestions(items []question) {
	rand.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
}

func seenQuestionKeys(ctx context.Context, queryer questionBankQuerier, uid int64) (map[string]struct{}, error) {
	keys := map[string]struct{}{}
	rows, err := queryer.QueryContext(ctx, `SELECT question_key FROM user_question_history WHERE user_id=?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys[key] = struct{}{}
	}
	return keys, rows.Err()
}

func bankHistoryKey(level string, target float64, q question) string {
	q.Prompt = visiblePromptKey(q.Prompt)
	q.ReviewKey = ""
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%.1f\x00%s", level, target, questionKey(q))))
	return hex.EncodeToString(sum[:])
}

func recordUserQuestionHistory(ctx context.Context, execer questionHistoryExecer, uid int64, sessionID, level string, target float64, questions []question, seenAt time.Time) error {
	unique := make(map[string]struct{}, len(questions))
	placeholders := make([]string, 0, len(questions))
	args := make([]any, 0, len(questions)*4)
	for _, q := range questions {
		key := bankHistoryKey(level, target, q)
		if _, exists := unique[key]; exists {
			continue
		}
		unique[key] = struct{}{}
		placeholders = append(placeholders, "(?,?,?,?)")
		args = append(args, uid, key, sessionID, seenAt)
	}
	if len(placeholders) == 0 {
		return nil
	}
	_, err := execer.ExecContext(ctx, `INSERT IGNORE INTO user_question_history (user_id,question_key,session_id,first_seen_at) VALUES `+strings.Join(placeholders, ","), args...)
	return err
}

func backfillUserQuestionHistory(ctx context.Context, db *sql.DB) error {
	const migrationName = "user_question_history_v2_scoped"
	var completed int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_data_migrations WHERE name=?`, migrationName).Scan(&completed); err != nil {
		return fmt.Errorf("question history migration check: %w", err)
	}
	if completed > 0 {
		return nil
	}

	rows, err := db.QueryContext(ctx, `SELECT id,user_id,created_at,level,ielts_target,questions_json FROM practice_sessions`)
	if err != nil {
		return fmt.Errorf("question history backfill query: %w", err)
	}
	type historicalSession struct {
		id        string
		uid       int64
		createdAt time.Time
		level     string
		target    float64
		questions []question
	}
	sessions := []historicalSession{}
	for rows.Next() {
		var item historicalSession
		var raw []byte
		if err := rows.Scan(&item.id, &item.uid, &item.createdAt, &item.level, &item.target, &raw); err != nil {
			rows.Close()
			return fmt.Errorf("question history backfill scan: %w", err)
		}
		if err := json.Unmarshal(raw, &item.questions); err != nil {
			rows.Close()
			return fmt.Errorf("question history backfill decode session %s: %w", item.id, err)
		}
		sessions = append(sessions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("question history backfill rows: %w", err)
	}
	rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("question history backfill transaction: %w", err)
	}
	defer tx.Rollback()
	for _, item := range sessions {
		if err := recordUserQuestionHistory(ctx, tx, item.uid, item.id, item.level, item.target, item.questions, item.createdAt); err != nil {
			return fmt.Errorf("question history backfill session %s: %w", item.id, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO app_data_migrations (name,completed_at) VALUES (?,?)`, migrationName, time.Now().UTC()); err != nil {
		return fmt.Errorf("question history migration marker: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("question history backfill commit: %w", err)
	}
	return nil
}

// visiblePromptKey removes the generated practice marker so variants that
// look identical to a learner are never selected in the same test.
func visiblePromptKey(prompt string) string {
	if marker := strings.Index(prompt, " [Level "); marker >= 0 {
		return strings.TrimSpace(prompt[:marker])
	}
	return strings.TrimSpace(prompt)
}

func (a *app) questionBank(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT level,ielts_target,type,source,COUNT(*) FROM question_bank WHERE active=TRUE GROUP BY level,ielts_target,type,source ORDER BY level,ielts_target,type,source`)
	if err != nil {
		writeError(w, 500, "Bank soal belum dapat dibuka.")
		return
	}
	defer rows.Close()
	stats := []questionBankStat{}
	for rows.Next() {
		var item questionBankStat
		if rows.Scan(&item.Level, &item.IELTSTarget, &item.Type, &item.Source, &item.Count) == nil {
			stats = append(stats, item)
		}
	}
	writeJSON(w, 200, map[string]any{"stats": stats})
}

func parseAdminQuestionFilters(r *http.Request) adminQuestionFilters {
	query := r.URL.Query()
	page, _ := strconv.Atoi(query.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(query.Get("pageSize"))
	if pageSize != 24 && pageSize != 48 && pageSize != 96 {
		pageSize = 24
	}
	search := strings.TrimSpace(query.Get("query"))
	if runes := []rune(search); len(runes) > 200 {
		search = string(runes[:200])
	}
	status := query.Get("status")
	if status != "active" && status != "inactive" {
		status = "all"
	}
	sortOrder := strings.ToLower(strings.TrimSpace(query.Get("sort")))
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	return adminQuestionFilters{
		Search: search, Level: query.Get("level"), Target: query.Get("target"),
		Type: query.Get("type"), Source: strings.TrimSpace(query.Get("source")),
		Status: status, SortOrder: sortOrder, Page: page, PageSize: pageSize,
	}
}

func buildAdminQuestionsWhere(filters adminQuestionFilters) (string, []any) {
	clauses := []string{"1=1"}
	args := make([]any, 0, 8)
	if filters.Search != "" {
		clauses = append(clauses, `(CAST(id AS CHAR)=? OR question_json LIKE ?)`)
		args = append(args, filters.Search, "%"+filters.Search+"%")
	}
	if filters.Level != "" {
		clauses = append(clauses, "level=?")
		args = append(args, filters.Level)
	}
	if filters.Target != "" {
		if target, err := strconv.ParseFloat(filters.Target, 64); err == nil && target >= 4 && target <= 9 && float64(int(target*2)) == target*2 {
			clauses = append(clauses, "ielts_target=?")
			args = append(args, target)
		}
	}
	if filters.Type != "" {
		clauses = append(clauses, "type=?")
		args = append(args, filters.Type)
	}
	if filters.Source != "" {
		clauses = append(clauses, "source=?")
		args = append(args, filters.Source)
	}
	if filters.Status == "active" {
		clauses = append(clauses, "active=TRUE")
	} else if filters.Status == "inactive" {
		clauses = append(clauses, "active=FALSE")
	}
	return strings.Join(clauses, " AND "), args
}

func (a *app) adminQuestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	filters := parseAdminQuestionFilters(r)
	where, args := buildAdminQuestionsWhere(filters)

	var total int
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM question_bank WHERE `+where, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "Jumlah soal belum dapat dibaca.")
		return
	}
	pages := 0
	if total > 0 {
		pages = (total + filters.PageSize - 1) / filters.PageSize
		if filters.Page > pages {
			filters.Page = pages
		}
	}

	orderClause := "ORDER BY created_at DESC, id DESC"
	if filters.SortOrder == "asc" {
		orderClause = "ORDER BY created_at ASC, id ASC"
	}

	listArgs := append(append([]any{}, args...), filters.PageSize, (filters.Page-1)*filters.PageSize)
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,level,ielts_target,type,source,active,created_at,question_json
		FROM question_bank WHERE `+where+` `+orderClause+` LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Daftar soal belum dapat dibuka.")
		return
	}
	defer rows.Close()
	items := make([]adminQuestionRecord, 0, filters.PageSize)
	for rows.Next() {
		var item adminQuestionRecord
		var raw []byte
		if err := rows.Scan(&item.DatabaseID, &item.Level, &item.IELTSTarget, &item.Type, &item.Source, &item.Active, &item.CreatedAt, &raw); err != nil {
			writeError(w, http.StatusInternalServerError, "Salah satu soal belum dapat dibaca.")
			return
		}
		if err := json.Unmarshal(raw, &item.Question); err != nil {
			writeError(w, http.StatusInternalServerError, "Format salah satu soal tidak valid.")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Daftar soal terputus saat dibaca.")
		return
	}
	rows.Close()

	sourceRows, err := a.db.QueryContext(r.Context(), `SELECT DISTINCT source FROM question_bank ORDER BY source`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Daftar sumber soal belum dapat dibaca.")
		return
	}
	sources := []string{}
	for sourceRows.Next() {
		var source string
		if sourceRows.Scan(&source) == nil {
			sources = append(sources, source)
		}
	}
	sourceErr := sourceRows.Err()
	sourceRows.Close()
	if sourceErr != nil {
		writeError(w, http.StatusInternalServerError, "Daftar sumber soal terputus saat dibaca.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": filters.Page, "pageSize": filters.PageSize,
		"pages": pages, "sources": sources,
	})
}

func (a *app) generateQuestionBank(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !a.allow(r) {
		writeError(w, 429, "Terlalu banyak permintaan. Tunggu satu menit lalu coba lagi.")
		return
	}
	var input generateInput
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if message := validateInput(input); message != "" {
		writeError(w, 422, message)
		return
	}
	questions, source, err := a.generateQuestions(r.Context(), input)
	if err != nil {
		writeError(w, 502, "Soal belum dapat dibuat. Periksa konfigurasi DeepSeek lalu coba lagi.")
		return
	}
	added, err := insertBankQuestions(r.Context(), a.db, input.Level, input.IELTSTarget, source, questions)
	if err != nil {
		writeError(w, 500, "Soal berhasil dibuat tetapi belum dapat disimpan.")
		return
	}
	if err := dedupeQuestionBank(r.Context(), a.db); err != nil {
		writeError(w, 500, "Soal tersimpan tetapi bank belum dapat dibersihkan dari duplikat.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"added": added, "generated": len(questions), "source": source})
}
