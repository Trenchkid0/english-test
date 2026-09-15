package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestCEFRBaseDatasetAndOneThousandExtension(t *testing.T) {
	files := []string{"authentic_15000_final.json", "authentic_3000_exam_set.json"}
	allowedTypes := map[string]bool{
		"vocabulary": true, "reading": true, "fill_blank": true,
		"listening": true, "error_identification": true,
	}
	counts := map[string]int{}
	seen := map[string]string{}
	total := 0
	missingExplanation := 0
	missingLearningTip := 0
	duplicates := 0
	allRecords := make([]curatedQuestionRecord, 0, 18000)
	for _, name := range files {
		items, err := loadCuratedQuestionFile(filepath.Join("data", "questions", name))
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		allRecords = append(allRecords, items...)
		for index, item := range items {
			if !allowedTypes[item.Type] {
				t.Fatalf("%s item %d has unsupported type %q", name, index+1, item.Type)
			}
			key := item.Level + "|" + item.Type
			counts[key]++
			total++
			if strings.TrimSpace(item.Explanation) == "" {
				missingExplanation++
			}
			if strings.TrimSpace(item.LearningTip) == "" {
				missingLearningTip++
			}
			q := curatedRecordQuestion(item, fmt.Sprintf("audit-%d", total))
			identity := bankQuestionHashForTarget(item.Level, item.IELTSTarget, q)
			if previous, exists := seen[identity]; exists {
				_ = previous
				duplicates++
				continue
			}
			seen[identity] = fmt.Sprintf("%s item %d", name, index+1)
		}
	}
	if total != 18000 {
		t.Fatalf("dataset has %d questions; expected 18000", total)
	}
	if missingExplanation != 0 || missingLearningTip != 0 {
		t.Errorf("dataset has %d missing explanations and %d missing learning tips", missingExplanation, missingLearningTip)
	}
	if duplicates != 0 {
		t.Errorf("dataset has %d duplicate questions and %d unique questions", duplicates, len(seen))
	}
	grouped, _, _, err := prepareCEFRSeed(allRecords)
	if err != nil {
		t.Errorf("CEFR seed preflight failed: %v", err)
	} else if err := extendCEFRQuestions(grouped, map[string]struct{}{}, map[string]struct{}{}); err != nil {
		t.Errorf("CEFR extension failed: %v", err)
	} else {
		extendedCounts := map[string]int{}
		for cell, questions := range grouped {
			extendedCounts[cell.level+"|"+cell.typ] += len(questions)
		}
		for _, level := range localLevels {
			for typ := range allowedTypes {
				if count := extendedCounts[level+"|"+typ]; count != 1000 {
					t.Errorf("extended %s %s has %d questions; expected 1000", level, typ, count)
				}
			}
		}
	}
	for _, level := range localLevels {
		for typ := range allowedTypes {
			if count := counts[level+"|"+typ]; count != 600 {
				t.Errorf("%s %s has %d questions; expected 600", level, typ, count)
			}
		}
	}
}
