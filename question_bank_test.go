package main

import (
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestBankQuestionHashSeparatesLevels(t *testing.T) {
	q := question{Type: "grammar", Prompt: "Choose one.", Choices: []string{"A", "B", "C", "D"}}
	if bankQuestionHash("A1", q) == bankQuestionHash("B2", q) {
		t.Fatal("same question at different levels must have different bank hashes")
	}
}

func TestBankQuestionHashIgnoresMutableMetadata(t *testing.T) {
	q1 := question{ID: "one", ReviewKey: "old", Type: "grammar", Prompt: "Choose one.", Choices: []string{"A", "B", "C", "D"}}
	q2 := q1
	q2.ID = "two"
	q2.ReviewKey = "new"
	if bankQuestionHash("B1", q1) != bankQuestionHash("B1", q2) {
		t.Fatal("bank hash should remain stable when session metadata changes")
	}
}

func TestBankHistoryKeyIgnoresInternalPracticeMarker(t *testing.T) {
	first := question{Type: "grammar", Prompt: "Choose the correct verb. [Level B1 · Band 6.5 · Practice 01]", Choices: []string{"A", "B", "C", "D"}}
	second := first
	second.Prompt = "Choose the correct verb. [Level B1 · Band 6.5 · Practice 499]"
	if bankHistoryKey("B1", 6.5, first) != bankHistoryKey("B1", 6.5, second) {
		t.Fatal("internal practice markers must not make a previously seen question look new")
	}
	second.Context = "A genuinely different context."
	if bankHistoryKey("B1", 6.5, first) == bankHistoryKey("B1", 6.5, second) {
		t.Fatal("questions with different visible content must keep different history keys")
	}
}

func TestBankHistoryIsScopedByLevelAndTarget(t *testing.T) {
	q := question{Type: "grammar", Prompt: "Choose the correct verb.", Choices: []string{"A", "B", "C", "D"}}
	if bankHistoryKey("B1", 6.5, q) == bankHistoryKey("B1", 9, q) {
		t.Fatal("the same content at different IELTS targets must have separate histories")
	}
	if bankHistoryKey("B1", 9, q) == bankHistoryKey("C1", 9, q) {
		t.Fatal("the same content at different levels must have separate histories")
	}
}

func TestParseAdminQuestionFiltersUsesSafePaginationDefaults(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/admin/questions?page=-4&pageSize=1000&status=unknown&query=%20modal%20verbs%20", nil)
	filters := parseAdminQuestionFilters(req)
	if filters.Page != 1 || filters.PageSize != 24 || filters.Status != "all" || filters.SortOrder != "desc" {
		t.Fatalf("unexpected defaults: %#v", filters)
	}
	if filters.Search != "modal verbs" {
		t.Fatalf("search=%q, want trimmed search", filters.Search)
	}

	reqAsc := httptest.NewRequest("GET", "/api/admin/questions?sort=asc", nil)
	filtersAsc := parseAdminQuestionFilters(reqAsc)
	if filtersAsc.SortOrder != "asc" {
		t.Fatalf("sort=%q, want 'asc'", filtersAsc.SortOrder)
	}
}

func TestBuildAdminQuestionsWhereKeepsValuesParameterized(t *testing.T) {
	filters := adminQuestionFilters{
		Search: "present%", Level: "B2", Target: "7.5", Type: "grammar",
		Source: "codex_local", Status: "active",
	}
	where, args := buildAdminQuestionsWhere(filters)
	wantWhere := "1=1 AND (CAST(id AS CHAR)=? OR question_json LIKE ?) AND level=? AND ielts_target=? AND type=? AND source=? AND active=TRUE"
	if where != wantWhere {
		t.Fatalf("where=%q, want %q", where, wantWhere)
	}
	wantArgs := []any{"present%", "%present%%", "B2", 7.5, "grammar", "codex_local"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args=%#v, want %#v", args, wantArgs)
	}
}

func TestBuildAdminQuestionsWhereIgnoresInvalidTarget(t *testing.T) {
	where, args := buildAdminQuestionsWhere(adminQuestionFilters{Target: "7.3"})
	if where != "1=1" || len(args) != 0 {
		t.Fatalf("invalid target changed query: where=%q args=%#v", where, args)
	}
}

func TestTestQuestionDedupeKey(t *testing.T) {
	q1 := question{Prompt: "Choose the correct word. [Level B1 · Band 6.5 · Practice 01]"}
	q2 := question{Prompt: "Choose the correct word. [Level B1 · Band 6.5 · Practice 99]"}
	if testQuestionDedupeKey(q1) != testQuestionDedupeKey(q2) {
		t.Fatal("dedupe key should normalize practice markers to prevent duplicate questions in a test")
	}

	q3 := question{Prompt: "What is the author's opinion?", Context: "Passage A"}
	q4 := question{Prompt: "What is the author's opinion?", Context: "Passage B"}
	if testQuestionDedupeKey(q3) == testQuestionDedupeKey(q4) {
		t.Fatal("questions with different contexts must have different dedupe keys")
	}
}

func TestUserQuestionCooldownConstant(t *testing.T) {
	if userQuestionCooldownSessions != 5 {
		t.Fatalf("expected 5 sessions cooldown, got %d", userQuestionCooldownSessions)
	}
}


