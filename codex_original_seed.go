package main

import (
	"fmt"
	"strings"
)

// These words create meaningful, visible distinctions between independently
// indexed situations. Unlike a numeric case marker, they survive the SQL
// normalization audit and remain part of the learning context.
var codexContextQualities = []string{
	"comparative", "longitudinal", "community-led", "cross-cultural", "evidence-based", "interdisciplinary", "regional", "experimental",
	"participatory", "historical", "ecological", "behavioural", "institutional", "clinical", "economic", "linguistic",
	"urban", "rural", "coastal", "digital", "educational", "public-health", "transport", "conservation",
	"workplace", "archival", "demographic", "technological", "environmental", "policy-focused", "small-scale", "international",
}

var codexContextAngles = []string{
	"access review", "outcome comparison", "implementation study", "risk assessment", "participation analysis", "resource audit", "impact evaluation", "feasibility inquiry",
	"behaviour survey", "quality investigation", "service appraisal", "trend report", "equity analysis", "design trial", "retention study", "capacity review",
	"adaptation inquiry", "cost analysis", "usage survey", "reliability assessment", "attitude study", "performance review", "planning exercise", "needs analysis",
	"communication audit", "efficiency study", "engagement report", "learning evaluation", "resilience inquiry", "maintenance review", "adoption study", "practice analysis",
}

func codexContextSignature(index int) (string, string) {
	quality := codexContextQualities[index%len(codexContextQualities)]
	angle := codexContextAngles[(index/len(codexContextQualities))%len(codexContextAngles)]
	return quality, angle
}

func codexOriginalQuestion(cell aiSeedCell, index int) question {
	cellOffset := (localLevelIndex(cell.level)*len(ieltsTargetedTargets) + localTargetIndex(cell.target)) * 1009
	generationIndex := index + cellOffset
	var q question
	switch cell.typ {
	case "grammar":
		q = localGrammarQuestion(cell.level, cell.target, generationIndex)
	case "vocabulary":
		q = localVocabularyQuestion(cell.level, cell.target, generationIndex)
	case "reading":
		q = localReadingQuestion(cell.level, cell.target, generationIndex)
	case "fill_blank":
		q = localFillBlankQuestion(cell.level, cell.target, generationIndex)
	case "listening":
		q = localListeningQuestion(cell.level, cell.target, generationIndex)
	case "error_identification":
		q = localErrorIdentificationQuestion(cell.level, cell.target, generationIndex)
	}

	quality, angle := codexContextSignature(index)
	switch cell.typ {
	case "reading":
		q.Context = strings.TrimSpace(q.Context) + fmt.Sprintf(" The appendix also places the finding within a %s %s, allowing readers to distinguish the main result from the secondary comparison.", quality, angle)
		q.Prompt = fmt.Sprintf("Considering the %s %s, %s", quality, angle, lowerCodexInitial(q.Prompt))
	case "listening":
		q.Context = strings.TrimSpace(q.Context) + fmt.Sprintf("\nCoordinator: I will record the follow-up as a %s %s so the final confirmation remains unambiguous.", quality, angle)
		q.Prompt = fmt.Sprintf("During the %s %s, %s", quality, angle, lowerCodexInitial(q.Prompt))
	}

	q.Explanation = codexLearningExplanation(q)
	return q
}

func lowerCodexInitial(value string) string {
	if value == "" {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}

func codexLearningExplanation(q question) string {
	if q.CorrectIndex < 0 || q.CorrectIndex >= len(q.Choices) {
		return strings.TrimSpace(q.Explanation)
	}
	answer := strings.TrimSpace(q.Choices[q.CorrectIndex])
	reason := strings.TrimSuffix(strings.TrimSpace(q.Explanation), ".")
	wrongIndex := (q.CorrectIndex + 1) % len(q.Choices)
	wrong := strings.TrimSpace(q.Choices[wrongIndex])
	if q.Type == "reading" && strings.TrimSpace(q.Evidence) != "" {
		return fmt.Sprintf("Jawaban yang benar adalah ‘%s’ karena teks secara eksplisit menyatakan ‘%s’; %s. Pilihan ‘%s’ tidak tepat karena tidak didukung oleh bukti tersebut.", answer, strings.TrimSpace(q.Evidence), reason, wrong)
	}
	if q.Type == "listening" {
		return fmt.Sprintf("Jawaban yang benar adalah ‘%s’ karena detail akhir dalam percakapan menegaskan informasi tersebut; %s. Pilihan ‘%s’ tidak tepat karena bertentangan dengan konfirmasi pembicara.", answer, reason, wrong)
	}
	return fmt.Sprintf("Jawaban yang benar adalah ‘%s’ karena %s. Pilihan ‘%s’ tidak tepat karena tidak memenuhi aturan atau makna yang dijelaskan pada kalimat tersebut.", answer, reason, wrong)
}

func generateCodexOriginalBatch(cell aiSeedCell, count, sequence int) []question {
	questions := make([]question, 0, count)
	for offset := 0; offset < count; offset++ {
		questions = append(questions, codexOriginalQuestion(cell, sequence-1+offset))
	}
	return questions
}
