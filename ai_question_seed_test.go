package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestAISeedPaymentErrorIsTyped(t *testing.T) {
	err := error(&aiSeedHTTPError{status: http.StatusPaymentRequired})
	var statusError *aiSeedHTTPError
	if !errors.As(err, &statusError) || !strings.Contains(statusError.Error(), "balance") {
		t.Fatalf("payment error is not actionable: %v", err)
	}
}

func TestAISeedFingerprintRejectsMechanicalVariations(t *testing.T) {
	cell := aiSeedCell{level: "B2", target: 6.5, typ: "reading"}
	fingerprints := newAISeedFingerprints()
	first := question{
		Type: "reading", Context: "A coastal laboratory tracked eleven seal colonies during winter. Researchers found that artificial harbour lighting changed the timing of nocturnal feeding.",
		Prompt: "What effect did harbour lighting have on the seals?",
	}
	if err := fingerprints.accept(cell, first); err != nil {
		t.Fatalf("first original question rejected: %v", err)
	}
	mechanicalCopy := question{
		Type: "reading", Context: "A coastal laboratory tracked twenty-seven seal colonies during winter. Researchers found that artificial harbour lighting changed the timing of nocturnal feeding.",
		Prompt: "What effect did harbour lighting have on the seals?",
	}
	if err := fingerprints.accept(cell, mechanicalCopy); err == nil {
		t.Fatal("number-only variation was accepted")
	}
}

func TestAIOriginalQuestionRequiresAnswerInExplanation(t *testing.T) {
	q := question{
		Type: "vocabulary", Prompt: "Which option best conveys the author's cautious stance?",
		Choices: []string{"tentative", "absolute", "careless", "irrelevant"}, CorrectIndex: 0,
		Explanation: "Pilihan pertama sesuai dengan sikap penulis dan distraktor lainnya terlalu kuat untuk konteks tersebut.",
		LearningTip: "Bandingkan kekuatan makna setiap pilihan dengan sikap penulis.", IELTSSkill: "Lexical Resource",
	}
	if err := validateAIOriginalQuestion(q, "vocabulary"); err == nil {
		t.Fatal("explanation without the exact correct answer was accepted")
	}
	q.Explanation = "Jawaban yang benar adalah ‘tentative’ karena kata tersebut menunjukkan sikap belum pasti; ‘absolute’ justru menyatakan kepastian penuh yang tidak didukung konteks."
	if err := validateAIOriginalQuestion(q, "vocabulary"); err != nil {
		t.Fatalf("valid explanation rejected: %v", err)
	}
}

func TestAIOriginalQuestionRequiresReasonWhyAnswerIsCorrect(t *testing.T) {
	q := question{
		Type: "grammar", Prompt: "Neither proposal _____ sufficient evidence.",
		Choices: []string{"contain", "contains", "containing", "have contained"}, CorrectIndex: 1,
		Explanation: "Jawaban yang benar adalah ‘contains’. Pilihan ini merupakan bentuk yang paling tepat untuk kalimat tersebut dan pilihan lain tidak tepat digunakan pada posisi ini.",
		LearningTip: "Cari inti subjek sebelum menentukan agreement kata kerja.", IELTSSkill: "Grammatical Range and Accuracy",
	}
	if err := validateAIOriginalQuestion(q, "grammar"); err == nil {
		t.Fatal("generic explanation without a reason was accepted")
	}
	q.Explanation = "Jawaban yang benar adalah ‘contains’ karena inti subjek ‘Neither proposal’ bersifat tunggal, sehingga present simple memerlukan akhiran -s; ‘contain’ memakai agreement jamak."
	if err := validateAIOriginalQuestion(q, "grammar"); err != nil {
		t.Fatalf("specific causal explanation rejected: %v", err)
	}
}

func TestNormalizeAIGeneratedFillBlank(t *testing.T) {
	q := normalizeAIGeneratedQuestion(question{
		Prompt: "The committee ________ the proposal.", Choices: []string{"approved", "approve", "approving", "approval"}, CorrectIndex: 0,
		Explanation: "Bentuk lampau diperlukan karena keputusan terjadi kemarin dan pilihan lainnya tidak sesuai dengan penanda waktu.",
	}, "fill_blank")
	if q.Prompt != "The committee _____ the proposal." {
		t.Fatalf("unexpected normalized prompt: %q", q.Prompt)
	}
	if !strings.Contains(q.Explanation, "approved") {
		t.Fatalf("correct answer was not added to explanation: %q", q.Explanation)
	}
}

func TestCodexOriginalBatchHasReasonsAndUniqueNormalizedContexts(t *testing.T) {
	for _, typ := range ieltsTargetedTypes {
		cell := aiSeedCell{level: "B1", target: 5, typ: typ}
		questions := generateCodexOriginalBatch(cell, 1000, 1)
		contexts := map[string]struct{}{}
		fingerprints := newAISeedFingerprints()
		for index, q := range questions {
			q = normalizeAIGeneratedQuestion(q, typ)
			q.Type = typ
			q.ID = aiOriginalID(cell, index+1)
			if err := validateAIOriginalQuestion(q, typ); err != nil {
				t.Fatalf("%s question %d failed validation: %v; prompt=%q", typ, index+1, err, q.Prompt)
			}
			if err := fingerprints.accept(cell, q); err != nil {
				t.Fatalf("%s question %d failed uniqueness gate: %v; prompt=%q", typ, index+1, err, q.Prompt)
			}
			if typ == "reading" || typ == "listening" {
				pattern := normalizeQuestionContext(q.Context)
				if _, exists := contexts[pattern]; exists {
					t.Fatalf("%s question %d has a duplicate normalized context", typ, index+1)
				}
				contexts[pattern] = struct{}{}
			}
		}
	}
}
