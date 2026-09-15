package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type dashboardResponse struct {
	CompletedSessions int              `json:"completedSessions"`
	AverageScore      *float64         `json:"averageScore"`
	DueReviews        int              `json:"dueReviews"`
	VocabularyCount   int              `json:"vocabularyCount"`
	WritingCount      int              `json:"writingCount"`
	Weaknesses        []weaknessStat   `json:"weaknesses"`
	Weekly            []weeklyProgress `json:"weekly"`
}

type weaknessStat struct {
	Type     string `json:"type"`
	Wrong    int    `json:"wrong"`
	Total    int    `json:"total"`
	Accuracy int    `json:"accuracy"`
}

type weeklyProgress struct {
	Date     string `json:"date"`
	Sessions int    `json:"sessions"`
	Average  int    `json:"average"`
}

type reviewCardSummary struct {
	Key        string         `json:"key"`
	Question   publicQuestion `json:"question"`
	Mastery    int            `json:"mastery"`
	NextReview time.Time      `json:"nextReviewAt"`
}

type vocabularyCard struct {
	ID          int64     `json:"id"`
	Word        string    `json:"word"`
	Meaning     string    `json:"meaning"`
	Example     string    `json:"example"`
	Collocation string    `json:"collocation"`
	Mastery     int       `json:"mastery"`
	NextReview  time.Time `json:"nextReviewAt"`
	CreatedAt   time.Time `json:"createdAt"`
}

type writingFeedback struct {
	TaskResponse float64  `json:"taskResponse"`
	Coherence    float64  `json:"coherence"`
	Lexical      float64  `json:"lexicalResource"`
	Grammar      float64  `json:"grammar"`
	Overall      float64  `json:"overallBand"`
	Summary      string   `json:"summary"`
	Strengths    []string `json:"strengths"`
	NextSteps    []string `json:"nextSteps"`
	Corrections  []string `json:"corrections"`
}

type writingSubmission struct {
	ID        int64           `json:"id"`
	TaskType  string          `json:"taskType"`
	Prompt    string          `json:"prompt"`
	Response  string          `json:"response"`
	WordCount int             `json:"wordCount"`
	Feedback  writingFeedback `json:"feedback"`
	Source    string          `json:"source"`
	CreatedAt time.Time       `json:"createdAt"`
}

func assignReviewKeys(questions []question) {
	for i := range questions {
		if questions[i].ReviewKey == "" {
			questions[i].ReviewKey = questionKey(questions[i])
		}
	}
}

func questionKey(q question) string {
	payload := q.Type + "\x00" + q.Context + "\x00" + q.Prompt + "\x00" + strings.Join(q.Choices, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func userQuestionKey(uid int64, q question) string {
	sum := sha256.Sum256([]byte(strconv.FormatInt(uid, 10) + "\x00" + questionKey(q)))
	return hex.EncodeToString(sum[:])
}

func (a *app) updateReviewSchedule(ctx context.Context, s session, now time.Time) error {
	assignReviewKeys(s.Questions)
	uid := userID(ctx)
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index, q := range s.Questions {
		q.ReviewKey = userQuestionKey(uid, q)
		selected, answered := s.Answers[index]
		correct := answered && selected == q.CorrectIndex
		mastery := 0
		var existing int
		err := tx.QueryRowContext(ctx, `SELECT mastery FROM review_cards WHERE card_key=? AND user_id=? FOR UPDATE`, q.ReviewKey, uid).Scan(&existing)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if correct {
			mastery = min(5, existing+1)
		}
		interval := reviewInterval(mastery, correct)
		next := now.AddDate(0, 0, interval)
		raw, _ := json.Marshal(q)
		_, err = tx.ExecContext(ctx, `INSERT INTO review_cards
			(card_key,user_id,question_json,mastery,interval_days,next_review_at,last_correct,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE question_json=VALUES(question_json), mastery=VALUES(mastery),
			interval_days=VALUES(interval_days), next_review_at=VALUES(next_review_at), last_correct=VALUES(last_correct), updated_at=VALUES(updated_at)`,
			q.ReviewKey, uid, raw, mastery, interval, next, correct, now, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func reviewInterval(mastery int, correct bool) int {
	if !correct {
		return 1
	}
	switch {
	case mastery >= 5:
		return 14
	case mastery >= 3:
		return 7
	default:
		return 3
	}
}

func (a *app) reviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT card_key,question_json,mastery,next_review_at FROM review_cards WHERE user_id=? AND next_review_at<=? ORDER BY next_review_at LIMIT 50`, userID(r.Context()), time.Now().UTC())
	if err != nil {
		writeError(w, 500, "Review terjadwal belum dapat dibuka.")
		return
	}
	defer rows.Close()
	items := []reviewCardSummary{}
	for rows.Next() {
		var key string
		var raw []byte
		var mastery int
		var next time.Time
		if rows.Scan(&key, &raw, &mastery, &next) != nil {
			continue
		}
		var q question
		if json.Unmarshal(raw, &q) != nil {
			continue
		}
		items = append(items, reviewCardSummary{Key: key, Question: publicQuestions([]question{q})[0], Mastery: mastery, NextReview: next})
	}
	writeJSON(w, 200, items)
}

func (a *app) reviewSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !a.allow(r) {
		writeError(w, 429, "Terlalu banyak permintaan. Tunggu satu menit lalu coba lagi.")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT question_json FROM review_cards WHERE user_id=? AND next_review_at<=? ORDER BY next_review_at LIMIT 20`, userID(r.Context()), time.Now().UTC())
	if err != nil {
		writeError(w, 500, "Review terjadwal belum dapat dibuat.")
		return
	}
	questions := []question{}
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) == nil {
			var q question
			if json.Unmarshal(raw, &q) == nil {
				questions = append(questions, q)
			}
		}
	}
	rows.Close()
	if len(questions) == 0 {
		writeError(w, 409, "Belum ada review yang jatuh tempo.")
		return
	}
	s, err := a.storeQuestionSession(r.Context(), questions, "spaced_review", "B1", 6.5, max(5, len(questions)*2))
	if err != nil {
		writeError(w, 500, "Review terjadwal belum dapat dibuat.")
		return
	}
	writeJSON(w, http.StatusCreated, sessionResponse{Session: s})
}

func (a *app) storeQuestionSession(ctx context.Context, questions []question, source, level string, target float64, duration int) (session, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return session{}, err
	}
	defer tx.Rollback()
	s, err := storeQuestionSessionTx(ctx, tx, userID(ctx), questions, source, level, target, duration)
	if err != nil {
		return session{}, err
	}
	if err := tx.Commit(); err != nil {
		return session{}, err
	}
	return s, nil
}

func storeQuestionSessionTx(ctx context.Context, tx *sql.Tx, uid int64, questions []question, source, level string, target float64, duration int) (session, error) {
	assignReviewKeys(questions)
	types := questionTypes(questions)
	now := time.Now().UTC()
	s := session{ID: newID(), CreatedAt: now, UpdatedAt: now, Level: level, IELTSTarget: target, DurationMinutes: min(60, duration), Types: types, Source: source, Status: "in_progress", Questions: questions, Answers: map[int]int{}}
	s.PublicQuestions = publicQuestions(questions)
	qJSON, _ := json.Marshal(questions)
	typeJSON, _ := json.Marshal(types)
	_, err := tx.ExecContext(ctx, `INSERT INTO practice_sessions
		(id,user_id,created_at,updated_at,level,ielts_target,duration_minutes,question_types,source,status,questions_json)
		VALUES (?,?,?,?,?,?,?,?,?, 'in_progress',?)`, s.ID, uid, now, now, level, target, s.DurationMinutes, typeJSON, source, qJSON)
	if err != nil {
		return session{}, err
	}
	if err := recordUserQuestionHistory(ctx, tx, uid, s.ID, level, target, questions, now); err != nil {
		return session{}, err
	}
	return s, nil
}

func (a *app) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	var out dashboardResponse
	var average sql.NullFloat64
	uid := userID(r.Context())
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*),AVG(score) FROM practice_sessions WHERE user_id=? AND status='completed'`, uid).Scan(&out.CompletedSessions, &average); err != nil {
		writeError(w, 500, "Dashboard belum dapat dibuka.")
		return
	}
	if average.Valid {
		value := average.Float64
		out.AverageScore = &value
	}
	_ = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM review_cards WHERE user_id=? AND next_review_at<=?`, uid, time.Now().UTC()).Scan(&out.DueReviews)
	_ = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM vocabulary_cards WHERE user_id=?`, uid).Scan(&out.VocabularyCount)
	_ = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM writing_submissions WHERE user_id=?`, uid).Scan(&out.WritingCount)
	out.Weaknesses = a.calculateWeaknesses(r.Context())
	rows, err := a.db.QueryContext(r.Context(), `SELECT DATE(created_at),COUNT(*),ROUND(AVG(score)) FROM practice_sessions WHERE user_id=? AND status='completed' AND created_at>=DATE_SUB(CURDATE(),INTERVAL 6 DAY) GROUP BY DATE(created_at) ORDER BY DATE(created_at)`, uid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var day time.Time
			var item weeklyProgress
			if rows.Scan(&day, &item.Sessions, &item.Average) == nil {
				item.Date = day.Format("2006-01-02")
				out.Weekly = append(out.Weekly, item)
			}
		}
	}
	if out.Weaknesses == nil {
		out.Weaknesses = []weaknessStat{}
	}
	if out.Weekly == nil {
		out.Weekly = []weeklyProgress{}
	}
	writeJSON(w, 200, out)
}

func (a *app) calculateWeaknesses(ctx context.Context) []weaknessStat {
	rows, err := a.db.QueryContext(ctx, `SELECT id FROM practice_sessions WHERE user_id=? AND status='completed' ORDER BY created_at DESC LIMIT 50`, userID(ctx))
	if err != nil {
		return []weaknessStat{}
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	stats := map[string]*weaknessStat{}
	for _, id := range ids {
		s, err := a.loadSession(ctx, id)
		if err != nil {
			continue
		}
		for i, q := range s.Questions {
			item := stats[q.Type]
			if item == nil {
				item = &weaknessStat{Type: q.Type}
				stats[q.Type] = item
			}
			item.Total++
			if selected, ok := s.Answers[i]; !ok || selected != q.CorrectIndex {
				item.Wrong++
			}
		}
	}
	out := make([]weaknessStat, 0, len(stats))
	for _, item := range stats {
		item.Accuracy = int(float64(item.Total-item.Wrong)*100/float64(item.Total) + .5)
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Accuracy == out[j].Accuracy {
			return out[i].Type < out[j].Type
		}
		return out[i].Accuracy < out[j].Accuracy
	})
	return out
}

func (a *app) vocabulary(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := a.db.QueryContext(r.Context(), `SELECT id,word,meaning,example,collocation,mastery,next_review_at,created_at FROM vocabulary_cards WHERE user_id=? ORDER BY next_review_at,word LIMIT 100`, userID(r.Context()))
		if err != nil {
			writeError(w, 500, "Notebook kosakata belum dapat dibuka.")
			return
		}
		defer rows.Close()
		items := []vocabularyCard{}
		for rows.Next() {
			var item vocabularyCard
			if rows.Scan(&item.ID, &item.Word, &item.Meaning, &item.Example, &item.Collocation, &item.Mastery, &item.NextReview, &item.CreatedAt) == nil {
				items = append(items, item)
			}
		}
		writeJSON(w, 200, items)
	case http.MethodPost:
		var body struct {
			Word, Meaning, Example, Collocation string
		}
		if decodeJSON(w, r, &body) != nil {
			return
		}
		body.Word = strings.TrimSpace(body.Word)
		body.Meaning = strings.TrimSpace(body.Meaning)
		body.Example = strings.TrimSpace(body.Example)
		body.Collocation = strings.TrimSpace(body.Collocation)
		if body.Word == "" || body.Meaning == "" || body.Example == "" || len(body.Word) > 160 || len(body.Collocation) > 255 {
			writeError(w, 422, "Isi kata, arti, dan contoh kalimat dengan lengkap.")
			return
		}
		now := time.Now().UTC()
		result, err := a.db.ExecContext(r.Context(), `INSERT INTO vocabulary_cards (user_id,word,meaning,example,collocation,mastery,next_review_at,created_at,updated_at) VALUES (?,?,?,?,?,0,?,?,?)`, userID(r.Context()), body.Word, body.Meaning, body.Example, body.Collocation, now, now, now)
		if err != nil {
			writeError(w, 409, "Kata tersebut sudah ada atau belum dapat disimpan.")
			return
		}
		id, _ := result.LastInsertId()
		writeJSON(w, http.StatusCreated, vocabularyCard{ID: id, Word: body.Word, Meaning: body.Meaning, Example: body.Example, Collocation: body.Collocation, NextReview: now, CreatedAt: now})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (a *app) vocabularyByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w, http.MethodPut)
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/vocabulary/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "review" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeError(w, 400, "ID kosakata tidak valid.")
		return
	}
	var body struct {
		Remembered bool `json:"remembered"`
	}
	if decodeJSON(w, r, &body) != nil {
		return
	}
	var mastery int
	if err := a.db.QueryRowContext(r.Context(), `SELECT mastery FROM vocabulary_cards WHERE id=? AND user_id=?`, id, userID(r.Context())).Scan(&mastery); errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Kosakata tidak ditemukan.")
		return
	} else if err != nil {
		writeError(w, 500, "Kosakata belum dapat diperbarui.")
		return
	}
	if body.Remembered {
		mastery = min(5, mastery+1)
	} else {
		mastery = 0
	}
	interval := reviewInterval(mastery, body.Remembered)
	next := time.Now().UTC().AddDate(0, 0, interval)
	_, err = a.db.ExecContext(r.Context(), `UPDATE vocabulary_cards SET mastery=?,next_review_at=?,updated_at=? WHERE id=? AND user_id=?`, mastery, next, time.Now().UTC(), id, userID(r.Context()))
	if err != nil {
		writeError(w, 500, "Kosakata belum dapat diperbarui.")
		return
	}
	writeJSON(w, 200, map[string]any{"mastery": mastery, "nextReviewAt": next})
}

func writingTaskPrompt(taskType string) (string, bool) {
	switch taskType {
	case "task1":
		return "The table shows the percentage of households using renewable energy in four regions in 2010, 2015, and 2025: North 18%, 27%, 49%; South 12%, 20%, 38%; East 25%, 34%, 57%; West 16%, 22%, 41%. Summarise the main features and make relevant comparisons.", true
	case "task2":
		return "Some people believe schools should focus primarily on practical skills, while others believe academic subjects remain more important. Discuss both views and give your opinion.", true
	default:
		return "", false
	}
}

func (a *app) writingPrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	taskType := r.URL.Query().Get("type")
	prompt, ok := writingTaskPrompt(taskType)
	if !ok {
		writeError(w, 422, "Pilih Writing Task 1 atau Task 2.")
		return
	}
	writeJSON(w, 200, map[string]string{"taskType": taskType, "prompt": prompt})
}

func (a *app) writing(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.listWriting(w, r)
	case http.MethodPost:
		a.submitWriting(w, r)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (a *app) submitWriting(w http.ResponseWriter, r *http.Request) {
	if !a.allow(r) {
		writeError(w, http.StatusTooManyRequests, "Terlalu banyak permintaan penilaian. Tunggu satu menit lalu coba lagi.")
		return
	}
	var body struct {
		TaskType string `json:"taskType"`
		Prompt   string `json:"prompt"`
		Response string `json:"response"`
	}
	if decodeJSON(w, r, &body) != nil {
		return
	}
	expected, ok := writingTaskPrompt(body.TaskType)
	if !ok || strings.TrimSpace(body.Prompt) != expected {
		writeError(w, 422, "Prompt writing tidak valid. Muat ulang prompt lalu coba lagi.")
		return
	}
	body.Response = strings.TrimSpace(body.Response)
	words := countWords(body.Response)
	if words < 50 || words > 1200 {
		writeError(w, 422, "Tulisan harus berisi 50–1.200 kata agar dapat dinilai.")
		return
	}
	feedback, source, err := a.evaluateWriting(r.Context(), body.TaskType, body.Prompt, body.Response, words)
	if err != nil {
		writeError(w, 502, "Tulisan belum dapat dinilai. Periksa koneksi AI lalu coba lagi.")
		return
	}
	raw, _ := json.Marshal(feedback)
	now := time.Now().UTC()
	result, err := a.db.ExecContext(r.Context(), `INSERT INTO writing_submissions (user_id,task_type,prompt,response_text,word_count,overall_band,feedback_json,source,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, userID(r.Context()), body.TaskType, body.Prompt, body.Response, words, feedback.Overall, raw, source, now)
	if err != nil {
		writeError(w, 500, "Hasil writing belum dapat disimpan.")
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, http.StatusCreated, writingSubmission{ID: id, TaskType: body.TaskType, Prompt: body.Prompt, Response: body.Response, WordCount: words, Feedback: feedback, Source: source, CreatedAt: now})
}

func (a *app) listWriting(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,task_type,prompt,response_text,word_count,feedback_json,source,created_at FROM writing_submissions WHERE user_id=? ORDER BY created_at DESC LIMIT 20`, userID(r.Context()))
	if err != nil {
		writeError(w, 500, "Riwayat writing belum dapat dibuka.")
		return
	}
	defer rows.Close()
	items := []writingSubmission{}
	for rows.Next() {
		var item writingSubmission
		var raw []byte
		if rows.Scan(&item.ID, &item.TaskType, &item.Prompt, &item.Response, &item.WordCount, &raw, &item.Source, &item.CreatedAt) == nil && json.Unmarshal(raw, &item.Feedback) == nil {
			items = append(items, item)
		}
	}
	writeJSON(w, 200, items)
}

func (a *app) evaluateWriting(ctx context.Context, taskType, prompt, response string, words int) (writingFeedback, string, error) {
	if a.cfg.DeepSeekKey == "" {
		return demoWritingFeedback(response, words), "demo", nil
	}
	instruction := fmt.Sprintf(`Evaluate this IELTS %s response for an Indonesian learner. Prompt: %s\nResponse: %s\nReturn JSON only with scores from 0 to 9 in 0.5 steps for taskResponse, coherence, lexicalResource, grammar, and overallBand; summary in Indonesian; strengths, nextSteps, and corrections as arrays of concise Indonesian strings. Do not rewrite the entire essay.`, taskType, prompt, response)
	payload := map[string]any{"model": a.cfg.DeepSeekModel, "temperature": 0.2, "response_format": map[string]string{"type": "json_object"}, "messages": []map[string]string{{"role": "system", "content": "You are a careful IELTS writing evaluator. Scores are estimates, not official IELTS results."}, {"role": "user", "content": instruction}}}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.DeepSeekBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return writingFeedback{}, "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.DeepSeekKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return writingFeedback{}, "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return writingFeedback{}, "", errors.New("writing evaluation failed")
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &completion) != nil || len(completion.Choices) == 0 {
		return writingFeedback{}, "", errors.New("invalid writing completion")
	}
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	content = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(content, "```json"), "```"), "```")
	var feedback writingFeedback
	if json.Unmarshal([]byte(strings.TrimSpace(content)), &feedback) != nil || feedback.Summary == "" || feedback.Overall < 0 || feedback.Overall > 9 {
		return writingFeedback{}, "", errors.New("invalid writing feedback")
	}
	return feedback, "deepseek", nil
}

func demoWritingFeedback(response string, words int) writingFeedback {
	paragraphs := 0
	for _, paragraph := range strings.Split(response, "\n") {
		if strings.TrimSpace(paragraph) != "" {
			paragraphs++
		}
	}
	base := 5
	if words >= 150 {
		base++
	}
	if paragraphs >= 3 {
		base++
	}
	base = min(7, base)
	return writingFeedback{
		TaskResponse: float64(base), Coherence: float64(max(4, base-1)), Lexical: float64(base), Grammar: float64(max(4, base-1)), Overall: float64(base) - .5,
		Summary:     "Ini perkiraan lokal berdasarkan panjang dan struktur tulisan, bukan skor resmi IELTS. Aktifkan DeepSeek untuk evaluasi bahasa yang lebih rinci.",
		Strengths:   []string{"Tulisan memenuhi panjang minimum untuk menerima umpan balik.", "Gagasan sudah dituangkan dalam bentuk respons utuh."},
		NextSteps:   []string{"Pastikan setiap paragraf memiliki satu gagasan utama dan bukti pendukung.", "Periksa ulang subject–verb agreement dan penggunaan artikel."},
		Corrections: []string{"Mode demo tidak mengubah kalimat spesifik. Aktifkan DeepSeek untuk koreksi berbasis teks."},
	}
}

func countWords(value string) int {
	return len(strings.FieldsFunc(value, func(r rune) bool { return unicode.IsSpace(r) }))
}

func (a *app) diagnostic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	uid := userID(r.Context())
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "Tes diagnostik belum dapat disiapkan.")
		return
	}
	defer tx.Rollback()
	if uid > 0 {
		var lockedUserID int64
		err = tx.QueryRowContext(r.Context(), `SELECT id FROM users WHERE id=? FOR UPDATE`, uid).Scan(&lockedUserID)
	}
	if err != nil {
		writeError(w, 500, "Riwayat soal diagnostik belum dapat diperiksa.")
		return
	}
	questions, err := a.selectBankQuestions(r.Context(), tx, uid, generateInput{Level: "B1", IELTSTarget: 6.5, Count: 12, DurationMinutes: 25, Types: []string{"grammar", "vocabulary", "reading", "fill_blank"}})
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	s, err := storeQuestionSessionTx(r.Context(), tx, uid, questions, "diagnostic", "B1", 6.5, 25)
	if err != nil {
		writeError(w, 500, "Tes diagnostik belum dapat dibuat.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, "Tes diagnostik belum dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusCreated, sessionResponse{Session: s, Notice: "Tes diagnostik singkat ini memberi perkiraan awal, bukan hasil resmi CEFR atau IELTS."})
}

func (a *app) saveReflection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w, http.MethodPut)
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/reflections/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	index, err := strconv.Atoi(parts[1])
	if err != nil || index < 0 {
		writeError(w, 400, "Nomor soal tidak valid.")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if decodeJSON(w, r, &body) != nil {
		return
	}
	allowed := []string{"vocabulary", "inference", "detail", "rushed"}
	if !slicesContains(allowed, body.Reason) {
		writeError(w, 422, "Pilih penyebab kesalahan yang tersedia.")
		return
	}
	s, err := a.loadSession(r.Context(), parts[0])
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Sesi tidak ditemukan.")
		return
	}
	if err != nil {
		writeError(w, 500, "Sesi belum dapat dibuka.")
		return
	}
	if s.Status != "completed" || index >= len(s.Questions) || s.Questions[index].Type != "reading" {
		writeError(w, 422, "Refleksi hanya dapat disimpan untuk soal reading yang sudah dinilai.")
		return
	}
	now := time.Now().UTC()
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO reading_reflections (session_id,question_index,reason,created_at) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE reason=VALUES(reason),created_at=VALUES(created_at)`, parts[0], index, body.Reason, now)
	if err != nil {
		writeError(w, 500, "Refleksi reading belum dapat disimpan.")
		return
	}
	writeJSON(w, 200, map[string]bool{"saved": true})
}

func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
