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
	"log"
	"math"
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

type SentenceFeedback struct {
	Index      int    `json:"index"`
	Sentence   string `json:"sentence"`
	Status     string `json:"status"` // "strong", "warning", "error"
	Feedback   string `json:"feedback"`
	Suggestion string `json:"suggestion,omitempty"`
}

type StructuralAnalysis struct {
	HasOverview             bool     `json:"hasOverview"`
	OverviewQuote           string   `json:"overviewQuote,omitempty"`
	HasThesis               bool     `json:"hasThesis"`
	ThesisQuote             string   `json:"thesisQuote,omitempty"`
	ParagraphTopicSentences []string `json:"paragraphTopicSentences"`
}

type LexicalRepetition struct {
	Word              string   `json:"word"`
	Count             int      `json:"count"`
	SuggestedSynonyms []string `json:"suggestedSynonyms"`
}

type writingFeedback struct {
	TaskResponse float64             `json:"taskResponse"`
	Coherence    float64             `json:"coherence"`
	Lexical      float64             `json:"lexicalResource"`
	Grammar      float64             `json:"grammar"`
	Overall      float64             `json:"overallBand"`
	Summary      string              `json:"summary"`
	Strengths    []string            `json:"strengths"`
	NextSteps    []string            `json:"nextSteps"`
	Corrections  []string            `json:"corrections"`
	Sentences    []SentenceFeedback  `json:"sentences,omitempty"`
	Structure    StructuralAnalysis  `json:"structure"`
	Repetitions  []LexicalRepetition `json:"repetitions,omitempty"`
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
	if err := insertSessionQuestions(ctx, tx, s.ID, questions); err != nil {
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
	now := time.Now().UTC()
	if err := a.db.QueryRowContext(r.Context(), `SELECT
		(SELECT COUNT(*) FROM practice_sessions WHERE user_id=? AND status='completed'),
		(SELECT AVG(score) FROM practice_sessions WHERE user_id=? AND status='completed'),
		(SELECT COUNT(*) FROM review_cards WHERE user_id=? AND next_review_at<=?),
		(SELECT COUNT(*) FROM vocabulary_cards WHERE user_id=?),
		(SELECT COUNT(*) FROM writing_submissions WHERE user_id=?)`, uid, uid, uid, now, uid, uid).
		Scan(&out.CompletedSessions, &average, &out.DueReviews, &out.VocabularyCount, &out.WritingCount); err != nil {
		writeError(w, 500, "Dashboard belum dapat dibuka.")
		return
	}
	if average.Valid {
		value := average.Float64
		out.AverageScore = &value
	}
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
	rows, err := a.db.QueryContext(ctx, `SELECT q.question_type,COUNT(*),
		SUM(CASE WHEN a.selected_index=q.correct_index THEN 0 ELSE 1 END)
		FROM (
			SELECT id FROM practice_sessions
			WHERE user_id=? AND status='completed'
			ORDER BY created_at DESC LIMIT 50
		) recent
		JOIN practice_session_questions q ON q.session_id=recent.id
		LEFT JOIN practice_answers a ON a.session_id=q.session_id AND a.question_index=q.question_index
		GROUP BY q.question_type`, userID(ctx))
	if err != nil {
		return []weaknessStat{}
	}
	defer rows.Close()
	out := []weaknessStat{}
	for rows.Next() {
		var item weaknessStat
		if rows.Scan(&item.Type, &item.Total, &item.Wrong) == nil && item.Total > 0 {
			item.Accuracy = int(float64(item.Total-item.Wrong)*100/float64(item.Total) + .5)
			out = append(out, item)
		}
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
		p := getCategorizedTask1Prompts()
		if len(p) > 0 {
			return p[0].Prompt, true
		}
	case "task2":
		p := getCategorizedTask2Prompts()
		if len(p) > 0 {
			return p[0].Prompt, true
		}
	}
	return "", false
}

func demoWritingFeedback(response string, words int) writingFeedback {
	return evaluateWritingResponse("task2", "standard prompt", response)
}

func (a *app) writingPromptsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	t1 := getCategorizedTask1Prompts()
	t2 := getCategorizedTask2Prompts()
	writeJSON(w, 200, map[string]any{
		"task1Prompts": t1,
		"task2Prompts": t2,
	})
}

func (a *app) writingPrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	taskType := r.URL.Query().Get("type")
	promptID := r.URL.Query().Get("id")

	if promptID != "" {
		for _, p := range append(getCategorizedTask1Prompts(), getCategorizedTask2Prompts()...) {
			if p.ID == promptID {
				writeJSON(w, 200, p)
				return
			}
		}
	}

	if taskType == "task1" {
		prompts := getCategorizedTask1Prompts()
		writeJSON(w, 200, prompts[randInt(len(prompts))])
		return
	}
	if taskType == "task2" {
		prompts := getCategorizedTask2Prompts()
		writeJSON(w, 200, prompts[randInt(len(prompts))])
		return
	}

	writeError(w, 422, "Pilih Writing Task 1 atau Task 2.")
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
	body.Response = strings.TrimSpace(body.Response)
	words := countWords(body.Response)
	if words < 30 || words > 1500 {
		writeError(w, 422, "Tulisan harus berisi minimal 30 kata agar dapat dianalisis.")
		return
	}
	feedback, source, err := a.evaluateWriting(r.Context(), body.TaskType, body.Prompt, body.Response, words)
	if err != nil {
		log.Printf("evaluate writing err: %v", err)
		writeError(w, 502, "Tulisan belum dapat dinilai. Silakan coba beberapa saat lagi.")
		return
	}
	raw, _ := json.Marshal(feedback)
	now := time.Now().UTC()
	result, err := a.db.ExecContext(r.Context(), `
		INSERT INTO writing_submissions (user_id,task_type,prompt,response_text,word_count,overall_band,feedback_json,source,created_at)
		VALUES (?,?,?,?,?,?,?,?,?)
	`, userID(r.Context()), body.TaskType, body.Prompt, body.Response, words, feedback.Overall, raw, source, now)
	if err != nil {
		log.Printf("insert writing submission err: %v", err)
		writeError(w, 500, "Hasil writing belum dapat disimpan.")
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, http.StatusCreated, writingSubmission{
		ID:        id,
		TaskType:  body.TaskType,
		Prompt:    body.Prompt,
		Response:  body.Response,
		WordCount: words,
		Feedback:  feedback,
		Source:    source,
		CreatedAt: now,
	})
}

type RevisionDiffItem struct {
	Type     string `json:"type"` // "added", "removed", "modified", "unchanged"
	Original string `json:"original,omitempty"`
	Revised  string `json:"revised,omitempty"`
}

func (a *app) submitWritingRevision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	uid := userID(r.Context())
	var body struct {
		SubmissionID int64  `json:"submissionId"`
		RevisedText  string `json:"revisedText"`
	}
	if decodeJSON(w, r, &body) != nil {
		return
	}

	body.RevisedText = strings.TrimSpace(body.RevisedText)
	revisedWords := countWords(body.RevisedText)
	if revisedWords < 30 {
		writeError(w, 422, "Draf revisi minimal 30 kata.")
		return
	}

	// Fetch original first-draft submission
	var origTaskType, origPrompt, origResponse string
	var origFeedbackRaw []byte
	err := a.db.QueryRowContext(r.Context(), `
		SELECT task_type, prompt, response_text, feedback_json
		FROM writing_submissions
		WHERE id = ? AND user_id = ?
	`, body.SubmissionID, uid).Scan(&origTaskType, &origPrompt, &origResponse, &origFeedbackRaw)

	if err != nil {
		writeError(w, 404, "Draf pertama tidak ditemukan.")
		return
	}

	var origFeedback writingFeedback
	json.Unmarshal(origFeedbackRaw, &origFeedback)

	// Evaluate revised draft
	revFeedback, source, err := a.evaluateWriting(r.Context(), origTaskType, origPrompt, body.RevisedText, revisedWords)
	if err != nil {
		writeError(w, 502, "Draf revisi belum dapat dinilai.")
		return
	}

	// Compute simple sentence diffs
	origSentences := splitSentences(origResponse)
	revSentences := splitSentences(body.RevisedText)
	var diffs []RevisionDiffItem

	maxLen := max(len(origSentences), len(revSentences))
	for i := 0; i < maxLen; i++ {
		var o, n string
		if i < len(origSentences) {
			o = origSentences[i]
		}
		if i < len(revSentences) {
			n = revSentences[i]
		}
		if o == n && o != "" {
			diffs = append(diffs, RevisionDiffItem{Type: "unchanged", Original: o, Revised: n})
		} else if o != "" && n != "" {
			diffs = append(diffs, RevisionDiffItem{Type: "modified", Original: o, Revised: n})
		} else if o == "" && n != "" {
			diffs = append(diffs, RevisionDiffItem{Type: "added", Revised: n})
		} else if o != "" && n == "" {
			diffs = append(diffs, RevisionDiffItem{Type: "removed", Original: o})
		}
	}

	diffRaw, _ := json.Marshal(diffs)
	revFeedbackRaw, _ := json.Marshal(revFeedback)
	now := time.Now().UTC()

	result, err := a.db.ExecContext(r.Context(), `
		INSERT INTO writing_revisions (submission_id, user_id, revision_number, revised_text, word_count, overall_band, feedback_json, diff_json, created_at)
		VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?)
	`, body.SubmissionID, uid, body.RevisedText, revisedWords, revFeedback.Overall, revFeedbackRaw, diffRaw, now)

	if err != nil {
		log.Printf("insert revision err: %v", err)
	}
	revID, _ := result.LastInsertId()

	writeJSON(w, 200, map[string]any{
		"revisionId":      revID,
		"submissionId":    body.SubmissionID,
		"revisedText":     body.RevisedText,
		"revisedWords":    revisedWords,
		"firstDraftBand":  origFeedback.Overall,
		"revisedBand":     revFeedback.Overall,
		"bandDelta":       math.Round((revFeedback.Overall-origFeedback.Overall)*10) / 10,
		"diffs":           diffs,
		"firstFeedback":   origFeedback,
		"revisedFeedback": revFeedback,
		"source":          source,
	})
}

func (a *app) listWriting(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT id,task_type,prompt,response_text,word_count,feedback_json,source,created_at
		FROM writing_submissions
		WHERE user_id=?
		ORDER BY created_at DESC
		LIMIT 20
	`, userID(r.Context()))
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
	if a.cfg.DeepSeekKey != "" {
		instruction := fmt.Sprintf(`Evaluate this IELTS %s response for an Indonesian learner.
Prompt: %s
Response: %s

Return valid JSON with:
- "taskResponse": number (0-9, step 0.5)
- "coherence": number (0-9, step 0.5)
- "lexicalResource": number (0-9, step 0.5)
- "grammar": number (0-9, step 0.5)
- "overallBand": number (0-9, step 0.5)
- "summary": string in Indonesian
- "strengths": array of Indonesian strings
- "nextSteps": array of Indonesian strings
- "corrections": array of Indonesian strings
- "sentences": array of objects with { "index": int, "sentence": string, "status": "strong"|"warning"|"error", "feedback": string, "suggestion": string }`, taskType, prompt, response)

		payload := map[string]any{
			"model":           a.cfg.DeepSeekModel,
			"temperature":     0.2,
			"response_format": map[string]string{"type": "json_object"},
			"messages": []map[string]string{
				{"role": "system", "content": "You are an expert IELTS Writing examiner assessing Task 1 and Task 2 with strict rubric adherence."},
				{"role": "user", "content": instruction},
			},
		}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.DeepSeekBaseURL+"/chat/completions", bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+a.cfg.DeepSeekKey)
			req.Header.Set("Content-Type", "application/json")
			resp, err := a.client.Do(req)
			if err == nil && resp.StatusCode == 200 {
				defer resp.Body.Close()
				raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
				var completion struct {
					Choices []struct {
						Message struct {
							Content string `json:"content"`
						} `json:"message"`
					} `json:"choices"`
				}
				if json.Unmarshal(raw, &completion) == nil && len(completion.Choices) > 0 {
					content := strings.TrimSpace(completion.Choices[0].Message.Content)
					content = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(content, "```json"), "```"), "```")
					var fb writingFeedback
					if json.Unmarshal([]byte(strings.TrimSpace(content)), &fb) == nil && fb.Summary != "" && fb.Overall >= 1 && fb.Overall <= 9 {
						// Enrich with structural analysis and repetition check
						fb.Structure = analyzeEssayStructure(taskType, response)
						fb.Repetitions = detectLexicalRepetitions(response)
						return fb, "deepseek", nil
					}
				}
			}
		}
	}

	return evaluateWritingResponse(taskType, prompt, response), "local_engine", nil
}

func evaluateWritingResponse(taskType, prompt, response string) writingFeedback {
	words := countWords(response)
	sentences := splitSentences(response)
	paragraphs := 0
	for _, p := range strings.Split(response, "\n") {
		if strings.TrimSpace(p) != "" {
			paragraphs++
		}
	}

	structure := analyzeEssayStructure(taskType, response)
	repetitions := detectLexicalRepetitions(response)

	// Base score calculation based on length, structure, and lexical variety
	tr := 5.0
	cc := 5.0
	lr := 5.0
	gr := 5.0

	// Task 1 length benchmark: 150 words; Task 2: 250 words
	if taskType == "task1" {
		if words >= 150 {
			tr += 1.0
		} else if words >= 120 {
			tr += 0.5
		}
		if structure.HasOverview {
			tr += 1.0
			cc += 0.5
		}
	} else {
		if words >= 250 {
			tr += 1.0
		} else if words >= 200 {
			tr += 0.5
		}
		if structure.HasThesis {
			tr += 1.0
		}
	}

	if paragraphs >= 4 {
		cc += 1.0
	} else if paragraphs >= 3 {
		cc += 0.5
	}

	// Lexical score
	if len(repetitions) <= 2 && words >= 150 {
		lr += 1.0
	} else if len(repetitions) > 5 {
		lr -= 0.5
	}

	// Sentence-level analysis
	var sentenceItems []SentenceFeedback
	errorCount := 0
	for idx, s := range sentences {
		sTrim := strings.TrimSpace(s)
		if sTrim == "" {
			continue
		}
		status := "strong"
		feedbackText := "Struktur kalimat lugas dan mendukung alur paragraf."
		suggestion := ""

		lower := strings.ToLower(sTrim)
		// Check common grammar traps
		if strings.Contains(lower, "despite of") {
			status = "error"
			feedbackText = "Kekeliruan preposisi: 'despite of' tidak baku."
			suggestion = strings.ReplaceAll(sTrim, "despite of", "despite")
			errorCount++
		} else if strings.Contains(lower, "very good") || strings.Contains(lower, "very bad") || strings.Contains(lower, "a lot of") {
			status = "warning"
			feedbackText = "Gunakan leksikon akademis yang lebih presisi untuk meningkatkan Lexical Resource."
			suggestion = "Pertimbangkan kata seperti 'highly beneficial', 'detrimental', atau 'a substantial proportion'."
		} else if len(strings.Fields(sTrim)) > 35 {
			status = "warning"
			feedbackText = "Kalimat terlalu panjang (run-on sentence). Pertimbangkan membaginya menjadi dua kalimat majemuk."
		}

		sentenceItems = append(sentenceItems, SentenceFeedback{
			Index:      idx + 1,
			Sentence:   sTrim,
			Status:     status,
			Feedback:   feedbackText,
			Suggestion: suggestion,
		})
	}

	if errorCount == 0 && words >= 180 {
		gr += 1.0
	} else if errorCount > 3 {
		gr -= 1.0
	}

	tr = math.Min(8.5, math.Max(4.0, tr))
	cc = math.Min(8.5, math.Max(4.0, cc))
	lr = math.Min(8.5, math.Max(4.0, lr))
	gr = math.Min(8.5, math.Max(4.0, gr))

	overall := math.Round(((tr+cc+lr+gr)/4.0)*2.0) / 2.0

	var strengths []string
	var nextSteps []string
	var corrections []string

	if taskType == "task1" {
		if structure.HasOverview {
			strengths = append(strengths, "Overview berhasil diidentifikasi dengan jelas, menyajikan tren dan perbandingan utama.")
		} else {
			nextSteps = append(nextSteps, "Wajib menambahkan satu paragraf/kalimat Overview yang merangkum tren umum tanpa mencantumkan detail angka.")
		}
	} else {
		if structure.HasThesis {
			strengths = append(strengths, "Thesis statement pada paragraf pengantar menyajikan posisi argumen secara tegas.")
		} else {
			nextSteps = append(nextSteps, "Sertakan Thesis Statement yang eksplisit di paragraf introduksi untuk menegaskan posisi Anda.")
		}
	}

	if words >= 250 || (taskType == "task1" && words >= 150) {
		strengths = append(strengths, fmt.Sprintf("Panjang tulisan (%d kata) memenuhi batas minimum standar ujian IELTS.", words))
	} else {
		nextSteps = append(nextSteps, fmt.Sprintf("Panjang tulisan (%d kata) masih di bawah batas aman. Perluas elaborasi ide.", words))
	}

	if len(repetitions) > 0 {
		var repWords []string
		for _, r := range repetitions {
			repWords = append(repWords, fmt.Sprintf("'%s' (%dx)", r.Word, r.Count))
		}
		nextSteps = append(nextSteps, fmt.Sprintf("Kurangi repetisi kata yang sering berulang: %s. Gunakan variasi sinonim akademis.", strings.Join(repWords, ", ")))
	}

	corrections = append(corrections, "Gunakan kata penghubung transisi formal (e.g. 'Furthermore', 'Conversely', 'Consequently') di awal kalimat topik.")

	summary := fmt.Sprintf("Estimasi skor keseluruhan Band %.1f (TR: %.1f, CC: %.1f, LR: %.1f, GRA: %.1f). Tulisan terstruktur dengan %d paragraf dan %d kata.",
		overall, tr, cc, lr, gr, paragraphs, words)

	return writingFeedback{
		TaskResponse: tr,
		Coherence:    cc,
		Lexical:      lr,
		Grammar:      gr,
		Overall:      overall,
		Summary:      summary,
		Strengths:    strengths,
		NextSteps:    nextSteps,
		Corrections:  corrections,
		Sentences:    sentenceItems,
		Structure:    structure,
		Repetitions:  repetitions,
	}
}

func analyzeEssayStructure(taskType, text string) StructuralAnalysis {
	var analysis StructuralAnalysis
	paragraphs := strings.Split(text, "\n")
	for _, p := range paragraphs {
		pTrim := strings.TrimSpace(p)
		if pTrim == "" {
			continue
		}
		sents := splitSentences(pTrim)
		if len(sents) > 0 {
			analysis.ParagraphTopicSentences = append(analysis.ParagraphTopicSentences, sents[0])
		}
	}

	lower := strings.ToLower(text)
	// Task 1 Overview detection
	overviewKeywords := []string{"overall,", "in general,", "it is noticeable that", "a salient feature", "it can be seen that", "to summarize,"}
	for _, kw := range overviewKeywords {
		if strings.Contains(lower, kw) {
			analysis.HasOverview = true
			analysis.OverviewQuote = extractSentenceContaining(text, kw)
			break
		}
	}

	// Task 2 Thesis detection
	thesisKeywords := []string{"i agree", "i disagree", "in my opinion", "i believe", "this essay will argue", "my view is", "i would argue that"}
	for _, kw := range thesisKeywords {
		if strings.Contains(lower, kw) {
			analysis.HasThesis = true
			analysis.ThesisQuote = extractSentenceContaining(text, kw)
			break
		}
	}

	return analysis
}

func extractSentenceContaining(text, keyword string) string {
	for _, s := range splitSentences(text) {
		if strings.Contains(strings.ToLower(s), keyword) {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func detectLexicalRepetitions(text string) []LexicalRepetition {
	commonWords := map[string][]string{
		"people":    {"individuals", "citizens", "the general public", "populations"},
		"good":      {"beneficial", "advantageous", "favorable", "salutary"},
		"bad":       {"detrimental", "adverse", "unfavorable", "pernicious"},
		"important": {"crucial", "pivotal", "paramount", "indispensable"},
		"increase":  {"escalate", "proliferate", "augment", "surge"},
		"decrease":  {"diminish", "dwindle", "curtail", "decline"},
		"problem":   {"dilemma", "impediment", "predicament", "challenge"},
		"make":      {"generate", "construct", "implement", "execute"},
		"show":      {"illustrate", "demonstrate", "depict", "reveal"},
	}

	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	counts := make(map[string]int)
	for _, w := range words {
		if len(w) >= 3 {
			counts[w]++
		}
	}

	var results []LexicalRepetition
	for target, syns := range commonWords {
		if count := counts[target]; count >= 3 {
			results = append(results, LexicalRepetition{
				Word:              target,
				Count:             count,
				SuggestedSynonyms: syns,
			})
		}
	}
	return results
}

func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		current.WriteRune(ch)
		if ch == '.' || ch == '?' || ch == '!' {
			// Check if next char is whitespace or end of string
			if i+1 == len(runes) || unicode.IsSpace(runes[i+1]) {
				s := strings.TrimSpace(current.String())
				if s != "" {
					sentences = append(sentences, s)
				}
				current.Reset()
			}
		}
	}
	s := strings.TrimSpace(current.String())
	if s != "" {
		sentences = append(sentences, s)
	}
	return sentences
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
	questions, err := a.selectBankQuestions(r.Context(), a.db, uid, generateInput{Level: "B1", IELTSTarget: 6.5, Count: 12, DurationMinutes: 25, Types: []string{"grammar", "vocabulary", "reading", "fill_blank"}})
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "Tes diagnostik belum dapat disiapkan.")
		return
	}
	defer tx.Rollback()
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
