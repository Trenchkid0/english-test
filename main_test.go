package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateInput(t *testing.T) {
	valid := generateInput{Level: "B2", IELTSTarget: 7, Count: 10, DurationMinutes: 15, Types: []string{"grammar", "reading"}}
	if got := validateInput(valid); got != "" {
		t.Fatalf("valid input rejected: %s", got)
	}
	valid.Count = 501
	if got := validateInput(valid); got == "" {
		t.Fatal("invalid count accepted")
	}
	valid.Count = 10
	valid.QuestionMode = "deepseek"
	if got := validateInput(valid); got != "" {
		t.Fatalf("deepseek mode rejected: %s", got)
	}
	valid.QuestionMode = "unknown"
	if got := validateInput(valid); got == "" {
		t.Fatal("unknown question mode accepted")
	}
	valid.QuestionMode = ""
	valid.Types = []string{"grammar", "grammar"}
	if got := validateInput(valid); got == "" {
		t.Fatal("duplicate question type accepted")
	}
}

func TestCreateSessionDeepSeekModeRequiresAPIKey(t *testing.T) {
	a := &app{hits: map[string][]time.Time{}}
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"level":"B1","ieltsTarget":6.5,"count":5,"durationMinutes":10,"types":["grammar"],"questionMode":"deepseek"}`))
	req.RemoteAddr = "127.0.0.1:1234"
	recorder := httptest.NewRecorder()
	a.createSession(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDemoQuestionsMatchRequest(t *testing.T) {
	in := generateInput{Count: 17, Types: []string{"reading", "fill_blank"}}
	items := demoQuestions(in)
	if len(items) != 17 {
		t.Fatalf("got %d questions", len(items))
	}
	for _, q := range items {
		if q.Type != "reading" && q.Type != "fill_blank" {
			t.Fatalf("unexpected type %q", q.Type)
		}
		if len(q.Choices) != 4 {
			t.Fatal("question must have four choices")
		}
	}
}

func TestDemoQuestionsBalanceRequestedTypes(t *testing.T) {
	types := []string{"grammar", "vocabulary", "reading", "fill_blank"}
	items := demoQuestions(generateInput{Count: 5, Types: types})
	counts := make(map[string]int, len(types))
	for _, q := range items {
		counts[q.Type]++
	}
	for _, typ := range types {
		if counts[typ] == 0 {
			t.Fatalf("type %q missing from short mixed session", typ)
		}
	}
	for _, count := range counts {
		if count < 1 || count > 2 {
			t.Fatalf("unbalanced type counts: %#v", counts)
		}
	}
}

func TestDecodeJSONRejectsTrailingValue(t *testing.T) {
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"level":"B1"} {"level":"B2"}`))
	recorder := httptest.NewRecorder()
	var input generateInput
	if err := decodeJSON(recorder, req, &input); err == nil {
		t.Fatal("trailing JSON value accepted")
	}
	if recorder.Code != 400 {
		t.Fatalf("status=%d, want 400", recorder.Code)
	}
}

func TestPracticeFormScopesQuestionTypeSelection(t *testing.T) {
	source, err := webFiles.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(source, []byte(`$$('input[name="types"]:checked', form)`)) {
		t.Fatal("practice form type selector must be scoped to the submitted form")
	}
}

func TestAdminQuestionCatalogueAssetsAreEmbedded(t *testing.T) {
	page, err := webFiles.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range [][]byte{[]byte(`id="filter-form"`), []byte(`id="question-grid"`), []byte(`id="detail-dialog"`)} {
		if !bytes.Contains(page, marker) {
			t.Fatalf("admin page missing %q", marker)
		}
	}
	index, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(index, []byte(`id="admin-page-link"`)) {
		t.Fatal("learner shell is missing the admin-only catalogue link")
	}
}

func TestListeningAudioStopsWhenQuestionOrScreenChanges(t *testing.T) {
	source, err := webFiles.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	checks := [][]byte{
		[]byte(`function stopQuizAudio()`),
		[]byte(`if (name !== "quiz") stopQuizAudio()`),
		[]byte("function renderQuestion() {\n  stopQuizAudio();"),
		[]byte(`window.addEventListener("pagehide", stopQuizAudio)`),
	}
	for _, check := range checks {
		if !bytes.Contains(source, check) {
			t.Fatalf("missing listening audio lifecycle guard %q", check)
		}
	}
}

func TestShuffleQuestionsPreservesEveryQuestion(t *testing.T) {
	items := make([]question, 500)
	for i := range items {
		items[i].ID = fmt.Sprintf("q-%03d", i)
	}
	shuffleQuestions(items)

	seen := make(map[string]bool, len(items))
	changed := false
	for i, item := range items {
		if seen[item.ID] {
			t.Fatalf("question %q appears more than once", item.ID)
		}
		seen[item.ID] = true
		if item.ID != fmt.Sprintf("q-%03d", i) {
			changed = true
		}
	}
	if len(seen) != 500 {
		t.Fatalf("shuffle preserved %d questions, want 500", len(seen))
	}
	if !changed {
		t.Fatal("shuffle left all 500 questions in their original order")
	}
}

func TestRetryQuestionsIncludesWrongAndUnanswered(t *testing.T) {
	s := session{
		Questions: []question{
			{ID: "q1", Type: "grammar", CorrectIndex: 1},
			{ID: "q2", Type: "reading", CorrectIndex: 2},
			{ID: "q3", Type: "grammar", CorrectIndex: 0},
		},
		Answers: map[int]int{0: 1, 1: 0},
	}
	items := retryQuestions(s)
	if len(items) != 2 {
		t.Fatalf("got %d retry questions, want 2", len(items))
	}
	if items[0].ID != "retry-1" || items[1].ID != "retry-2" {
		t.Fatalf("retry IDs not normalized: %q, %q", items[0].ID, items[1].ID)
	}
	types := questionTypes(items)
	if len(types) != 2 || types[0] != "reading" || types[1] != "grammar" {
		t.Fatalf("unexpected retry types: %#v", types)
	}
}
