package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestMySQLSessionLifecycle(t *testing.T) {
	if os.Getenv("RUN_MYSQL_INTEGRATION") != "1" {
		t.Skip("set RUN_MYSQL_INTEGRATION=1 to test the configured MySQL server")
	}

	loadDotEnv(".env")
	cfg := loadConfig()
	cfg.DeepSeekKey = ""
	db, err := openDB(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	a := &app{db: db, cfg: cfg, client: &http.Client{Timeout: time.Second}, skipReviewSchedule: true, hits: map[string][]time.Time{}}
	body := []byte(`{"level":"B1","ieltsTarget":6.5,"count":5,"durationMinutes":10,"types":["grammar","reading"]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(body))
	request.RemoteAddr = "127.0.0.1:1234"
	recorder := httptest.NewRecorder()
	a.createSession(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	var created sessionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created.Session.ID
	defer db.Exec("DELETE FROM practice_sessions WHERE id=?", id)
	defer db.Exec("DELETE FROM user_question_history WHERE session_id=?", id)

	generated := make([]question, 5)
	for i := range generated {
		generated[i] = question{
			ID: fmt.Sprintf("new-%d", i+1), Type: "grammar", Prompt: fmt.Sprintf("Generated integration question %s %d", newID(), i+1),
			Choices: []string{"A", "B", "C", "D"}, CorrectIndex: i % 4, Explanation: "Penjelasan integrasi.",
			LearningTip: "Tip integrasi.", IELTSSkill: "Grammatical Range and Accuracy",
		}
		defer db.Exec("DELETE FROM question_bank WHERE content_hash=?", bankQuestionHash("B1", generated[i]))
	}
	deepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		content, _ := json.Marshal(map[string]any{"questions": generated})
		writeJSON(w, http.StatusOK, map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": string(content)}}}})
	}))
	oldCfg, oldClient := a.cfg, a.client
	a.cfg.DeepSeekKey = "integration-key"
	a.cfg.DeepSeekBaseURL = deepSeekServer.URL
	a.cfg.DeepSeekModel = "integration-model"
	a.client = deepSeekServer.Client()
	deepSeekBody := []byte(`{"level":"B1","ieltsTarget":6.5,"count":5,"durationMinutes":10,"types":["grammar"],"questionMode":"deepseek"}`)
	deepSeekRequest := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(deepSeekBody))
	deepSeekRequest.RemoteAddr = "127.0.0.1:1234"
	deepSeekRecorder := httptest.NewRecorder()
	a.createSession(deepSeekRecorder, deepSeekRequest)
	a.cfg, a.client = oldCfg, oldClient
	deepSeekServer.Close()
	if deepSeekRecorder.Code != http.StatusCreated {
		t.Fatalf("deepseek create status=%d body=%s", deepSeekRecorder.Code, deepSeekRecorder.Body.String())
	}
	var deepSeekSession sessionResponse
	if err := json.Unmarshal(deepSeekRecorder.Body.Bytes(), &deepSeekSession); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM practice_sessions WHERE id=?", deepSeekSession.Session.ID)
	defer db.Exec("DELETE FROM user_question_history WHERE session_id=?", deepSeekSession.Session.ID)
	if deepSeekSession.Session.Source != "deepseek" || len(deepSeekSession.Session.PublicQuestions) != 5 {
		t.Fatalf("unexpected generated session: source=%s questions=%d", deepSeekSession.Session.Source, len(deepSeekSession.Session.PublicQuestions))
	}
	var storedGenerated int
	if err := db.QueryRow("SELECT COUNT(*) FROM question_bank WHERE content_hash=?", bankQuestionHash("B1", generated[0])).Scan(&storedGenerated); err != nil || storedGenerated != 1 {
		t.Fatalf("generated question not stored in bank: count=%d err=%v", storedGenerated, err)
	}

	answerRequest := httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"selectedIndex":1}`))
	answerRecorder := httptest.NewRecorder()
	a.saveAnswer(answerRecorder, answerRequest, id, 0)
	if answerRecorder.Code != http.StatusOK {
		t.Fatalf("answer status=%d body=%s", answerRecorder.Code, answerRecorder.Body.String())
	}

	completeRecorder := httptest.NewRecorder()
	a.completeSession(completeRecorder, httptest.NewRequest(http.MethodPost, "/", nil), id)
	if completeRecorder.Code != http.StatusOK {
		t.Fatalf("complete status=%d body=%s", completeRecorder.Code, completeRecorder.Body.String())
	}
	var completed sessionResponse
	if err := json.Unmarshal(completeRecorder.Body.Bytes(), &completed); err != nil {
		t.Fatal(err)
	}
	if completed.Session.Status != "completed" || len(completed.Review) != 5 {
		t.Fatalf("unexpected completed session: status=%s review=%d", completed.Session.Status, len(completed.Review))
	}
	uniqueQuestion := question{Type: "grammar", Prompt: "Integration review " + newID(), Choices: []string{"A", "B", "C", "D"}, CorrectIndex: 1, Explanation: "Test", LearningTip: "Test", IELTSSkill: "Grammar"}
	assignReviewKeys([]question{uniqueQuestion})
	uniqueQuestion.ReviewKey = userQuestionKey(0, uniqueQuestion)
	defer db.Exec("DELETE FROM review_cards WHERE card_key=?", uniqueQuestion.ReviewKey)
	if err := a.updateReviewSchedule(request.Context(), session{Questions: []question{uniqueQuestion}, Answers: map[int]int{0: 0}}, time.Now().UTC()); err != nil {
		t.Fatalf("populate review schedule: %v", err)
	}
	var scheduled int
	if err := db.QueryRow("SELECT COUNT(*) FROM review_cards WHERE card_key=?", uniqueQuestion.ReviewKey).Scan(&scheduled); err != nil || scheduled != 1 {
		t.Fatalf("review schedule not populated: count=%d err=%v", scheduled, err)
	}
	if _, err := db.Exec("UPDATE review_cards SET next_review_at='2000-01-01' WHERE card_key=?", uniqueQuestion.ReviewKey); err != nil {
		t.Fatal(err)
	}
	dueRecorder := httptest.NewRecorder()
	dueRequest := httptest.NewRequest(http.MethodPost, "/api/reviews/session", nil)
	dueRequest.RemoteAddr = "127.0.0.1:1234"
	a.reviewSession(dueRecorder, dueRequest)
	if dueRecorder.Code != http.StatusCreated {
		t.Fatalf("due review status=%d body=%s", dueRecorder.Code, dueRecorder.Body.String())
	}
	var dueSession sessionResponse
	if err := json.Unmarshal(dueRecorder.Body.Bytes(), &dueSession); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM practice_sessions WHERE id=?", dueSession.Session.ID)
	if dueSession.Session.Source != "spaced_review" || len(dueSession.Session.PublicQuestions) == 0 {
		t.Fatalf("unexpected due review session: %#v", dueSession.Session)
	}

	retryRecorder := httptest.NewRecorder()
	retryRequest := httptest.NewRequest(http.MethodPost, "/", nil)
	retryRequest.RemoteAddr = "127.0.0.1:1234"
	a.retrySession(retryRecorder, retryRequest, id)
	if retryRecorder.Code != http.StatusCreated {
		t.Fatalf("retry status=%d body=%s", retryRecorder.Code, retryRecorder.Body.String())
	}
	var retried sessionResponse
	if err := json.Unmarshal(retryRecorder.Body.Bytes(), &retried); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM practice_sessions WHERE id=?", retried.Session.ID)
	if retried.Session.Status != "in_progress" || retried.Session.Source != "review" || len(retried.Session.PublicQuestions) != 4 {
		t.Fatalf("unexpected retry session: status=%s source=%s questions=%d", retried.Session.Status, retried.Session.Source, len(retried.Session.PublicQuestions))
	}

	vocabWord := "integration-" + newID()
	vocabBody, _ := json.Marshal(map[string]string{"word": vocabWord, "meaning": "frasa untuk pengujian", "example": "This phrase is stored during an integration test.", "collocation": "store a phrase"})
	vocabRecorder := httptest.NewRecorder()
	a.vocabulary(vocabRecorder, httptest.NewRequest(http.MethodPost, "/api/vocabulary", bytes.NewReader(vocabBody)))
	if vocabRecorder.Code != http.StatusCreated {
		t.Fatalf("vocabulary status=%d body=%s", vocabRecorder.Code, vocabRecorder.Body.String())
	}
	var vocab vocabularyCard
	if err := json.Unmarshal(vocabRecorder.Body.Bytes(), &vocab); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM vocabulary_cards WHERE id=?", vocab.ID)

	dashboardRecorder := httptest.NewRecorder()
	a.dashboard(dashboardRecorder, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	if dashboardRecorder.Code != http.StatusOK {
		t.Fatalf("dashboard status=%d body=%s", dashboardRecorder.Code, dashboardRecorder.Body.String())
	}
	var dashboard dashboardResponse
	if err := json.Unmarshal(dashboardRecorder.Body.Bytes(), &dashboard); err != nil {
		t.Fatal(err)
	}
	if dashboard.CompletedSessions == 0 || len(dashboard.Weaknesses) == 0 {
		t.Fatalf("dashboard missing learning data: %#v", dashboard)
	}

	reflectionRecorder := httptest.NewRecorder()
	reflectionRequest := httptest.NewRequest(http.MethodPut, "/api/reflections/"+id+"/1", bytes.NewBufferString(`{"reason":"detail"}`))
	a.saveReflection(reflectionRecorder, reflectionRequest)
	if reflectionRecorder.Code != http.StatusOK {
		t.Fatalf("reflection status=%d body=%s", reflectionRecorder.Code, reflectionRecorder.Body.String())
	}

	writingPrompt, _ := writingTaskPrompt("task2")
	writingText := "Schools need to balance practical skills with academic subjects. Practical lessons prepare students for daily responsibilities and employment. Academic subjects, however, develop reasoning and provide the knowledge required for advanced study. In my view, schools should connect both approaches so that students can understand ideas and apply them in realistic situations."
	writingBody, _ := json.Marshal(map[string]string{"taskType": "task2", "prompt": writingPrompt, "response": writingText})
	writingRecorder := httptest.NewRecorder()
	writingRequest := httptest.NewRequest(http.MethodPost, "/api/writing", bytes.NewReader(writingBody))
	writingRequest.RemoteAddr = "127.0.0.1:1234"
	a.submitWriting(writingRecorder, writingRequest)
	if writingRecorder.Code != http.StatusCreated {
		t.Fatalf("writing status=%d body=%s", writingRecorder.Code, writingRecorder.Body.String())
	}
	var submission writingSubmission
	if err := json.Unmarshal(writingRecorder.Body.Bytes(), &submission); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM writing_submissions WHERE id=?", submission.ID)
	if submission.Source != "demo" || submission.WordCount < 50 || submission.Feedback.Summary == "" {
		t.Fatalf("unexpected writing submission: %#v", submission)
	}
}

func TestMySQLSelfRegistrationAndLogin(t *testing.T) {
	if os.Getenv("RUN_MYSQL_INTEGRATION") != "1" {
		t.Skip("set RUN_MYSQL_INTEGRATION=1 to test the configured MySQL server")
	}
	loadDotEnv(".env")
	db, err := openDB(loadConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC()
	guardEmail := "guard-" + newID() + "@example.test"
	guardHash, _ := hashPassword("guardPassword2026")
	guard, err := db.Exec(`INSERT INTO users (name,email,password_hash,role,created_at,updated_at) VALUES (?,?,?,?,?,?)`, "Integration Guard", guardEmail, guardHash, "learner", now, now)
	if err != nil {
		t.Fatal(err)
	}
	guardID, _ := guard.LastInsertId()
	defer db.Exec("DELETE FROM users WHERE id=?", guardID)

	a := &app{db: db, client: &http.Client{Timeout: time.Second}, hits: map[string][]time.Time{}}
	email := "learner-" + newID() + "@example.test"
	body, _ := json.Marshal(map[string]string{"name": "Integration Learner", "email": email, "password": "latihanIELTS2026"})
	registerRequest := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	registerRequest.RemoteAddr = "127.0.0.2:1234"
	registerRecorder := httptest.NewRecorder()
	a.register(registerRecorder, registerRequest)
	if registerRecorder.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", registerRecorder.Code, registerRecorder.Body.String())
	}
	var registered struct {
		User authUser `json:"user"`
	}
	if err := json.Unmarshal(registerRecorder.Body.Bytes(), &registered); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM users WHERE id=?", registered.User.ID)
	if registered.User.Role != "learner" {
		t.Fatalf("role=%s, want learner", registered.User.Role)
	}
	cookies := registerRecorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != authCookieName || !cookies[0].HttpOnly {
		t.Fatalf("unexpected auth cookie: %#v", cookies)
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meRequest.AddCookie(cookies[0])
	meRecorder := httptest.NewRecorder()
	a.requireAuth(http.HandlerFunc(a.me)).ServeHTTP(meRecorder, meRequest)
	if meRecorder.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", meRecorder.Code, meRecorder.Body.String())
	}

	loginBody, _ := json.Marshal(map[string]string{"email": email, "password": "latihanIELTS2026"})
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginRequest.RemoteAddr = "127.0.0.3:1234"
	loginRecorder := httptest.NewRecorder()
	a.login(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
}
