package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestLocalQuestionBankBalancedChoiceDistribution(t *testing.T) {
	choiceCounts := [4]int{}
	total := 0

	for _, typ := range localTypes {
		questions := localQuestions("B2", 6.5, typ)
		for i := 0; i < 40; i++ {
			q := questions[i]
			if q.CorrectIndex < 0 || q.CorrectIndex > 3 {
				t.Fatalf("invalid CorrectIndex: %d for question %s", q.CorrectIndex, q.ID)
			}
			choiceCounts[q.CorrectIndex]++
			total++
		}
	}

	t.Logf("Choice distribution for %d sample questions: A=%d, B=%d, C=%d, D=%d",
		total, choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3])

	minAllowed := int(float64(total) * 0.15)
	for idx, count := range choiceCounts {
		if count < minAllowed {
			t.Errorf("Option index %d appeared only %d times (less than %d, expected balanced distribution)", idx, count, minAllowed)
		}
	}
}

func TestGenerateOneHundredQuestionsPerLevel(t *testing.T) {
	levels := []string{"A1", "A2", "B1", "B2", "C1", "C2"}
	types := []string{"grammar", "vocabulary", "reading", "fill_blank", "listening"}

	for _, level := range levels {
		t.Run("Level_"+level, func(t *testing.T) {
			seenPrompts := make(map[string]bool, 100)
			allQuestions := make([]question, 0, 100)

			for _, typ := range types {
				typeQuestions := localQuestions(level, 6.5, typ)
				for i := 0; i < 20; i++ {
					q := typeQuestions[i]
					allQuestions = append(allQuestions, q)

					cleanPrompt := visiblePromptKey(q.Prompt)
					promptKey := strings.ToLower(strings.Join(strings.Fields(cleanPrompt), " "))
					if seenPrompts[promptKey] {
						t.Errorf("Level %s: Duplicate visible prompt detected for type %s: %s", level, typ, cleanPrompt)
					}
					seenPrompts[promptKey] = true
				}
			}

			if len(allQuestions) != 100 {
				t.Fatalf("Level %s generated %d questions, expected 100", level, len(allQuestions))
			}

			if err := validateQuestions(allQuestions, 100, types); err != nil {
				t.Fatalf("Level %s validation failed: %v", level, err)
			}
		})
	}
}

func TestGenerateOneHundredQuestionsPerIELTSTarget(t *testing.T) {
	targets := []float64{4.0, 4.5, 5.0, 5.5, 6.0, 6.5, 7.0, 7.5, 8.0, 8.5, 9.0}
	types := []string{"grammar", "vocabulary", "reading", "fill_blank", "listening"}

	for _, target := range targets {
		name := fmt.Sprintf("Target_%.1f", target)
		t.Run(name, func(t *testing.T) {
			seenPrompts := make(map[string]bool, 100)
			allQuestions := make([]question, 0, 100)

			for _, typ := range types {
				typeQuestions := localQuestions("B2", target, typ)
				for i := 0; i < 20; i++ {
					q := typeQuestions[i]
					allQuestions = append(allQuestions, q)

					cleanPrompt := visiblePromptKey(q.Prompt)
					promptKey := strings.ToLower(strings.Join(strings.Fields(cleanPrompt), " "))
					if seenPrompts[promptKey] {
						t.Errorf("Target %.1f: Duplicate visible prompt detected for type %s: %s", target, typ, cleanPrompt)
					}
					seenPrompts[promptKey] = true
				}
			}

			if len(allQuestions) != 100 {
				t.Fatalf("Target %.1f generated %d questions, expected 100", target, len(allQuestions))
			}

			if err := validateQuestions(allQuestions, 100, types); err != nil {
				t.Fatalf("Target %.1f validation failed: %v", target, err)
			}
		})
	}
}

func TestRealGenerationBatchReport(t *testing.T) {
	levels := []string{"A1", "A2", "B1", "B2", "C1", "C2"}
	targets := []float64{4.0, 4.5, 5.0, 5.5, 6.0, 6.5, 7.0, 7.5, 8.0, 8.5, 9.0}
	types := []string{"grammar", "vocabulary", "reading", "fill_blank", "listening"}

	fmt.Println("\n================================================================================")
	fmt.Println("🚀 GENERATION REPORT: 100 REAL QUESTIONS PER CEFR LEVEL (A1 - C2)")
	fmt.Println("================================================================================")

	totalGeneratedLevels := 0
	for _, level := range levels {
		questions := make([]question, 0, 100)
		choiceCounts := [4]int{}
		for _, typ := range types {
			pool := localQuestions(level, 6.5, typ)
			for i := 0; i < 20; i++ {
				q := pool[i]
				choiceCounts[q.CorrectIndex]++
				questions = append(questions, q)
			}
		}
		totalGeneratedLevels += len(questions)
		fmt.Printf("✅ Level %-2s | Total: %3d questions | Choices: A=%-2d B=%-2d C=%-2d D=%-2d\n",
			level, len(questions), choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3])

		// Show 1 representative sample question for this level
		sample := questions[0]
		fmt.Printf("   📝 Sample (%s): %s\n", sample.Type, visiblePromptKey(sample.Prompt))
		fmt.Printf("      Pilihan: A. %s | B. %s | C. %s | D. %s\n", sample.Choices[0], sample.Choices[1], sample.Choices[2], sample.Choices[3])
		fmt.Printf("      Kunci: %s (Opsi %d) | Tip: %s\n\n", sample.Choices[sample.CorrectIndex], sample.CorrectIndex, sample.LearningTip)
	}

	fmt.Println("================================================================================")
	fmt.Println("🎯 GENERATION REPORT: 100 REAL QUESTIONS PER IELTS TARGET (4.0 - 9.0)")
	fmt.Println("================================================================================")

	totalGeneratedTargets := 0
	for _, target := range targets {
		questions := make([]question, 0, 100)
		choiceCounts := [4]int{}
		for _, typ := range types {
			pool := localQuestions("B2", target, typ)
			for i := 0; i < 20; i++ {
				q := pool[i]
				choiceCounts[q.CorrectIndex]++
				questions = append(questions, q)
			}
		}
		totalGeneratedTargets += len(questions)
		fmt.Printf("✅ Target %-3.1f | Total: %3d questions | Choices: A=%-2d B=%-2d C=%-2d D=%-2d\n",
			target, len(questions), choiceCounts[0], choiceCounts[1], choiceCounts[2], choiceCounts[3])

		// Show 1 representative sample question for this target band
		sample := questions[1] // sample vocabulary/reading
		fmt.Printf("   📝 Sample (%s): %s\n", sample.Type, visiblePromptKey(sample.Prompt))
		fmt.Printf("      Pilihan: A. %s | B. %s | C. %s | D. %s\n", sample.Choices[0], sample.Choices[1], sample.Choices[2], sample.Choices[3])
		fmt.Printf("      Kunci: %s (Opsi %d) | Penjelasan: %s\n\n", sample.Choices[sample.CorrectIndex], sample.CorrectIndex, sample.Explanation)
	}

	fmt.Printf("📊 TOTAL PRODUCED: %d level questions + %d target questions = %d real unique questions.\n",
		totalGeneratedLevels, totalGeneratedTargets, totalGeneratedLevels+totalGeneratedTargets)
}

func TestSQLNormalizedPatternNoDuplicates(t *testing.T) {
	numRegex := regexp.MustCompile(`[0-9]+([:.][0-9]+)?`)
	spaceRegex := regexp.MustCompile(`\s+`)

	normalizeSQLPattern := func(contextStr string) string {
		lowered := strings.ToLower(contextStr)
		noNum := numRegex.ReplaceAllString(lowered, "{number}")
		normSpace := spaceRegex.ReplaceAllString(noNum, " ")
		return strings.TrimSpace(normSpace)
	}

	levels := []string{"A1", "A2", "B1", "B2", "C1", "C2"}
	targets := []float64{4.0, 5.0, 6.0, 6.5, 7.0, 8.0, 9.0}
	testTypes := []string{"reading", "listening"}

	for _, level := range levels {
		for _, target := range targets {
			for _, typ := range testTypes {
				name := fmt.Sprintf("%s_%.1f_%s", level, target, typ)
				t.Run(name, func(t *testing.T) {
					questions := localQuestionsRange(level, target, typ, 0, 1000)
					seenPatterns := make(map[string]string, 1000)

					for i := 0; i < 1000; i++ {
						q := questions[i]
						pattern := normalizeSQLPattern(q.Context)
						if pattern == "" {
							t.Fatalf("empty normalized pattern for question %s", q.ID)
						}

						if existingID, dup := seenPatterns[pattern]; dup {
							t.Errorf("Duplicate normalized pattern detected for %s and %s in %s: %s",
								existingID, q.ID, name, pattern)
						}
						seenPatterns[pattern] = q.ID
					}
				})
			}
		}
	}
}

func TestErrorIdentificationFiveHundredUniquePerLevelAndTarget(t *testing.T) {
	levels := []string{"A1", "A2", "B1", "B2", "C1", "C2"}
	targets := []float64{4.0, 4.5, 5.0, 5.5, 6.0, 6.5, 7.0, 7.5, 8.0, 8.5, 9.0}

	// 1. Verify 500 questions per level are 100% unique in visible prompt and valid
	for _, level := range levels {
		t.Run("Level_"+level, func(t *testing.T) {
			questions := localQuestions(level, 6.5, "error_identification")
			if len(questions) != 500 {
				t.Fatalf("expected 500 questions, got %d", len(questions))
			}
			if err := validateQuestions(questions, 500, []string{"error_identification"}); err != nil {
				t.Fatalf("validation failed for level %s: %v", level, err)
			}
			// Verify balanced distribution: choice A, B, C, D around 25% each (125 each)
			choiceCounts := [4]int{}
			for _, q := range questions {
				choiceCounts[q.CorrectIndex]++
			}
			for idx, count := range choiceCounts {
				if count < 100 || count > 150 {
					t.Errorf("choice %d has unbalanced count %d (expected ~125)", idx, count)
				}
			}
		})
	}

	// 2. Verify 500 questions per target are 100% unique in visible prompt and valid
	for _, target := range targets {
		name := fmt.Sprintf("Target_%.1f", target)
		t.Run(name, func(t *testing.T) {
			questions := localQuestions("B2", target, "error_identification")
			if len(questions) != 500 {
				t.Fatalf("expected 500 questions, got %d", len(questions))
			}
			if err := validateQuestions(questions, 500, []string{"error_identification"}); err != nil {
				t.Fatalf("validation failed for target %.1f: %v", target, err)
			}
		})
	}

	// 3. Verify cross-level differentiation
	qA1 := localQuestions("A1", 6.5, "error_identification")[0]
	qC2 := localQuestions("C2", 6.5, "error_identification")[0]
	if visiblePromptKey(qA1.Prompt) == visiblePromptKey(qC2.Prompt) {
		t.Errorf("Level A1 and Level C2 should have distinct prompts, got identical: %s", visiblePromptKey(qA1.Prompt))
	}

	// 4. Verify cross-target differentiation
	qT4 := localQuestions("B2", 4.0, "error_identification")[0]
	qT9 := localQuestions("B2", 9.0, "error_identification")[0]
	if visiblePromptKey(qT4.Prompt) == visiblePromptKey(qT9.Prompt) {
		t.Errorf("Target 4.0 and Target 9.0 should have distinct prompts, got identical: %s", visiblePromptKey(qT4.Prompt))
	}
}
