package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

const (
	aiOriginalPrefix = "ai-original-"
	aiOriginalQuota  = 1000
	aiOriginalBatch  = 10
)

var nonWordPattern = regexp.MustCompile(`[^\p{L}\p{N}]+`)
var blankRunPattern = regexp.MustCompile(`_{3,}`)

type aiSeedCell struct {
	level  string
	target float64
	typ    string
}

type aiSeedHTTPError struct {
	status int
}

func (err *aiSeedHTTPError) Error() string {
	if err.status == http.StatusPaymentRequired {
		return "DeepSeek status 402 (payment or API balance required)"
	}
	return fmt.Sprintf("DeepSeek status %d", err.status)
}

type aiSeedFingerprints struct {
	mu       sync.Mutex
	prompts  map[string]struct{}
	contexts map[string]struct{}
	texts    map[string][]map[string]struct{}
}

func newAISeedFingerprints() *aiSeedFingerprints {
	return &aiSeedFingerprints{
		prompts:  make(map[string]struct{}, 120000),
		contexts: make(map[string]struct{}, 50000),
		texts:    make(map[string][]map[string]struct{}, 216),
	}
}

func aiSeedCellKey(cell aiSeedCell) string {
	return fmt.Sprintf("%s|%.1f|%s", cell.level, cell.target, cell.typ)
}

func aiOriginalID(cell aiSeedCell, sequence int) string {
	typ := strings.ReplaceAll(cell.typ, "_", "-")
	return fmt.Sprintf("%s%s-%.1f-%s-%04d", aiOriginalPrefix, strings.ToLower(cell.level), cell.target, typ, sequence)
}

func aiOriginalCounts(ctx context.Context, db *sql.DB) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT level,ielts_target,type,COUNT(*)
		FROM question_bank
		WHERE active=TRUE AND source=?
		  AND JSON_UNQUOTE(JSON_EXTRACT(question_json,'$.id')) LIKE ?
		GROUP BY level,ielts_target,type`, authenticQuestionSource, aiOriginalPrefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int, 216)
	for rows.Next() {
		var cell aiSeedCell
		var count int
		if err := rows.Scan(&cell.level, &cell.target, &cell.typ, &count); err != nil {
			return nil, err
		}
		counts[aiSeedCellKey(cell)] = count
	}
	return counts, rows.Err()
}

func loadAISeedFingerprints(ctx context.Context, db *sql.DB) (*aiSeedFingerprints, error) {
	fingerprints := newAISeedFingerprints()
	rows, err := db.QueryContext(ctx, `SELECT level,ielts_target,type,
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(question_json,'$.id')),''),
		JSON_UNQUOTE(JSON_EXTRACT(question_json,'$.prompt')),
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(question_json,'$.context')),'')
		FROM question_bank
		WHERE active=TRUE
		  AND level IN ('A1','A2','B1','B2','C1','C2')
		  AND ielts_target>=5
		  AND type IN ('grammar','vocabulary','reading','fill_blank','listening','error_identification')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cell aiSeedCell
		var id, prompt, questionContext string
		if err := rows.Scan(&cell.level, &cell.target, &cell.typ, &id, &prompt, &questionContext); err != nil {
			return nil, err
		}
		fingerprints.prompts[normalizedContextKey(cell.level, cell.target, cell.typ, normalizeAIPattern(prompt))] = struct{}{}
		if cell.typ == "reading" || cell.typ == "listening" {
			fingerprints.contexts[normalizedContextKey(cell.level, cell.target, cell.typ, normalizeQuestionContext(questionContext))] = struct{}{}
		}
		if strings.HasPrefix(id, aiOriginalPrefix) {
			fingerprints.addSimilarityText(cell, prompt, questionContext)
		}
	}
	return fingerprints, rows.Err()
}

func normalizeAIPattern(value string) string {
	value = strings.ToLower(visiblePromptKey(value))
	value = contextNumberPattern.ReplaceAllString(value, "{number}")
	value = nonWordPattern.ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(value), " ")
}

func similarityTokens(prompt, questionContext string) map[string]struct{} {
	text := prompt
	if strings.TrimSpace(questionContext) != "" {
		text = questionContext + " " + prompt
	}
	words := strings.Fields(normalizeAIPattern(text))
	tokens := make(map[string]struct{}, len(words))
	for _, word := range words {
		if len([]rune(word)) >= 4 {
			tokens[word] = struct{}{}
		}
	}
	return tokens
}

func tokenJaccard(left, right map[string]struct{}) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	intersection := 0
	for token := range left {
		if _, exists := right[token]; exists {
			intersection++
		}
	}
	union := len(left) + len(right) - intersection
	return float64(intersection) / float64(union)
}

func (fingerprints *aiSeedFingerprints) addSimilarityText(cell aiSeedCell, prompt, questionContext string) {
	key := aiSeedCellKey(cell)
	fingerprints.texts[key] = append(fingerprints.texts[key], similarityTokens(prompt, questionContext))
}

func (fingerprints *aiSeedFingerprints) accept(cell aiSeedCell, q question) error {
	fingerprints.mu.Lock()
	defer fingerprints.mu.Unlock()

	promptPattern := normalizeAIPattern(q.Prompt)
	if promptPattern == "" {
		return errors.New("empty normalized prompt")
	}
	promptKey := normalizedContextKey(cell.level, cell.target, cell.typ, promptPattern)
	if _, exists := fingerprints.prompts[promptKey]; exists {
		return errors.New("duplicate normalized prompt")
	}
	contextKey := ""
	if cell.typ == "reading" || cell.typ == "listening" {
		pattern := normalizeQuestionContext(q.Context)
		if pattern == "" {
			return errors.New("empty normalized context")
		}
		contextKey = normalizedContextKey(cell.level, cell.target, cell.typ, pattern)
		if _, exists := fingerprints.contexts[contextKey]; exists {
			return errors.New("duplicate normalized context")
		}
	}
	tokens := similarityTokens(q.Prompt, q.Context)
	fingerprints.prompts[promptKey] = struct{}{}
	if contextKey != "" {
		fingerprints.contexts[contextKey] = struct{}{}
	}
	fingerprints.texts[aiSeedCellKey(cell)] = append(fingerprints.texts[aiSeedCellKey(cell)], tokens)
	return nil
}

func validateAIOriginalQuestion(q question, typ string) error {
	if err := validateQuestions([]question{q}, 1, []string{typ}); err != nil {
		return err
	}
	answer := strings.TrimSpace(q.Choices[q.CorrectIndex])
	explanation := strings.ToLower(q.Explanation)
	if !strings.Contains(explanation, strings.ToLower(answer)) {
		return errors.New("explanation does not name the exact correct answer")
	}
	if !strings.Contains(explanation, "karena") && !strings.Contains(explanation, "because") {
		return errors.New("explanation does not explain why the answer is correct")
	}
	if len(strings.Fields(q.Explanation)) < 20 {
		return errors.New("explanation is too short")
	}
	if hasVisibleTemplateMarker(q.Prompt) || hasVisibleTemplateMarker(q.Context) {
		return errors.New("visible template or internal metadata marker")
	}
	switch typ {
	case "reading":
		words := len(strings.Fields(q.Context))
		if words < 80 || words > 180 {
			return fmt.Errorf("reading context has %d words", words)
		}
	case "listening":
		if strings.Count(q.Context, "\n") < 5 || strings.Count(q.Context, ":") < 6 {
			return errors.New("listening dialogue has too few turns")
		}
	case "fill_blank":
		if strings.Count(q.Prompt, "_____") != 1 {
			return errors.New("fill_blank must contain exactly one blank")
		}
	case "error_identification":
		if strings.Count(q.Prompt, "[") != 4 || strings.Count(q.Prompt, "]") != 4 {
			return errors.New("error_identification must mark exactly four segments")
		}
	}
	return nil
}

func normalizeAIGeneratedQuestion(q question, typ string) question {
	q.Prompt = strings.TrimSpace(q.Prompt)
	q.Context = strings.TrimSpace(q.Context)
	q.Evidence = strings.TrimSpace(q.Evidence)
	q.Explanation = strings.TrimSpace(q.Explanation)
	q.LearningTip = strings.TrimSpace(q.LearningTip)
	q.IELTSSkill = strings.TrimSpace(q.IELTSSkill)
	for index := range q.Choices {
		q.Choices[index] = strings.TrimSpace(q.Choices[index])
	}
	if q.CorrectIndex >= 0 && q.CorrectIndex < len(q.Choices) {
		answer := q.Choices[q.CorrectIndex]
		if answer != "" && !strings.Contains(strings.ToLower(q.Explanation), strings.ToLower(answer)) {
			q.Explanation = fmt.Sprintf("Jawaban yang benar adalah ‘%s’ karena %s", answer, q.Explanation)
		}
	}
	if typ == "fill_blank" {
		q.Prompt = blankRunPattern.ReplaceAllString(q.Prompt, "_____")
	}
	return q
}

func aiBandGuidance(target float64) string {
	switch {
	case target <= 5.5:
		return "Use clear B1-B2 language, direct evidence, and one plausible linguistic challenge."
	case target <= 6.5:
		return "Use upper-intermediate academic language, paraphrased evidence, and plausible distractors requiring careful comparison."
	case target <= 7.5:
		return "Use advanced academic language, nuanced inference or grammar, and distractors that differ by scope, stance, or precision."
	default:
		return "Use sophisticated C1-C2 discourse, subtle qualification, dense but natural syntax, and highly plausible distractors distinguished by precise meaning."
	}
}

func originalTypeRules(typ string) string {
	switch typ {
	case "grammar":
		return "Create self-contained grammar-in-context questions. Vary the tested construction organically; do not repeat an opening frame or merely swap nouns."
	case "vocabulary":
		return "Test meaning, collocation, register, connotation, or lexical precision inside an original context. Do not write bare dictionary-definition questions."
	case "reading":
		return "Each item needs a wholly original 80-180 word passage and an evidence field copied verbatim from it. Vary genre, rhetorical purpose, reasoning, and question operation."
	case "fill_blank":
		return "Each prompt contains exactly one _____. Vary discourse cohesion, grammar, and lexical collocation; make the surrounding situation independently conceived."
	case "listening":
		return "Each context is a wholly original 6-10 turn dialogue with speaker labels, natural repair or clarification, and one final key detail. Do not reuse a booking-dialogue skeleton."
	case "error_identification":
		return "Each prompt marks exactly four segments with square brackets and exactly one segment is wrong. Vary sentence architecture and error category. Choices equal the four marked segments."
	default:
		return "Create independently conceived questions."
	}
}

func (a *app) generateAIOriginalBatch(ctx context.Context, cell aiSeedCell, count, sequence int) ([]question, error) {
	requestPrompt := fmt.Sprintf(`Write exactly %d independently conceived IELTS practice questions.
Level: %s. Target band: %.1f. Type: %s. Starting sequence: %d.

NON-TEMPLATE REQUIREMENTS:
- Invent every item independently from a different real-world or academic situation.
- Do not use a reusable sentence skeleton, repeated opening phrase, numbered-case pattern, or variations made by swapping names, places, topics, or numbers.
- Do not place level, band, sequence, batch, or internal metadata in visible content.
- Vary sentence architecture, communicative purpose, subject domain, reasoning operation, and distractor logic across items.
- Do not copy official IELTS/Cambridge questions or any earlier item.

Difficulty calibration: %s
Type requirements: %s

Every item must have four distinct plausible choices and a balanced correctIndex. Write a specific Indonesian explanation of 2-4 sentences and at least 20 words. It MUST begin with "Jawaban yang benar adalah ‘<exact choice>’ karena ...", state the decisive grammar rule, lexical meaning, or passage/dialogue evidence, and explain why at least one tempting distractor is wrong. Never use generic claims such as "sesuai konteks" without naming the actual rule or evidence. learningTip must be actionable and specific.

Return JSON only:
{"questions":[{"type":"%s","context":"","evidence":"","errorTag":"grammar|vocabulary|detail|inference","prompt":"","choices":["","","",""],"correctIndex":0,"explanation":"","learningTip":"","ieltsSkill":""}]}`,
		count, cell.level, cell.target, cell.typ, sequence, aiBandGuidance(cell.target), originalTypeRules(cell.typ), cell.typ)
	payload := map[string]any{
		"model": a.cfg.DeepSeekModel, "temperature": 1.0,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": "You are an expert IELTS assessment writer. Create each question from first principles; templated or mechanically varied items are unacceptable. Return valid JSON only."},
			{"role": "user", "content": requestPrompt},
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.DeepSeekBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.DeepSeekKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &aiSeedHTTPError{status: resp.StatusCode}
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &completion); err != nil || len(completion.Choices) == 0 {
		return nil, errors.New("invalid DeepSeek completion")
	}
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	var result struct {
		Questions []question `json:"questions"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &result); err != nil {
		return nil, err
	}
	if len(result.Questions) != count {
		return nil, fmt.Errorf("DeepSeek returned %d questions; expected %d", len(result.Questions), count)
	}
	return result.Questions, nil
}

func seedAIOriginalQuestions(ctx context.Context, a *app, pilot bool) error {
	fingerprints, err := loadAISeedFingerprints(ctx, a.db)
	if err != nil {
		return err
	}
	counts, err := aiOriginalCounts(ctx, a.db)
	if err != nil {
		return err
	}
	var insertedThisRun atomic.Int64
	if !pilot {
		taskContext, cancelTasks := context.WithCancel(ctx)
		defer cancelTasks()
		tasks := make(chan aiSeedCell, len(ieltsTargetedLevels)*len(ieltsTargetedTargets)*len(ieltsTargetedTypes))
		for _, level := range ieltsTargetedLevels {
			for _, target := range ieltsTargetedTargets {
				for _, typ := range ieltsTargetedTypes {
					tasks <- aiSeedCell{level: level, target: target, typ: typ}
				}
			}
		}
		close(tasks)

		errChannel := make(chan error, 1)
		var workers sync.WaitGroup
		const workerCount = 8
		for worker := 0; worker < workerCount; worker++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for cell := range tasks {
					if taskContext.Err() != nil {
						return
					}
					current := counts[aiSeedCellKey(cell)]
					if err := seedAIOriginalCell(taskContext, a, fingerprints, cell, current, aiOriginalQuota, &insertedThisRun); err != nil {
						select {
						case errChannel <- err:
						default:
						}
						cancelTasks()
						return
					}
				}
			}()
		}
		workers.Wait()
		select {
		case err := <-errChannel:
			return err
		default:
		}
		return verifyAIOriginalQuestions(ctx, a.db)
	}

	for _, level := range ieltsTargetedLevels {
		for _, target := range ieltsTargetedTargets {
			for _, typ := range ieltsTargetedTypes {
				cell := aiSeedCell{level: level, target: target, typ: typ}
				key := aiSeedCellKey(cell)
				if err := seedAIOriginalCell(ctx, a, fingerprints, cell, counts[key], 1, &insertedThisRun); err != nil {
					return err
				}
			}
			return nil
		}
	}
	return nil
}

func seedAIOriginalCell(ctx context.Context, a *app, fingerprints *aiSeedFingerprints, cell aiSeedCell, current, quota int, insertedThisRun *atomic.Int64) error {
	key := aiSeedCellKey(cell)
	failedAttempts := 0
	cursor := current
	for current < quota {
		needed := min(aiOriginalBatch, quota-current)
		questions := generateCodexOriginalBatch(cell, needed, cursor+1)
		cursor += needed
		accepted := make([]question, 0, len(questions))
		for _, q := range questions {
			q = normalizeAIGeneratedQuestion(q, cell.typ)
			q.Type = cell.typ
			q.ID = aiOriginalID(cell, current+len(accepted)+1)
			q.ReviewKey = ""
			if err := validateAIOriginalQuestion(q, cell.typ); err != nil {
				log.Printf("AI seed rejected %s: %v", key, err)
				continue
			}
			if err := fingerprints.accept(cell, q); err != nil {
				log.Printf("AI seed rejected %s: %v", key, err)
				continue
			}
			accepted = append(accepted, q)
		}
		if len(accepted) == 0 {
			failedAttempts++
			if failedAttempts >= 8 {
				return fmt.Errorf("quality gate rejected eight consecutive batches for %s", key)
			}
			continue
		}
		if _, err := insertBankQuestions(ctx, a.db, cell.level, cell.target, authenticQuestionSource, accepted); err != nil {
			return fmt.Errorf("insert AI batch %s: %w", key, err)
		}
		current += len(accepted)
		inserted := insertedThisRun.Add(int64(len(accepted)))
		failedAttempts = 0
		log.Printf("AI seed progress %s=%d/%d inserted_this_run=%d", key, current, quota, inserted)
	}
	return nil
}

func verifyAIOriginalQuestions(ctx context.Context, db *sql.DB) error {
	counts, err := aiOriginalCounts(ctx, db)
	if err != nil {
		return err
	}
	for _, level := range ieltsTargetedLevels {
		for _, target := range ieltsTargetedTargets {
			for _, typ := range ieltsTargetedTypes {
				cell := aiSeedCell{level: level, target: target, typ: typ}
				if counts[aiSeedCellKey(cell)] != aiOriginalQuota {
					return fmt.Errorf("AI original cell %s contains %d/%d questions", aiSeedCellKey(cell), counts[aiSeedCellKey(cell)], aiOriginalQuota)
				}
			}
		}
	}
	duplicates, err := normalizedContextDuplicateGroups(ctx, db)
	if err != nil {
		return err
	}
	if duplicates != 0 {
		return fmt.Errorf("database contains %d duplicate normalized reading/listening contexts", duplicates)
	}
	return nil
}

func hasVisibleTemplateMarker(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return true
		}
	}
	return strings.Contains(value, "[Level ") || strings.Contains(value, "Practice ") || strings.Contains(value, "(Case ")
}
