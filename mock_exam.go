package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Standard IELTS Raw Score to Band Conversion Table (Academic / Standard 40-question scale)
var rawToBandTableListening = map[int]float64{
	40: 9.0, 39: 9.0,
	38: 8.5, 37: 8.5,
	36: 8.0, 35: 8.0,
	34: 7.5, 33: 7.5, 32: 7.5,
	31: 7.0, 30: 7.0,
	29: 6.5, 28: 6.5, 27: 6.5, 26: 6.5,
	25: 6.0, 24: 6.0, 23: 6.0,
	22: 5.5, 21: 5.5, 20: 5.5, 19: 5.5, 18: 5.5,
	17: 5.0, 16: 5.0,
	15: 4.5, 14: 4.5, 13: 4.5,
	12: 4.0, 11: 4.0, 10: 4.0,
	9: 3.5, 8: 3.5, 7: 3.5, 6: 3.5,
	5: 3.0, 4: 3.0,
	3: 2.5, 2: 2.5,
	1: 2.0, 0: 1.0,
}

var rawToBandTableReadingAcademic = map[int]float64{
	40: 9.0, 39: 9.0,
	38: 8.5, 37: 8.5,
	36: 8.0, 35: 8.0,
	34: 7.5, 33: 7.5,
	32: 7.0, 31: 7.0, 30: 7.0,
	29: 6.5, 28: 6.5, 27: 6.5,
	26: 6.0, 25: 6.0, 24: 6.0, 23: 6.0,
	22: 5.5, 21: 5.5, 20: 5.5, 19: 5.5,
	18: 5.0, 17: 5.0, 16: 5.0, 15: 5.0,
	14: 4.5, 13: 4.5,
	12: 4.0, 11: 4.0, 10: 4.0,
	9: 3.5, 8: 3.5, 7: 3.5, 6: 3.5,
	5: 3.0, 4: 3.0,
	3: 2.5, 2: 2.5,
	1: 2.0, 0: 1.0,
}

func calculateBand(rawScore, totalQuestions int, sectionType string) float64 {
	if totalQuestions <= 0 {
		return 0
	}
	// Normalize to standard 40-question scale if modular
	normalizedRaw := int(math.Round(float64(rawScore) * 40.0 / float64(totalQuestions)))
	if normalizedRaw > 40 {
		normalizedRaw = 40
	}
	if normalizedRaw < 0 {
		normalizedRaw = 0
	}

	if sectionType == "listening" {
		if b, ok := rawToBandTableListening[normalizedRaw]; ok {
			return b
		}
	} else {
		if b, ok := rawToBandTableReadingAcademic[normalizedRaw]; ok {
			return b
		}
	}
	return 4.0
}

type MockTestSectionConfig struct {
	Section         string `json:"section"`         // listening, reading, writing
	DurationMinutes int    `json:"durationMinutes"` // 30, 60, 60
	QuestionCount   int    `json:"questionCount"`   // e.g. 20-40
}

type MockTestQuestions struct {
	Listening []question      `json:"listening,omitempty"`
	Reading   []question      `json:"reading,omitempty"`
	Writing   []WritingPrompt `json:"writing,omitempty"`
}

type MockTestAnswers struct {
	Listening map[string]int    `json:"listening"`         // questionIndex (as string) -> selectedIndex
	Reading   map[string]int    `json:"reading"`           // questionIndex (as string) -> selectedIndex
	Writing   map[string]string `json:"writing,omitempty"` // task1, task2 -> essay text
	Flags     map[string]bool   `json:"flags,omitempty"`   // "listening_0", "reading_5" -> true
}

type SectionScoreReport struct {
	RawScore       int            `json:"rawScore"`
	TotalQuestions int            `json:"totalQuestions"`
	BandScore      float64        `json:"bandScore"`
	DetailedScores map[string]any `json:"detailedScores,omitempty"` // for writing criteria (TR, CC, LR, GRA)
}

type MockTestSessionRecord struct {
	ID                   string                        `json:"id"`
	UserID               int64                         `json:"userId"`
	Title                string                        `json:"title"`
	ModuleType           string                        `json:"moduleType"`
	Status               string                        `json:"status"` // in_progress, completed, expired
	CurrentSection       string                        `json:"currentSection"`
	StartedAt            time.Time                     `json:"startedAt"`
	CompletedAt          *time.Time                    `json:"completedAt,omitempty"`
	TimeRemainingSeconds int                           `json:"timeRemainingSeconds"`
	Config               []MockTestSectionConfig       `json:"config"`
	Questions            MockTestQuestions             `json:"questions"`
	Answers              MockTestAnswers               `json:"answers"`
	SectionScores        map[string]SectionScoreReport `json:"sectionScores"`
	OverallBand          *float64                      `json:"overallBand,omitempty"`
	CreatedAt            time.Time                     `json:"createdAt"`
	UpdatedAt            time.Time                     `json:"updatedAt"`
}

// Public safe view for mock test (conceals correct answers and explanations until completed)
type MockTestPublicView struct {
	ID                   string                        `json:"id"`
	Title                string                        `json:"title"`
	ModuleType           string                        `json:"moduleType"`
	Status               string                        `json:"status"`
	CurrentSection       string                        `json:"currentSection"`
	StartedAt            time.Time                     `json:"startedAt"`
	CompletedAt          *time.Time                    `json:"completedAt,omitempty"`
	TimeRemainingSeconds int                           `json:"timeRemainingSeconds"`
	Config               []MockTestSectionConfig       `json:"config"`
	ListeningQuestions   []publicQuestion              `json:"listeningQuestions,omitempty"`
	ReadingQuestions     []publicQuestion              `json:"readingQuestions,omitempty"`
	WritingTasks         []WritingPrompt               `json:"writingTasks,omitempty"`
	Answers              MockTestAnswers               `json:"answers"`
	SectionScores        map[string]SectionScoreReport `json:"sectionScores,omitempty"`
	OverallBand          *float64                      `json:"overallBand,omitempty"`
	// Full review questions only provided when status == 'completed'
	FullReviewQuestions *MockTestQuestions `json:"fullReviewQuestions,omitempty"`
}

func (a *app) mockTests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.listMockTests(w, r)
	case http.MethodPost:
		a.createMockTest(w, r)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (a *app) mockTestByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/mock-tests/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, 400, "ID simulasi tes tidak valid.")
		return
	}
	mockID := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			a.getMockTest(w, r, mockID)
		default:
			methodNotAllowed(w, http.MethodGet)
		}
		return
	}

	action := parts[1]
	switch action {
	case "autosave":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		a.autosaveMockTest(w, r, mockID)
	case "submit-section":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		a.submitMockTestSection(w, r, mockID)
	case "finish":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		a.finishMockTest(w, r, mockID)
	default:
		writeError(w, 404, "Aksi simulasi tes tidak ditemukan.")
	}
}

func (a *app) listMockTests(w http.ResponseWriter, r *http.Request) {
	uid := userID(r.Context())
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT id, title, module_type, status, current_section, started_at, completed_at, section_scores_json, overall_band, created_at
		FROM mock_test_sessions
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT 20
	`, uid)
	if err != nil {
		log.Printf("list mock tests: %v", err)
		writeError(w, 500, "Riwayat simulasi tes belum dapat dimuat.")
		return
	}
	defer rows.Close()

	type MockSummary struct {
		ID             string                        `json:"id"`
		Title          string                        `json:"title"`
		ModuleType     string                        `json:"moduleType"`
		Status         string                        `json:"status"`
		CurrentSection string                        `json:"currentSection"`
		StartedAt      time.Time                     `json:"startedAt"`
		CompletedAt    *time.Time                    `json:"completedAt,omitempty"`
		SectionScores  map[string]SectionScoreReport `json:"sectionScores,omitempty"`
		OverallBand    *float64                      `json:"overallBand,omitempty"`
		CreatedAt      time.Time                     `json:"createdAt"`
	}

	var results []MockSummary
	for rows.Next() {
		var item MockSummary
		var scoresRaw []byte
		if err := rows.Scan(&item.ID, &item.Title, &item.ModuleType, &item.Status, &item.CurrentSection, &item.StartedAt, &item.CompletedAt, &scoresRaw, &item.OverallBand, &item.CreatedAt); err != nil {
			log.Printf("scan mock test: %v", err)
			continue
		}
		if len(scoresRaw) > 0 {
			json.Unmarshal(scoresRaw, &item.SectionScores)
		}
		results = append(results, item)
	}

	if results == nil {
		results = []MockSummary{}
	}
	writeJSON(w, 200, map[string]any{"mockTests": results})
}

func (a *app) createMockTest(w http.ResponseWriter, r *http.Request) {
	uid := userID(r.Context())
	var body struct {
		Title      string `json:"title"`
		ModuleType string `json:"moduleType"` // academic (default), general
		FullLength bool   `json:"fullLength"` // full 40 questions per section vs compact 20 questions
	}
	if decodeJSON(w, r, &body) != nil {
		return
	}

	if body.Title == "" {
		body.Title = fmt.Sprintf("IELTS Academic Mock Test #%s", time.Now().Format("02 Jan 15:04"))
	}
	if body.ModuleType == "" {
		body.ModuleType = "academic"
	}

	countPerSection := 20
	if body.FullLength {
		countPerSection = 40
	}

	ctx := r.Context()
	// Pick unique questions from question bank for Listening and Reading
	listeningQuestions, err := a.selectBankQuestions(ctx, a.db, uid, generateInput{
		Level:       "B2",
		IELTSTarget: 7.0,
		Types:       []string{"listening"},
		Count:       countPerSection,
	})
	if err != nil {
		log.Printf("mock listening pick err: %v", err)
		writeError(w, 500, "Gagal mengalokasikan soal Listening untuk simulasi ujian.")
		return
	}

	readingQuestions, err := a.selectBankQuestions(ctx, a.db, uid, generateInput{
		Level:       "B2",
		IELTSTarget: 7.0,
		Types:       []string{"reading"},
		Count:       countPerSection,
	})
	if err != nil {
		log.Printf("mock reading pick err: %v", err)
		writeError(w, 500, "Gagal mengalokasikan soal Reading untuk simulasi ujian.")
		return
	}

	// Pick 1 Task 1 and 1 Task 2 prompt
	task1Prompts := getCategorizedTask1Prompts()
	task2Prompts := getCategorizedTask2Prompts()
	t1 := task1Prompts[randInt(len(task1Prompts))]
	t2 := task2Prompts[randInt(len(task2Prompts))]

	mockID := newID()
	now := time.Now().UTC()

	config := []MockTestSectionConfig{
		{Section: "listening", DurationMinutes: 30, QuestionCount: countPerSection},
		{Section: "reading", DurationMinutes: 60, QuestionCount: countPerSection},
		{Section: "writing", DurationMinutes: 60, QuestionCount: 2},
	}

	questionsObj := MockTestQuestions{
		Listening: listeningQuestions,
		Reading:   readingQuestions,
		Writing:   []WritingPrompt{t1, t2},
	}

	answersObj := MockTestAnswers{
		Listening: make(map[string]int),
		Reading:   make(map[string]int),
		Writing:   make(map[string]string),
		Flags:     make(map[string]bool),
	}

	configJSON, _ := json.Marshal(config)
	questionsJSON, _ := json.Marshal(questionsObj)
	answersJSON, _ := json.Marshal(answersObj)
	scoresJSON, _ := json.Marshal(map[string]SectionScoreReport{})

	totalSeconds := (30 + 60 + 60) * 60

	_, err = a.db.ExecContext(ctx, `
		INSERT INTO mock_test_sessions (
			id, user_id, title, module_type, status, current_section,
			started_at, time_remaining_seconds, config_json, questions_json,
			answers_json, section_scores_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'in_progress', 'listening', ?, ?, ?, ?, ?, ?, ?, ?)
	`, mockID, uid, body.Title, body.ModuleType, now, totalSeconds, configJSON, questionsJSON, answersJSON, scoresJSON, now, now)

	if err != nil {
		log.Printf("insert mock test session err: %v", err)
		writeError(w, 500, "Simulasi tes belum dapat dibuat.")
		return
	}

	pubView := MockTestPublicView{
		ID:                   mockID,
		Title:                body.Title,
		ModuleType:           body.ModuleType,
		Status:               "in_progress",
		CurrentSection:       "listening",
		StartedAt:            now,
		TimeRemainingSeconds: totalSeconds,
		Config:               config,
		ListeningQuestions:   publicQuestions(listeningQuestions),
		ReadingQuestions:     publicQuestions(readingQuestions),
		WritingTasks:         []WritingPrompt{t1, t2},
		Answers:              answersObj,
	}

	writeJSON(w, http.StatusCreated, map[string]any{"mockTest": pubView})
}

func (a *app) getMockTest(w http.ResponseWriter, r *http.Request, mockID string) {
	uid := userID(r.Context())
	var rec MockTestSessionRecord
	var configRaw, qRaw, aRaw, sRaw []byte

	err := a.db.QueryRowContext(r.Context(), `
		SELECT id, user_id, title, module_type, status, current_section,
		       started_at, completed_at, time_remaining_seconds, config_json,
		       questions_json, answers_json, section_scores_json, overall_band,
		       created_at, updated_at
		FROM mock_test_sessions
		WHERE id = ? AND user_id = ?
	`, mockID, uid).Scan(
		&rec.ID, &rec.UserID, &rec.Title, &rec.ModuleType, &rec.Status, &rec.CurrentSection,
		&rec.StartedAt, &rec.CompletedAt, &rec.TimeRemainingSeconds, &configRaw,
		&qRaw, &aRaw, &sRaw, &rec.OverallBand, &rec.CreatedAt, &rec.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, 404, "Simulasi tes tidak ditemukan.")
			return
		}
		log.Printf("get mock test err: %v", err)
		writeError(w, 500, "Simulasi tes belum dapat dimuat.")
		return
	}

	json.Unmarshal(configRaw, &rec.Config)
	json.Unmarshal(qRaw, &rec.Questions)
	json.Unmarshal(aRaw, &rec.Answers)
	json.Unmarshal(sRaw, &rec.SectionScores)

	pubView := MockTestPublicView{
		ID:                   rec.ID,
		Title:                rec.Title,
		ModuleType:           rec.ModuleType,
		Status:               rec.Status,
		CurrentSection:       rec.CurrentSection,
		StartedAt:            rec.StartedAt,
		CompletedAt:          rec.CompletedAt,
		TimeRemainingSeconds: rec.TimeRemainingSeconds,
		Config:               rec.Config,
		ListeningQuestions:   publicQuestions(rec.Questions.Listening),
		ReadingQuestions:     publicQuestions(rec.Questions.Reading),
		WritingTasks:         rec.Questions.Writing,
		Answers:              rec.Answers,
		SectionScores:        rec.SectionScores,
		OverallBand:          rec.OverallBand,
	}

	// Only provide full answer keys, context, and explanations if the test is completed
	if rec.Status == "completed" {
		pubView.FullReviewQuestions = &rec.Questions
	}

	writeJSON(w, 200, map[string]any{"mockTest": pubView})
}

func (a *app) autosaveMockTest(w http.ResponseWriter, r *http.Request, mockID string) {
	uid := userID(r.Context())
	var body struct {
		CurrentSection       string          `json:"currentSection"`
		TimeRemainingSeconds int             `json:"timeRemainingSeconds"`
		Answers              MockTestAnswers `json:"answers"`
	}
	if decodeJSON(w, r, &body) != nil {
		return
	}

	answersJSON, _ := json.Marshal(body.Answers)
	now := time.Now().UTC()

	_, err := a.db.ExecContext(r.Context(), `
		UPDATE mock_test_sessions
		SET current_section = ?, time_remaining_seconds = ?, answers_json = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND status = 'in_progress'
	`, body.CurrentSection, body.TimeRemainingSeconds, answersJSON, now, mockID, uid)

	if err != nil {
		log.Printf("autosave mock test err: %v", err)
		writeError(w, 500, "Gagal menyimpan progres simulasi tes.")
		return
	}

	writeJSON(w, 200, map[string]any{"saved": true, "timestamp": now})
}

func (a *app) submitMockTestSection(w http.ResponseWriter, r *http.Request, mockID string) {
	uid := userID(r.Context())
	var body struct {
		CompletedSection     string          `json:"completedSection"`
		NextSection          string          `json:"nextSection"`
		TimeRemainingSeconds int             `json:"timeRemainingSeconds"`
		Answers              MockTestAnswers `json:"answers"`
	}
	if decodeJSON(w, r, &body) != nil {
		return
	}

	var qRaw, sRaw []byte
	err := a.db.QueryRowContext(r.Context(), `
		SELECT questions_json, section_scores_json
		FROM mock_test_sessions
		WHERE id = ? AND user_id = ?
	`, mockID, uid).Scan(&qRaw, &sRaw)

	if err != nil {
		writeError(w, 404, "Simulasi tes tidak ditemukan.")
		return
	}

	var questions MockTestQuestions
	var scores map[string]SectionScoreReport
	json.Unmarshal(qRaw, &questions)
	json.Unmarshal(sRaw, &scores)
	if scores == nil {
		scores = make(map[string]SectionScoreReport)
	}

	// Grade the completed section
	switch body.CompletedSection {
	case "listening":
		raw := 0
		for i, q := range questions.Listening {
			ansKey := strconv.Itoa(i)
			if chosen, ok := body.Answers.Listening[ansKey]; ok && chosen == q.CorrectIndex {
				raw++
			}
		}
		band := calculateBand(raw, len(questions.Listening), "listening")
		scores["listening"] = SectionScoreReport{
			RawScore:       raw,
			TotalQuestions: len(questions.Listening),
			BandScore:      band,
		}
	case "reading":
		raw := 0
		for i, q := range questions.Reading {
			ansKey := strconv.Itoa(i)
			if chosen, ok := body.Answers.Reading[ansKey]; ok && chosen == q.CorrectIndex {
				raw++
			}
		}
		band := calculateBand(raw, len(questions.Reading), "reading")
		scores["reading"] = SectionScoreReport{
			RawScore:       raw,
			TotalQuestions: len(questions.Reading),
			BandScore:      band,
		}
	}

	answersJSON, _ := json.Marshal(body.Answers)
	scoresJSON, _ := json.Marshal(scores)
	now := time.Now().UTC()

	_, err = a.db.ExecContext(r.Context(), `
		UPDATE mock_test_sessions
		SET current_section = ?, time_remaining_seconds = ?, answers_json = ?, section_scores_json = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, body.NextSection, body.TimeRemainingSeconds, answersJSON, scoresJSON, now, mockID, uid)

	if err != nil {
		writeError(w, 500, "Gagal memperbarui bagian simulasi tes.")
		return
	}

	writeJSON(w, 200, map[string]any{
		"section":       body.CompletedSection,
		"sectionScore":  scores[body.CompletedSection],
		"nextSection":   body.NextSection,
		"sectionScores": scores,
	})
}

func (a *app) finishMockTest(w http.ResponseWriter, r *http.Request, mockID string) {
	uid := userID(r.Context())
	var body struct {
		Answers MockTestAnswers `json:"answers"`
	}
	decodeJSON(w, r, &body)

	var qRaw, sRaw []byte
	err := a.db.QueryRowContext(r.Context(), `
		SELECT questions_json, section_scores_json
		FROM mock_test_sessions
		WHERE id = ? AND user_id = ?
	`, mockID, uid).Scan(&qRaw, &sRaw)

	if err != nil {
		writeError(w, 404, "Simulasi tes tidak ditemukan.")
		return
	}

	var questions MockTestQuestions
	var scores map[string]SectionScoreReport
	json.Unmarshal(qRaw, &questions)
	json.Unmarshal(sRaw, &scores)
	if scores == nil {
		scores = make(map[string]SectionScoreReport)
	}

	// 1. Grade Listening
	rawL := 0
	for i, q := range questions.Listening {
		ansKey := strconv.Itoa(i)
		if chosen, ok := body.Answers.Listening[ansKey]; ok && chosen == q.CorrectIndex {
			rawL++
		}
	}
	bandL := calculateBand(rawL, len(questions.Listening), "listening")
	scores["listening"] = SectionScoreReport{
		RawScore:       rawL,
		TotalQuestions: len(questions.Listening),
		BandScore:      bandL,
	}

	// 2. Grade Reading
	rawR := 0
	for i, q := range questions.Reading {
		ansKey := strconv.Itoa(i)
		if chosen, ok := body.Answers.Reading[ansKey]; ok && chosen == q.CorrectIndex {
			rawR++
		}
	}
	bandR := calculateBand(rawR, len(questions.Reading), "reading")
	scores["reading"] = SectionScoreReport{
		RawScore:       rawR,
		TotalQuestions: len(questions.Reading),
		BandScore:      bandR,
	}

	// 3. Grade Writing (Task 1 & Task 2)
	t1Text := body.Answers.Writing["task1"]
	t2Text := body.Answers.Writing["task2"]
	bandW := 6.0
	var wDetails map[string]any

	if len(strings.Fields(t1Text)) >= 20 || len(strings.Fields(t2Text)) >= 20 {
		fb1 := evaluateWritingResponse("task1", questions.Writing[0].Prompt, t1Text)
		fb2 := evaluateWritingResponse("task2", questions.Writing[1].Prompt, t2Text)
		// IELTS Writing overall = (Task 1 * 1/3) + (Task 2 * 2/3)
		b1 := fb1.Overall
		b2 := fb2.Overall
		calculatedW := math.Round(((b1*1.0 + b2*2.0) / 3.0) * 2.0) / 2.0
		if calculatedW > 0 {
			bandW = calculatedW
		}
		wDetails = map[string]any{
			"task1Feedback": fb1,
			"task2Feedback": fb2,
			"task1Band":     b1,
			"task2Band":     b2,
		}
	}

	scores["writing"] = SectionScoreReport{
		RawScore:       int(bandW * 10),
		TotalQuestions: 2,
		BandScore:      bandW,
		DetailedScores: wDetails,
	}

	// Calculate overall IELTS Band: Average of the 3 sections rounded to nearest 0.5
	rawOverall := (bandL + bandR + bandW) / 3.0
	overallBand := math.Round(rawOverall*2.0) / 2.0

	now := time.Now().UTC()
	answersJSON, _ := json.Marshal(body.Answers)
	scoresJSON, _ := json.Marshal(scores)

	_, err = a.db.ExecContext(r.Context(), `
		UPDATE mock_test_sessions
		SET status = 'completed', completed_at = ?, time_remaining_seconds = 0,
		    answers_json = ?, section_scores_json = ?, overall_band = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, now, answersJSON, scoresJSON, overallBand, now, mockID, uid)

	if err != nil {
		log.Printf("finish mock test err: %v", err)
		writeError(w, 500, "Gagal menyelesaikan simulasi tes.")
		return
	}

	pubView := MockTestPublicView{
		ID:                   mockID,
		Status:               "completed",
		CompletedAt:          &now,
		TimeRemainingSeconds: 0,
		Answers:              body.Answers,
		SectionScores:        scores,
		OverallBand:          &overallBand,
		FullReviewQuestions:  &questions,
	}

	writeJSON(w, 200, map[string]any{"mockTest": pubView})
}

func randInt(n int) int {
	if n <= 1 {
		return 0
	}
	return int(time.Now().UnixNano()) % n
}
