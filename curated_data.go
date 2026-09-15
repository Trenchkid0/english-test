package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type curatedQuestionRecord struct {
	Level        string
	IELTSTarget  float64
	Type         string
	Context      string
	Evidence     string
	ErrorTag     string
	Prompt       string
	Choices      []string
	CorrectIndex int
	Explanation  string
	LearningTip  string
	IELTSSkill   string
}

type curatedQuestionJSON struct {
	Level             string   `json:"level"`
	IELTSTarget       *float64 `json:"ieltsTarget"`
	IELTSTargetSnake  *float64 `json:"ielts_target"`
	Type              string   `json:"type"`
	Context           string   `json:"context"`
	Evidence          string   `json:"evidence"`
	ReadingEvidence   string   `json:"reading_evidence"`
	ErrorTag          string   `json:"errorTag"`
	ErrorTagSnake     string   `json:"error_tag"`
	Prompt            string   `json:"prompt"`
	Choices           []string `json:"choices"`
	CorrectIndex      *int     `json:"correctIndex"`
	CorrectIndexSnake *int     `json:"correct_index"`
	CorrectAnswer     string   `json:"correct_answer"`
	Explanation       string   `json:"explanation"`
	LearningTip       string   `json:"learningTip"`
	LearningTipSnake  string   `json:"learning_tip"`
	IELTSSkill        string   `json:"ieltsSkill"`
	IELTSSkillSnake   string   `json:"ielts_skill"`
	Skill             string   `json:"skill"`
}

var curatedDataFiles = []string{
	"batch1_b1_b2_authentic.json",
	"curated_batch2.json",
	"curated_batch3.json",
	"curated_batch4_skills.json",
	"curated_batch5_expansion.json",
	"curated_batch6_expansion.json",
	"curated_batch7_expansion.json",
	"curated_batch8_expansion.json",
	"curated_batch9_expansion.json",
	"curated_batch10_expansion.json",
	"curated_batch11_expansion.json",
	"curated_batch12_expansion.json",
}

func loadCuratedQuestions(dataDir string) ([]curatedQuestionRecord, error) {
	questions := make([]curatedQuestionRecord, 0, 5365)
	for _, name := range curatedDataFiles {
		batch, err := loadCuratedQuestionFile(filepath.Join(dataDir, name))
		if err != nil {
			return nil, err
		}
		questions = append(questions, batch...)
	}
	return questions, nil
}

func loadCuratedQuestionFile(path string) ([]curatedQuestionRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open curated data %s: %w", path, err)
	}
	defer file.Close()

	var raw []curatedQuestionJSON
	if err := json.NewDecoder(file).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode curated data %s: %w", path, err)
	}
	questions := make([]curatedQuestionRecord, 0, len(raw))
	for index, item := range raw {
		question, err := normalizeCuratedQuestion(item)
		if err != nil {
			return nil, fmt.Errorf("curated data %s item %d: %w", path, index+1, err)
		}
		questions = append(questions, question)
	}
	return questions, nil
}

func normalizeCuratedQuestion(item curatedQuestionJSON) (curatedQuestionRecord, error) {
	target := 0.0
	if item.IELTSTarget != nil {
		target = *item.IELTSTarget
	} else if item.IELTSTargetSnake != nil {
		target = *item.IELTSTargetSnake
	}
	correctIndex := -1
	if item.CorrectIndex != nil {
		correctIndex = *item.CorrectIndex
	} else if item.CorrectIndexSnake != nil {
		correctIndex = *item.CorrectIndexSnake
	} else if item.CorrectAnswer != "" {
		for index, choice := range item.Choices {
			if choice == item.CorrectAnswer {
				correctIndex = index
				break
			}
		}
	}
	if item.Level == "" || item.Type == "" || item.Prompt == "" {
		return curatedQuestionRecord{}, fmt.Errorf("level, type, and prompt are required")
	}
	if len(item.Choices) != 4 {
		return curatedQuestionRecord{}, fmt.Errorf("expected 4 choices, got %d", len(item.Choices))
	}
	if correctIndex < 0 || correctIndex >= len(item.Choices) {
		return curatedQuestionRecord{}, fmt.Errorf("correct answer is not present in choices")
	}
	evidence := item.Evidence
	if evidence == "" {
		evidence = item.ReadingEvidence
	}
	errorTag := item.ErrorTag
	if errorTag == "" {
		errorTag = item.ErrorTagSnake
	}
	learningTip := item.LearningTip
	if learningTip == "" {
		learningTip = item.LearningTipSnake
	}
	ieltsSkill := item.IELTSSkill
	if ieltsSkill == "" {
		ieltsSkill = item.IELTSSkillSnake
	}
	if ieltsSkill == "" {
		ieltsSkill = item.Skill
	}
	return curatedQuestionRecord{
		Level: item.Level, IELTSTarget: target, Type: item.Type, Context: item.Context,
		Evidence: evidence, ErrorTag: errorTag, Prompt: item.Prompt, Choices: item.Choices,
		CorrectIndex: correctIndex, Explanation: item.Explanation, LearningTip: learningTip,
		IELTSSkill: ieltsSkill,
	}, nil
}

func curatedRecordQuestion(item curatedQuestionRecord, id string) question {
	q := question{
		ID: id, Type: item.Type, Context: item.Context, Evidence: item.Evidence,
		ErrorTag: item.ErrorTag, Prompt: item.Prompt, Choices: item.Choices,
		CorrectIndex: item.CorrectIndex, Explanation: item.Explanation,
		LearningTip: item.LearningTip, IELTSSkill: item.IELTSSkill,
	}
	q.Explanation = learnerFriendlyExplanation(q)
	return q
}
