package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"regexp"
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
	CursorAt  time.Time
	CursorID  int64
	HasCursor bool
}

type cachedQuestionSources struct {
	items     []string
	expiresAt time.Time
}

var adminSearchTokens = regexp.MustCompile(`[\pL\pN]+`)

type questionBankQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type questionHistoryExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
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

func bankPromptHash(q question) string {
	prompt := strings.ToLower(strings.TrimSpace(q.Prompt))
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

func insertBankQuestions(ctx context.Context, db *sql.DB, level string, target float64, source string, questions []question) (int64, error) {
	if len(questions) == 0 {
		return 0, nil
	}

	const chunkSize = 100
	now := time.Now().UTC()
	totalAdded := int64(0)

	for i := 0; i < len(questions); i += chunkSize {
		end := min(i+chunkSize, len(questions))
		chunk := questions[i:end]

		placeholders := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*9)
		for _, q := range chunk {
			q.ReviewKey = questionKey(q)
			raw, err := json.Marshal(q)
			if err != nil {
				return totalAdded, err
			}
			placeholders = append(placeholders, "(?,?,?,?,?,?,?,?,TRUE,?)")
			args = append(args, bankQuestionHashForTarget(level, target, q), level, target, q.Type, source, bankPromptHash(q), q.Prompt, raw, now)
		}
		query := `INSERT INTO question_bank (content_hash,level,ielts_target,type,source,prompt_hash,prompt_text,question_json,active,created_at) VALUES ` + strings.Join(placeholders, ",") +
			` ON DUPLICATE KEY UPDATE prompt_text=VALUES(prompt_text), question_json=VALUES(question_json), active=TRUE`

		var chunkErr error
		for attempt := 0; attempt < 5; attempt++ {
			if ctx.Err() != nil {
				return totalAdded, ctx.Err()
			}
			result, err := db.ExecContext(ctx, query, args...)
			if err == nil {
				added, _ := result.RowsAffected()
				totalAdded += added
				chunkErr = nil
				break
			}
			chunkErr = err
			time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
		}
		if chunkErr != nil {
			return totalAdded, chunkErr
		}
	}

	return totalAdded, nil
}

func testQuestionDedupeKey(q question) string {
	prompt := strings.ToLower(strings.Join(strings.Fields(visiblePromptKey(q.Prompt)), " "))
	ctx := strings.ToLower(strings.Join(strings.Fields(q.Context), " "))
	if ctx != "" {
		return prompt + "\x00" + ctx
	}
	return prompt
}

const userQuestionCooldownSessions = 5

func recentUserCooldownQuestionKeys(ctx context.Context, queryer questionBankQuerier, uid int64, sessionLimit int) (map[string]struct{}, error) {
	if uid <= 0 || sessionLimit <= 0 {
		return map[string]struct{}{}, nil
	}
	query := `
		SELECT uqh.question_key
		FROM user_question_history uqh
		INNER JOIN (
			SELECT id FROM practice_sessions
			WHERE user_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		) AS recent_s ON uqh.session_id = recent_s.id
		WHERE uqh.user_id = ?
	`
	rows, err := queryer.QueryContext(ctx, query, uid, sessionLimit, uid)
	if err != nil {
		return map[string]struct{}{}, nil
	}
	defer rows.Close()
	cooldown := make(map[string]struct{})
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err == nil && key != "" {
			cooldown[key] = struct{}{}
		}
	}
	return cooldown, rows.Err()
}

func (a *app) selectBankQuestions(ctx context.Context, queryer questionBankQuerier, uid int64, input generateInput) ([]question, error) {
	cooldownKeys, err := recentUserCooldownQuestionKeys(ctx, queryer, uid, userQuestionCooldownSessions)
	if err != nil {
		log.Printf("fetch cooldown question keys: %v", err)
		cooldownKeys = map[string]struct{}{}
	}

	needed := make(map[string]int, len(input.Types))
	for i := 0; i < input.Count; i++ {
		needed[input.Types[i%len(input.Types)]]++
	}

	byType := make(map[string][]question, len(input.Types))
	for _, typ := range input.Types {
		if _, exists := byType[typ]; exists {
			continue
		}
		// Read only the requested level/band/type cell. Previously this query did
		// not filter ielts_target and loaded tens of thousands of JSON documents
		// into Go before selecting a handful of questions.
		typeClause := "type=?"
		queryArgs := []any{input.Level, input.IELTSTarget, typ}
		if typ == "fill_blank" {
			typeClause = "type IN (?,?,?)"
			queryArgs = []any{input.Level, input.IELTSTarget, "fill_blank", "fill_in_blank", "fill_in_the_blank"}
		}
		candidateLimit := max(200, needed[typ]*4)
		queryArgs = append(queryArgs, candidateLimit)
		rows, err := queryer.QueryContext(ctx, `SELECT question_json, source
			FROM question_bank
			WHERE active=TRUE AND level=? AND ielts_target=? AND `+typeClause+`
			ORDER BY id DESC LIMIT ?`, queryArgs...)
		if err != nil {
			return nil, err
		}
		var freshAuthentic []question
		var cooldownAuthentic []question
		typeSeen := map[string]struct{}{}
		for rows.Next() {
			var raw []byte
			var src string
			var q question
			if rows.Scan(&raw, &src) == nil && json.Unmarshal(raw, &q) == nil {
				q.Prompt = visiblePromptKey(q.Prompt)
				if q.Type == "listening" && (len([]rune(strings.TrimSpace(q.Context))) < 60 || strings.Count(q.Context, ":") < 2) {
					continue
				}
				key := testQuestionDedupeKey(q)
				qKey := questionKey(q)
				if _, exists := typeSeen[key]; exists {
					continue
				}
				if _, exists := typeSeen[qKey]; exists {
					continue
				}
				typeSeen[key] = struct{}{}
				typeSeen[qKey] = struct{}{}

				hKey := bankHistoryKey(input.Level, input.IELTSTarget, q)
				_, inCooldown := cooldownKeys[hKey]

				if inCooldown {
					cooldownAuthentic = append(cooldownAuthentic, q)
				} else {
					freshAuthentic = append(freshAuthentic, q)
				}
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		shuffleQuestions(freshAuthentic)
		shuffleQuestions(cooldownAuthentic)

		// Prioritize fresh authentic questions (outside the 5-session cooldown)
		typePool := append(freshAuthentic, cooldownAuthentic...)

		byType[typ] = typePool
		if len(byType[typ]) == 0 {
			return nil, fmt.Errorf("stok soal %s untuk level %s dan target IELTS %.1f sudah habis", typ, input.Level, input.IELTSTarget)
		}
	}

	cursors := make(map[string]int, len(input.Types))
	selectedKeys := make(map[string]struct{}, input.Count)
	selected := make([]question, 0, input.Count)
	for i := 0; i < input.Count; i++ {
		typ := input.Types[i%len(input.Types)]
		pool := byType[typ]
		var chosen *question
		for cursors[typ] < len(pool) {
			candidate := pool[cursors[typ]]
			cursors[typ]++
			key := testQuestionDedupeKey(candidate)
			qKey := questionKey(candidate)
			if _, used := selectedKeys[key]; used {
				continue
			}
			if _, used := selectedKeys[qKey]; used {
				continue
			}
			selectedKeys[key] = struct{}{}
			selectedKeys[qKey] = struct{}{}
			chosen = &candidate
			break
		}
		if chosen == nil {
			return nil, fmt.Errorf("stok soal unik %s untuk level %s dan target IELTS %.1f tidak cukup (butuh %d)", typ, input.Level, input.IELTSTarget, needed[typ])
		}
		q := *chosen
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
	_, err := execer.ExecContext(ctx, `INSERT INTO user_question_history (user_id,question_key,session_id,first_seen_at) VALUES `+strings.Join(placeholders, ",")+` ON DUPLICATE KEY UPDATE session_id=VALUES(session_id), first_seen_at=VALUES(first_seen_at)`, args...)
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
	filters := adminQuestionFilters{
		Search: search, Level: query.Get("level"), Target: query.Get("target"),
		Type: query.Get("type"), Source: strings.TrimSpace(query.Get("source")),
		Status: status, SortOrder: sortOrder, Page: page, PageSize: pageSize,
	}
	if cursorAt, cursorID, ok := decodeAdminCursor(query.Get("cursor")); ok {
		filters.CursorAt, filters.CursorID, filters.HasCursor = cursorAt, cursorID, true
	}
	return filters
}

func encodeAdminCursor(createdAt time.Time, id int64) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeAdminCursor(value string) (time.Time, int64, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, 0, false
	}
	timestamp, rawID, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, 0, false
	}
	createdAt, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return time.Time{}, 0, false
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	return createdAt, id, err == nil && id > 0
}

func fullTextSearchValue(search string) string {
	tokens := adminSearchTokens.FindAllString(strings.ToLower(search), -1)
	terms := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if len([]rune(token)) >= 3 {
			terms = append(terms, "+"+token+"*")
		}
	}
	return strings.Join(terms, " ")
}

func buildAdminQuestionsWhere(filters adminQuestionFilters) (string, []any) {
	clauses := []string{"1=1"}
	args := make([]any, 0, 8)
	if filters.Search != "" {
		if id, err := strconv.ParseInt(filters.Search, 10, 64); err == nil && id > 0 {
			clauses = append(clauses, "id=?")
			args = append(args, id)
		} else if fullText := fullTextSearchValue(filters.Search); fullText != "" {
			clauses = append(clauses, "MATCH(prompt_text) AGAINST (? IN BOOLEAN MODE)")
			args = append(args, fullText)
		} else {
			clauses = append(clauses, "prompt_text LIKE ?")
			args = append(args, filters.Search+"%")
		}
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
	if filters.HasCursor {
		operator := "<"
		if filters.SortOrder == "asc" {
			operator = ">"
		}
		clauses = append(clauses, `(created_at `+operator+` ? OR (created_at=? AND id `+operator+` ?))`)
		args = append(args, filters.CursorAt, filters.CursorAt, filters.CursorID)
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

	orderClause := "ORDER BY created_at DESC, id DESC"
	if filters.SortOrder == "asc" {
		orderClause = "ORDER BY created_at ASC, id ASC"
	}

	listArgs := append(append([]any{}, args...), filters.PageSize+1)
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,level,ielts_target,type,source,active,created_at,question_json
		FROM question_bank WHERE `+where+` `+orderClause+` LIMIT ?`, listArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Daftar soal belum dapat dibuka.")
		return
	}
	defer rows.Close()
	items := make([]adminQuestionRecord, 0, filters.PageSize+1)
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
	hasNext := len(items) > filters.PageSize
	if hasNext {
		items = items[:filters.PageSize]
	}
	nextCursor := ""
	if hasNext && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeAdminCursor(last.CreatedAt, last.DatabaseID)
	}
	sources, err := a.questionSources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Daftar sumber soal belum dapat dibaca.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "page": filters.Page, "pageSize": filters.PageSize,
		"hasNext": hasNext, "nextCursor": nextCursor, "sources": sources,
	})
}

func (a *app) questionSources(ctx context.Context) ([]string, error) {
	now := time.Now()
	a.sourceMu.RLock()
	if now.Before(a.sourceCache.expiresAt) {
		items := append([]string(nil), a.sourceCache.items...)
		a.sourceMu.RUnlock()
		return items, nil
	}
	a.sourceMu.RUnlock()

	rows, err := a.db.QueryContext(ctx, `SELECT DISTINCT source FROM question_bank ORDER BY source`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []string{}
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			return nil, err
		}
		items = append(items, source)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	a.sourceMu.Lock()
	a.sourceCache = cachedQuestionSources{items: append([]string(nil), items...), expiresAt: now.Add(5 * time.Minute)}
	a.sourceMu.Unlock()
	return items, nil
}

func (a *app) clearQuestionSourceCache() {
	a.sourceMu.Lock()
	a.sourceCache = cachedQuestionSources{}
	a.sourceMu.Unlock()
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
	a.clearQuestionSourceCache()
	writeJSON(w, http.StatusCreated, map[string]any{"added": added, "generated": len(questions), "source": source})
}
