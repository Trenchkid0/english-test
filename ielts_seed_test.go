package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestIELTSTargetedMatrixProducesFiveHundredUniqueQuestionsPerCell(t *testing.T) {
	expectedCells := 6 * 9 * 6
	if cells := len(ieltsTargetedLevels) * len(ieltsTargetedTargets) * len(ieltsTargetedTypes); cells != expectedCells {
		t.Fatalf("IELTS matrix has %d cells; expected %d", cells, expectedCells)
	}

	for _, level := range ieltsTargetedLevels {
		for _, target := range ieltsTargetedTargets {
			for _, typ := range ieltsTargetedTypes {
				name := fmt.Sprintf("%s_%.1f_%s", level, target, typ)
				t.Run(name, func(t *testing.T) {
					questions := localQuestionsRange(level, target, typ, 0, ieltsTargetedQuota)
					if err := validateQuestions(questions, ieltsTargetedQuota, []string{typ}); err != nil {
						t.Fatal(err)
					}
					seenContexts := make(map[string]string, ieltsTargetedQuota)
					for _, q := range questions {
						if !strings.HasPrefix(q.Explanation, "Jawaban yang tepat adalah") || !strings.Contains(q.Explanation, "Alasannya:") {
							t.Fatalf("question %s does not explain its correct answer: %q", q.ID, q.Explanation)
						}
						if typ != "reading" && typ != "listening" {
							continue
						}
						pattern := normalizeQuestionContext(q.Context)
						if pattern == "" {
							t.Fatalf("question %s has an empty normalized context", q.ID)
						}
						if previous, exists := seenContexts[pattern]; exists {
							t.Fatalf("questions %s and %s duplicate normalized context", previous, q.ID)
						}
						seenContexts[pattern] = q.ID
					}
				})
			}
		}
	}
}

func TestIELTSTargetedBandsUseDifferentVisibleQuestions(t *testing.T) {
	for _, level := range ieltsTargetedLevels {
		for _, typ := range ieltsTargetedTypes {
			seen := make(map[string]float64, len(ieltsTargetedTargets))
			for _, target := range ieltsTargetedTargets {
				question := localQuestionsRange(level, target, typ, 0, 1)[0]
				key := strings.ToLower(strings.Join(strings.Fields(visiblePromptKey(question.Prompt)), " "))
				if previous, exists := seen[key]; exists {
					t.Fatalf("%s/%s bands %.1f and %.1f produced the same visible question", level, typ, previous, target)
				}
				seen[key] = target
			}
		}
	}
}
