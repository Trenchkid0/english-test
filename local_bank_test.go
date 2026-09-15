package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func mustLoadCuratedTestBatch(name string) []curatedQuestionRecord {
	items, err := loadCuratedQuestionFile(filepath.Join("data", "questions", name))
	if err != nil {
		panic(err)
	}
	return items
}

var (
	curatedBatch1Questions  = mustLoadCuratedTestBatch("batch1_b1_b2_authentic.json")
	curatedBatch2Questions  = mustLoadCuratedTestBatch("curated_batch2.json")
	curatedBatch3Questions  = mustLoadCuratedTestBatch("curated_batch3.json")
	curatedBatch4Questions  = mustLoadCuratedTestBatch("curated_batch4_skills.json")
	curatedBatch5Questions  = mustLoadCuratedTestBatch("curated_batch5_expansion.json")
	curatedBatch6Questions  = mustLoadCuratedTestBatch("curated_batch6_expansion.json")
	curatedBatch7Questions  = mustLoadCuratedTestBatch("curated_batch7_expansion.json")
	curatedBatch8Questions  = mustLoadCuratedTestBatch("curated_batch8_expansion.json")
	curatedBatch9Questions  = mustLoadCuratedTestBatch("curated_batch9_expansion.json")
	curatedBatch10Questions = mustLoadCuratedTestBatch("curated_batch10_expansion.json")
	curatedBatch11Questions = mustLoadCuratedTestBatch("curated_batch11_expansion.json")
	curatedBatch12Questions = mustLoadCuratedTestBatch("curated_batch12_expansion.json")
)

func TestLocalQuestionBankHasFiveHundredValidUniqueQuestionsPerType(t *testing.T) {
	for _, typ := range localTypes {
		items := localQuestions("B1", 6.5, typ)
		if len(items) != 500 {
			t.Fatalf("type %s generated %d questions, want 500", typ, len(items))
		}
		if err := validateQuestions(items, 500, []string{typ}); err != nil {
			t.Fatalf("type %s failed validation: %v", typ, err)
		}
		for _, q := range items {
			answer := q.Choices[q.CorrectIndex]
			if !strings.Contains(q.Explanation, answer) || !strings.Contains(q.Explanation, "Alasannya:") {
				t.Fatalf("type %s question %s lacks a learner-friendly answer and reason: %q", typ, q.ID, q.Explanation)
			}
		}
	}
}

func TestLocalQuestionBankSeparatesLevelAndTargetHashes(t *testing.T) {
	first := localQuestions("A1", 4, "grammar")[0]
	second := localQuestions("C2", 9, "grammar")[0]
	if bankQuestionHashForTarget("A1", 4, first) == bankQuestionHashForTarget("C2", 9, second) {
		t.Fatal("different level and target cells must have different database identities")
	}
}

func TestCuratedBatch1QuestionsAreValidAndUnique(t *testing.T) {
	if len(curatedBatch1Questions) != 100 {
		t.Fatalf("expected 100 curated questions in Batch 1, got %d", len(curatedBatch1Questions))
	}

	questions := make([]question, 0, len(curatedBatch1Questions))
	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch1Questions {
		q := question{
			ID:           cq.Prompt,
			Type:         cq.Type,
			Prompt:       cq.Prompt,
			Choices:      cq.Choices,
			CorrectIndex: cq.CorrectIndex,
			Explanation:  cq.Explanation,
			LearningTip:  cq.LearningTip,
			IELTSSkill:   cq.IELTSSkill,
		}

		cleanPrompt := visiblePromptKey(q.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt detected at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(q.Choices) != 4 {
			t.Errorf("Question %d does not have 4 choices: %d", i, len(q.Choices))
		}
		if q.CorrectIndex < 0 || q.CorrectIndex > 3 {
			t.Errorf("Question %d has invalid CorrectIndex: %d", i, q.CorrectIndex)
		}
		choiceCounts[q.CorrectIndex]++

		if q.Type == "error_identification" {
			bracketCount := 0
			for _, r := range q.Prompt {
				if r == '[' {
					bracketCount++
				}
			}
			if bracketCount != 4 {
				t.Errorf("Error identification question %d has %d brackets (expected 4): %s", i, bracketCount, q.Prompt)
			}
		}

		questions = append(questions, q)
	}

	t.Logf("Curated Batch 1 choice distribution: A=%d, B=%d, C=%d, D=%d",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3])

	// Ensure each option has at least 15 appearances out of 100
	for opt, count := range choiceCounts {
		if count < 15 {
			t.Errorf("Option %d appeared only %d times (less than 15, expected balanced distribution)", opt, count)
		}
	}
}

func TestCuratedBatch2QuestionsAreValidAndUnique(t *testing.T) {
	if len(curatedBatch2Questions) != 100 {
		t.Fatalf("Expected 100 questions in Curated Batch 2, got %d", len(curatedBatch2Questions))
	}

	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch2Questions {
		q := question{
			ID:           cq.Prompt,
			Type:         cq.Type,
			Prompt:       cq.Prompt,
			Choices:      cq.Choices,
			CorrectIndex: cq.CorrectIndex,
			Explanation:  cq.Explanation,
			LearningTip:  cq.LearningTip,
			IELTSSkill:   cq.IELTSSkill,
		}

		cleanPrompt := visiblePromptKey(q.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt in Batch 2 at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(q.Choices) != 4 {
			t.Errorf("Batch 2 Question %d does not have 4 choices: %d", i, len(q.Choices))
		}
		if q.CorrectIndex < 0 || q.CorrectIndex > 3 {
			t.Errorf("Batch 2 Question %d has invalid CorrectIndex: %d", i, q.CorrectIndex)
		}
		choiceCounts[q.CorrectIndex]++

		if q.Type == "error_identification" {
			bracketCount := 0
			for _, r := range q.Prompt {
				if r == '[' {
					bracketCount++
				}
			}
			if bracketCount != 4 {
				t.Errorf("Error identification Batch 2 question %d has %d brackets (expected 4): %s", i, bracketCount, q.Prompt)
			}
		}
	}

	t.Logf("Curated Batch 2 choice distribution: A=%d, B=%d, C=%d, D=%d",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3])

	for opt, count := range choiceCounts {
		if count < 15 {
			t.Errorf("Batch 2 Option %d appeared only %d times (less than 15)", opt, count)
		}
	}
}

func TestCuratedBatch3QuestionsAreValidAndUnique(t *testing.T) {
	if len(curatedBatch3Questions) != 100 {
		t.Fatalf("Expected 100 questions in Curated Batch 3, got %d", len(curatedBatch3Questions))
	}

	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch3Questions {
		q := question{
			ID:           cq.Prompt,
			Type:         cq.Type,
			Prompt:       cq.Prompt,
			Choices:      cq.Choices,
			CorrectIndex: cq.CorrectIndex,
			Explanation:  cq.Explanation,
			LearningTip:  cq.LearningTip,
			IELTSSkill:   cq.IELTSSkill,
		}

		cleanPrompt := visiblePromptKey(q.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt in Batch 3 at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(q.Choices) != 4 {
			t.Errorf("Batch 3 Question %d does not have 4 choices: %d", i, len(q.Choices))
		}
		if q.CorrectIndex < 0 || q.CorrectIndex > 3 {
			t.Errorf("Batch 3 Question %d has invalid CorrectIndex: %d", i, q.CorrectIndex)
		}
		choiceCounts[q.CorrectIndex]++

		if q.Type == "error_identification" {
			bracketCount := 0
			for _, r := range q.Prompt {
				if r == '[' {
					bracketCount++
				}
			}
			if bracketCount != 4 {
				t.Errorf("Error identification Batch 3 question %d has %d brackets (expected 4): %s", i, bracketCount, q.Prompt)
			}
		}
	}

	t.Logf("Curated Batch 3 choice distribution: A=%d, B=%d, C=%d, D=%d",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3])

	for opt, count := range choiceCounts {
		if count < 15 {
			t.Errorf("Batch 3 Option %d appeared only %d times (less than 15)", opt, count)
		}
	}
}

func TestCuratedBatch4SkillsQuestionsAreValid(t *testing.T) {
	if len(curatedBatch4Questions) == 0 {
		t.Fatalf("Expected non-empty questions in Curated Batch 4")
	}

	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch4Questions {
		cleanPrompt := visiblePromptKey(cq.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt in Batch 4 at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(cq.Choices) != 4 {
			t.Errorf("Batch 4 Question %d does not have 4 choices: %d", i, len(cq.Choices))
		}
		if cq.CorrectIndex < 0 || cq.CorrectIndex > 3 {
			t.Errorf("Batch 4 Question %d has invalid CorrectIndex: %d", i, cq.CorrectIndex)
		}
		choiceCounts[cq.CorrectIndex]++

		if cq.Type == "reading" {
			if len(cq.Context) < 40 {
				t.Errorf("Reading question %d context too short: %q", i, cq.Context)
			}
			if cq.Evidence == "" {
				t.Errorf("Reading question %d missing evidence", i)
			}
		}

		if cq.Type == "listening" {
			if len([]rune(strings.TrimSpace(cq.Context))) < 180 {
				t.Errorf("Listening question %d context shorter than 180 chars: %d", i, len([]rune(cq.Context)))
			}
			if strings.Count(cq.Context, ":") < 4 {
				t.Errorf("Listening question %d has fewer than 4 speaker turns: %d", i, strings.Count(cq.Context, ":"))
			}
			if strings.Count(cq.Context, "\n") < 4 {
				t.Errorf("Listening question %d has fewer than 4 line breaks: %d", i, strings.Count(cq.Context, "\n"))
			}
		}
	}

	t.Logf("Curated Batch 4 choice distribution: A=%d, B=%d, C=%d, D=%d across %d questions",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3], len(curatedBatch4Questions))
}

func TestCuratedBatch5ExpansionQuestionsAreValid(t *testing.T) {
	if len(curatedBatch5Questions) != 450 {
		t.Fatalf("Expected 450 questions in Curated Batch 5, got %d", len(curatedBatch5Questions))
	}

	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch5Questions {
		cleanPrompt := visiblePromptKey(cq.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt in Batch 5 at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(cq.Choices) != 4 {
			t.Errorf("Batch 5 Question %d does not have 4 choices: %d", i, len(cq.Choices))
		}
		if cq.CorrectIndex < 0 || cq.CorrectIndex > 3 {
			t.Errorf("Batch 5 Question %d has invalid CorrectIndex: %d", i, cq.CorrectIndex)
		}
		choiceCounts[cq.CorrectIndex]++

		if cq.Type == "reading" {
			if len(cq.Context) < 40 {
				t.Errorf("Reading question %d context too short: %q", i, cq.Context)
			}
			if cq.Evidence == "" {
				t.Errorf("Reading question %d missing evidence", i)
			}
		}

		if cq.Type == "listening" {
			if len([]rune(strings.TrimSpace(cq.Context))) < 180 {
				t.Errorf("Listening question %d context shorter than 180 chars: %d", i, len([]rune(cq.Context)))
			}
			if strings.Count(cq.Context, ":") < 4 {
				t.Errorf("Listening question %d has fewer than 4 speaker turns: %d", i, strings.Count(cq.Context, ":"))
			}
			if strings.Count(cq.Context, "\n") < 4 {
				t.Errorf("Listening question %d has fewer than 4 line breaks: %d", i, strings.Count(cq.Context, "\n"))
			}
		}

		if cq.Type == "error_identification" {
			bracketCount := strings.Count(cq.Prompt, "[")
			if bracketCount != 4 {
				t.Errorf("Error identification Batch 5 question %d has %d brackets (expected 4): %s", i, bracketCount, cq.Prompt)
			}
		}
	}

	t.Logf("Curated Batch 5 choice distribution: A=%d, B=%d, C=%d, D=%d across %d questions",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3], len(curatedBatch5Questions))

	for opt, count := range choiceCounts {
		if count < 50 {
			t.Errorf("Batch 5 Option %d appeared only %d times (less than 50)", opt, count)
		}
	}
}

func TestCuratedBatch6ExpansionQuestionsAreValid(t *testing.T) {
	if len(curatedBatch6Questions) != 606 {
		t.Fatalf("Expected 606 questions in Curated Batch 6, got %d", len(curatedBatch6Questions))
	}

	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch6Questions {
		cleanPrompt := visiblePromptKey(cq.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt in Batch 6 at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(cq.Choices) != 4 {
			t.Errorf("Batch 6 Question %d does not have 4 choices: %d", i, len(cq.Choices))
		}
		if cq.CorrectIndex < 0 || cq.CorrectIndex > 3 {
			t.Errorf("Batch 6 Question %d has invalid CorrectIndex: %d", i, cq.CorrectIndex)
		}
		choiceCounts[cq.CorrectIndex]++

		if cq.Type == "reading" {
			if len(cq.Context) < 40 {
				t.Errorf("Reading question %d context too short: %q", i, cq.Context)
			}
			if cq.Evidence == "" {
				t.Errorf("Reading question %d missing evidence", i)
			}
		}

		if cq.Type == "listening" {
			if len([]rune(strings.TrimSpace(cq.Context))) < 180 {
				t.Errorf("Listening question %d context shorter than 180 chars: %d", i, len([]rune(cq.Context)))
			}
			if strings.Count(cq.Context, ":") < 4 {
				t.Errorf("Listening question %d has fewer than 4 speaker turns: %d", i, strings.Count(cq.Context, ":"))
			}
			if strings.Count(cq.Context, "\n") < 4 {
				t.Errorf("Listening question %d has fewer than 4 line breaks: %d", i, strings.Count(cq.Context, "\n"))
			}
		}

		if cq.Type == "error_identification" {
			bracketCount := strings.Count(cq.Prompt, "[")
			if bracketCount != 4 {
				t.Errorf("Error identification Batch 6 question %d has %d brackets (expected 4): %s", i, bracketCount, cq.Prompt)
			}
		}
	}

	t.Logf("Curated Batch 6 choice distribution: A=%d, B=%d, C=%d, D=%d across %d questions",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3], len(curatedBatch6Questions))

	for opt, count := range choiceCounts {
		if count < 50 {
			t.Errorf("Batch 6 Option %d appeared only %d times (less than 50)", opt, count)
		}
	}
}

func TestCuratedBatch7ExpansionQuestionsAreValid(t *testing.T) {
	if len(curatedBatch7Questions) != 960 {
		t.Fatalf("Expected 960 questions in Curated Batch 7, got %d", len(curatedBatch7Questions))
	}

	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch7Questions {
		cleanPrompt := visiblePromptKey(cq.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt in Batch 7 at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(cq.Choices) != 4 {
			t.Errorf("Batch 7 Question %d does not have 4 choices: %d", i, len(cq.Choices))
		}
		if cq.CorrectIndex < 0 || cq.CorrectIndex > 3 {
			t.Errorf("Batch 7 Question %d has invalid CorrectIndex: %d", i, cq.CorrectIndex)
		}
		choiceCounts[cq.CorrectIndex]++

		if cq.Type == "reading" {
			if len(cq.Context) < 40 {
				t.Errorf("Reading question %d context too short: %q", i, cq.Context)
			}
			if cq.Evidence == "" {
				t.Errorf("Reading question %d missing evidence", i)
			}
		}

		if cq.Type == "listening" {
			if len([]rune(strings.TrimSpace(cq.Context))) < 180 {
				t.Errorf("Listening question %d context shorter than 180 chars: %d", i, len([]rune(cq.Context)))
			}
			if strings.Count(cq.Context, ":") < 4 {
				t.Errorf("Listening question %d has fewer than 4 speaker turns: %d", i, strings.Count(cq.Context, ":"))
			}
			if strings.Count(cq.Context, "\n") < 4 {
				t.Errorf("Listening question %d has fewer than 4 line breaks: %d", i, strings.Count(cq.Context, "\n"))
			}
		}

		if cq.Type == "error_identification" {
			bracketCount := strings.Count(cq.Prompt, "[")
			if bracketCount != 4 {
				t.Errorf("Error identification Batch 7 question %d has %d brackets (expected 4): %s", i, bracketCount, cq.Prompt)
			}
		}
	}

	t.Logf("Curated Batch 7 choice distribution: A=%d, B=%d, C=%d, D=%d across %d questions",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3], len(curatedBatch7Questions))

	for opt, count := range choiceCounts {
		if count < 100 {
			t.Errorf("Batch 7 Option %d appeared only %d times (less than 100)", opt, count)
		}
	}
}

func TestCuratedBatch8ExpansionQuestionsAreValid(t *testing.T) {
	if len(curatedBatch8Questions) != 720 {
		t.Fatalf("Expected 720 questions in Curated Batch 8, got %d", len(curatedBatch8Questions))
	}

	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch8Questions {
		cleanPrompt := visiblePromptKey(cq.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt in Batch 8 at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(cq.Choices) != 4 {
			t.Errorf("Batch 8 Question %d does not have 4 choices: %d", i, len(cq.Choices))
		}
		if cq.CorrectIndex < 0 || cq.CorrectIndex > 3 {
			t.Errorf("Batch 8 Question %d has invalid CorrectIndex: %d", i, cq.CorrectIndex)
		}
		choiceCounts[cq.CorrectIndex]++

		if cq.Type == "reading" {
			if len(cq.Context) < 40 {
				t.Errorf("Reading question %d context too short: %q", i, cq.Context)
			}
			if cq.Evidence == "" {
				t.Errorf("Reading question %d missing evidence", i)
			}
		}

		if cq.Type == "listening" {
			if len([]rune(strings.TrimSpace(cq.Context))) < 180 {
				t.Errorf("Listening question %d context shorter than 180 chars: %d", i, len([]rune(cq.Context)))
			}
			if strings.Count(cq.Context, ":") < 4 {
				t.Errorf("Listening question %d has fewer than 4 speaker turns: %d", i, strings.Count(cq.Context, ":"))
			}
			if strings.Count(cq.Context, "\n") < 4 {
				t.Errorf("Listening question %d has fewer than 4 line breaks: %d", i, strings.Count(cq.Context, "\n"))
			}
		}

		if cq.Type == "error_identification" {
			bracketCount := strings.Count(cq.Prompt, "[")
			if bracketCount != 4 {
				t.Errorf("Error identification Batch 8 question %d has %d brackets (expected 4): %s", i, bracketCount, cq.Prompt)
			}
		}
	}

	t.Logf("Curated Batch 8 choice distribution: A=%d, B=%d, C=%d, D=%d across %d questions",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3], len(curatedBatch8Questions))

	for opt, count := range choiceCounts {
		if count < 50 {
			t.Errorf("Batch 8 Option %d appeared only %d times (less than 50)", opt, count)
		}
	}
}

func TestCuratedBatch9ExpansionQuestionsAreValid(t *testing.T) {
	if len(curatedBatch9Questions) != 720 {
		t.Fatalf("Expected 720 questions in Curated Batch 9, got %d", len(curatedBatch9Questions))
	}

	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch9Questions {
		cleanPrompt := visiblePromptKey(cq.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt in Batch 9 at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(cq.Choices) != 4 {
			t.Errorf("Batch 9 Question %d does not have 4 choices: %d", i, len(cq.Choices))
		}
		if cq.CorrectIndex < 0 || cq.CorrectIndex > 3 {
			t.Errorf("Batch 9 Question %d has invalid CorrectIndex: %d", i, cq.CorrectIndex)
		}
		choiceCounts[cq.CorrectIndex]++

		if cq.Type == "reading" {
			if len(cq.Context) < 40 {
				t.Errorf("Reading question %d context too short: %q", i, cq.Context)
			}
			if cq.Evidence == "" {
				t.Errorf("Reading question %d missing evidence", i)
			}
		}

		if cq.Type == "listening" {
			if len([]rune(strings.TrimSpace(cq.Context))) < 180 {
				t.Errorf("Listening question %d context shorter than 180 chars: %d", i, len([]rune(cq.Context)))
			}
			if strings.Count(cq.Context, ":") < 4 {
				t.Errorf("Listening question %d has fewer than 4 speaker turns: %d", i, strings.Count(cq.Context, ":"))
			}
			if strings.Count(cq.Context, "\n") < 4 {
				t.Errorf("Listening question %d has fewer than 4 line breaks: %d", i, strings.Count(cq.Context, "\n"))
			}
		}
	}

	t.Logf("Curated Batch 9 choice distribution: A=%d, B=%d, C=%d, D=%d across %d questions",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3], len(curatedBatch9Questions))

	for opt, count := range choiceCounts {
		if count < 50 {
			t.Errorf("Batch 9 Option %d appeared only %d times (less than 50)", opt, count)
		}
	}
}

func TestCuratedBatch10ExpansionQuestionsAreValid(t *testing.T) {
	if len(curatedBatch10Questions) != 474 {
		t.Fatalf("Expected 474 questions in Curated Batch 10, got %d", len(curatedBatch10Questions))
	}

	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch10Questions {
		cleanPrompt := visiblePromptKey(cq.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt in Batch 10 at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(cq.Choices) != 4 {
			t.Errorf("Batch 10 Question %d does not have 4 choices: %d", i, len(cq.Choices))
		}
		if cq.CorrectIndex < 0 || cq.CorrectIndex > 3 {
			t.Errorf("Batch 10 Question %d has invalid CorrectIndex: %d", i, cq.CorrectIndex)
		}
		choiceCounts[cq.CorrectIndex]++

		if cq.Type == "error_identification" {
			bracketCount := strings.Count(cq.Prompt, "[")
			if bracketCount != 4 {
				t.Errorf("Error identification Batch 10 question %d has %d brackets (expected 4): %s", i, bracketCount, cq.Prompt)
			}
		}
	}

	t.Logf("Curated Batch 10 choice distribution: A=%d, B=%d, C=%d, D=%d across %d questions",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3], len(curatedBatch10Questions))

	for opt, count := range choiceCounts {
		if count < 30 {
			t.Errorf("Batch 10 Option %d appeared only %d times (less than 30)", opt, count)
		}
	}
}

func TestCuratedBatch11ExpansionQuestionsAreValid(t *testing.T) {
	if len(curatedBatch11Questions) != 520 {
		t.Fatalf("Expected 520 questions in Curated Batch 11, got %d", len(curatedBatch11Questions))
	}

	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch11Questions {
		cleanPrompt := visiblePromptKey(cq.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt in Batch 11 at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(cq.Choices) != 4 {
			t.Errorf("Batch 11 Question %d does not have 4 choices: %d", i, len(cq.Choices))
		}
		if cq.CorrectIndex < 0 || cq.CorrectIndex > 3 {
			t.Errorf("Batch 11 Question %d has invalid CorrectIndex: %d", i, cq.CorrectIndex)
		}
		choiceCounts[cq.CorrectIndex]++

		if cq.Type == "reading" {
			if len(cq.Context) < 40 {
				t.Errorf("Reading question %d context too short: %q", i, cq.Context)
			}
			if cq.Evidence == "" {
				t.Errorf("Reading question %d missing evidence", i)
			}
		}

		if cq.Type == "listening" {
			if len([]rune(strings.TrimSpace(cq.Context))) < 180 {
				t.Errorf("Listening question %d context shorter than 180 chars: %d", i, len([]rune(cq.Context)))
			}
			if strings.Count(cq.Context, ":") < 4 {
				t.Errorf("Listening question %d has fewer than 4 speaker turns: %d", i, strings.Count(cq.Context, ":"))
			}
			if strings.Count(cq.Context, "\n") < 4 {
				t.Errorf("Listening question %d has fewer than 4 line breaks: %d", i, strings.Count(cq.Context, "\n"))
			}
		}
	}

	t.Logf("Curated Batch 11 choice distribution: A=%d, B=%d, C=%d, D=%d across %d questions",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3], len(curatedBatch11Questions))

	for opt, count := range choiceCounts {
		if count < 40 {
			t.Errorf("Batch 11 Option %d appeared only %d times (less than 40)", opt, count)
		}
	}
}

func TestCuratedBatch12ExpansionQuestionsAreValid(t *testing.T) {
	if len(curatedBatch12Questions) != 580 {
		t.Fatalf("Expected 580 questions in Curated Batch 12, got %d", len(curatedBatch12Questions))
	}

	seenPrompts := make(map[string]bool)
	choiceCounts := [4]int{}

	for i, cq := range curatedBatch12Questions {
		cleanPrompt := visiblePromptKey(cq.Prompt)
		if seenPrompts[cleanPrompt] {
			t.Errorf("Duplicate prompt in Batch 12 at index %d: %s", i, cleanPrompt)
		}
		seenPrompts[cleanPrompt] = true

		if len(cq.Choices) != 4 {
			t.Errorf("Batch 12 Question %d does not have 4 choices: %d", i, len(cq.Choices))
		}
		if cq.CorrectIndex < 0 || cq.CorrectIndex > 3 {
			t.Errorf("Batch 12 Question %d has invalid CorrectIndex: %d", i, cq.CorrectIndex)
		}
		choiceCounts[cq.CorrectIndex]++

		if cq.Explanation == "" {
			t.Errorf("Batch 12 Question %d missing explanation", i)
		}

		if cq.Type == "reading" {
			if len(cq.Context) < 40 {
				t.Errorf("Reading question %d context too short: %q", i, cq.Context)
			}
			if cq.Evidence == "" {
				t.Errorf("Reading question %d missing evidence", i)
			}
		}

		if cq.Type == "listening" {
			if len([]rune(strings.TrimSpace(cq.Context))) < 180 {
				t.Errorf("Listening question %d context shorter than 180 chars: %d", i, len([]rune(cq.Context)))
			}
			if strings.Count(cq.Context, ":") < 4 {
				t.Errorf("Listening question %d has fewer than 4 speaker turns: %d", i, strings.Count(cq.Context, ":"))
			}
			if strings.Count(cq.Context, "\n") < 3 {
				t.Errorf("Listening question %d has fewer than 3 line breaks: %d", i, strings.Count(cq.Context, "\n"))
			}
		}
	}

	t.Logf("Curated Batch 12 choice distribution: A=%d, B=%d, C=%d, D=%d across %d questions",
		choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3], len(curatedBatch12Questions))

	for opt, count := range choiceCounts {
		if count < 50 {
			t.Errorf("Batch 12 Option %d appeared only %d times (less than 50)", opt, count)
		}
	}
}
