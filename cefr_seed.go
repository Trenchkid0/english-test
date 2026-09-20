package main

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	cefrQuestionSource          = "cefr_1000"
	legacyCEFRQuestionSource600 = "cefr_600"
	legacyCEFRQuestionSource900 = "cefr_900"
	cefrBaseQuota               = 600
	cefrTargetQuota             = 1000
)

var cefrQuestionTypes = []string{"vocabulary", "reading", "fill_blank", "listening", "error_identification"}
var cefrDataFiles = []string{"authentic_15000_final.json", "authentic_3000_exam_set.json"}

type cefrSeedCell struct {
	level  string
	target float64
	typ    string
}

func loadCEFRSeedQuestions(dataDir string) ([]curatedQuestionRecord, error) {
	items := make([]curatedQuestionRecord, 0, 18000)
	for _, name := range cefrDataFiles {
		batch, err := loadCuratedQuestionFile(filepath.Join(dataDir, name))
		if err != nil {
			return nil, err
		}
		items = append(items, batch...)
	}
	return items, nil
}

func seedCEFRQuestionBank(ctx context.Context, db *sql.DB, dataDir string) (int, error) {
	// Existing sources may have acquired duplicate contexts since their last
	// seed. Normalize the active bank first so the CEFR preflight compares
	// against the same clean baseline used by the final SQL audit.
	if _, err := deactivateNormalizedContextDuplicates(ctx, db); err != nil {
		return 0, fmt.Errorf("clean existing normalized context duplicates: %w", err)
	}
	records, err := loadCEFRSeedQuestions(dataDir)
	if err != nil {
		return 0, err
	}
	grouped, hashes, contexts, err := prepareCEFRSeed(records)
	if err != nil {
		return 0, err
	}
	existingHashes, existingContexts, err := existingQuestionIdentities(ctx, db)
	if err != nil {
		return 0, err
	}
	grouped, replacements, err := replaceExistingCEFRQuestions(grouped, hashes, contexts, existingHashes, existingContexts)
	if err != nil {
		return 0, err
	}
	if err := extendCEFRQuestions(grouped, existingHashes, existingContexts); err != nil {
		return 0, err
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM question_bank WHERE source IN (?,?,?)`, cefrQuestionSource, legacyCEFRQuestionSource600, legacyCEFRQuestionSource900); err != nil {
		return 0, fmt.Errorf("clear previous %s questions: %w", cefrQuestionSource, err)
	}
	for cell, questions := range grouped {
		if _, err := insertBankQuestions(ctx, db, cell.level, cell.target, cefrQuestionSource, questions); err != nil {
			return 0, fmt.Errorf("seed %s %.1f %s: %w", cell.level, cell.target, cell.typ, err)
		}
	}
	if err := verifyCEFRSeed(ctx, db); err != nil {
		return 0, err
	}
	return replacements, nil
}

func prepareCEFRSeed(records []curatedQuestionRecord) (map[cefrSeedCell][]question, map[string]string, map[string]string, error) {
	allowedLevels := make(map[string]bool, len(localLevels))
	for _, level := range localLevels {
		allowedLevels[level] = true
	}
	allowedTypes := make(map[string]bool, len(cefrQuestionTypes))
	for _, typ := range cefrQuestionTypes {
		allowedTypes[typ] = true
	}

	grouped := make(map[cefrSeedCell][]question)
	counts := make(map[string]int, len(localLevels)*len(cefrQuestionTypes))
	hashes := make(map[string]string, len(records))
	contexts := make(map[string]string, 7200)
	for index, record := range records {
		if !allowedLevels[record.Level] || !allowedTypes[record.Type] {
			return nil, nil, nil, fmt.Errorf("CEFR item %d has unsupported level/type %s/%s", index+1, record.Level, record.Type)
		}
		if strings.TrimSpace(record.Explanation) == "" || strings.TrimSpace(record.LearningTip) == "" {
			return nil, nil, nil, fmt.Errorf("CEFR item %d must contain explanation and learningTip", index+1)
		}
		q := curatedRecordQuestion(record, fmt.Sprintf("cefr-%s-%s-%05d", strings.ToLower(record.Level), record.Type, index+1))
		hash := bankQuestionHashForTarget(record.Level, record.IELTSTarget, q)
		if previous, exists := hashes[hash]; exists {
			return nil, nil, nil, fmt.Errorf("CEFR item %d duplicates %s", index+1, previous)
		}
		label := fmt.Sprintf("item %d (%s/%s/%.1f)", index+1, record.Level, record.Type, record.IELTSTarget)
		hashes[hash] = label

		if record.Type == "reading" || record.Type == "listening" {
			pattern := normalizeQuestionContext(record.Context)
			if pattern == "" {
				return nil, nil, nil, fmt.Errorf("CEFR %s has an empty context", label)
			}
			key := normalizedContextKey(record.Level, record.IELTSTarget, record.Type, pattern)
			if previous, exists := contexts[key]; exists {
				return nil, nil, nil, fmt.Errorf("CEFR %s duplicates normalized context from %s", label, previous)
			}
			contexts[key] = label
		}

		cell := cefrSeedCell{level: record.Level, target: record.IELTSTarget, typ: record.Type}
		grouped[cell] = append(grouped[cell], q)
		counts[record.Level+"|"+record.Type]++
	}
	if len(records) != len(localLevels)*len(cefrQuestionTypes)*cefrBaseQuota {
		return nil, nil, nil, fmt.Errorf("CEFR dataset contains %d questions; expected 18000", len(records))
	}
	for _, level := range localLevels {
		for _, typ := range cefrQuestionTypes {
			if count := counts[level+"|"+typ]; count != cefrBaseQuota {
				return nil, nil, nil, fmt.Errorf("CEFR dataset %s/%s contains %d questions; expected 600", level, typ, count)
			}
		}
	}
	return grouped, hashes, contexts, nil
}

func existingQuestionIdentities(ctx context.Context, db *sql.DB) (map[string]struct{}, map[string]struct{}, error) {
	hashes := make(map[string]struct{}, 220000)
	contexts := make(map[string]struct{}, 80000)
	rows, err := db.QueryContext(ctx, `SELECT content_hash FROM question_bank WHERE source NOT IN (?,?,?)`, cefrQuestionSource, legacyCEFRQuestionSource600, legacyCEFRQuestionSource900)
	if err != nil {
		return nil, nil, fmt.Errorf("read existing question hashes: %w", err)
	}
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			rows.Close()
			return nil, nil, err
		}
		hashes[hash] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `SELECT level, ielts_target, type,
		JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context'))
		FROM question_bank
		WHERE active=TRUE AND source NOT IN (?,?,?) AND type IN ('reading','listening')
		  AND JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context')) IS NOT NULL
		  AND TRIM(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context')))<>''`, cefrQuestionSource, legacyCEFRQuestionSource600, legacyCEFRQuestionSource900)
	if err != nil {
		return nil, nil, fmt.Errorf("read existing normalized contexts: %w", err)
	}
	for rows.Next() {
		var level, typ string
		var target float64
		var raw sql.NullString
		if err := rows.Scan(&level, &target, &typ, &raw); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if raw.Valid && raw.String != "" {
			pattern := normalizeQuestionContext(raw.String)
			if pattern != "" {
				contexts[normalizedContextKey(level, target, typ, pattern)] = struct{}{}
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()
	return hashes, contexts, nil
}

func replaceExistingCEFRQuestions(grouped map[cefrSeedCell][]question, datasetHashes, datasetContexts map[string]string, existingHashes, existingContexts map[string]struct{}) (map[cefrSeedCell][]question, int, error) {
	selectedHashes := make(map[string]struct{}, len(datasetHashes))
	selectedContexts := make(map[string]struct{}, len(datasetContexts))
	out := make(map[cefrSeedCell][]question, len(grouped))
	replacements := 0
	cells := make([]cefrSeedCell, 0, len(grouped))
	for cell := range grouped {
		cells = append(cells, cell)
	}
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].level != cells[j].level {
			return cells[i].level < cells[j].level
		}
		if cells[i].typ != cells[j].typ {
			return cells[i].typ < cells[j].typ
		}
		return cells[i].target < cells[j].target
	})
	for _, cell := range cells {
		questions := grouped[cell]
		accepted := make([]question, 0, len(questions))
		for _, q := range questions {
			hash := bankQuestionHashForTarget(cell.level, cell.target, q)
			_, hashExists := existingHashes[hash]
			_, hashSelected := selectedHashes[hash]
			contextKey := ""
			contextExists := false
			contextSelected := false
			if cell.typ == "reading" || cell.typ == "listening" {
				contextKey = normalizedContextKey(cell.level, cell.target, cell.typ, normalizeQuestionContext(q.Context))
				_, contextExists = existingContexts[contextKey]
				_, contextSelected = selectedContexts[contextKey]
			}
			if hashExists || hashSelected || contextExists || contextSelected {
				replacements++
				continue
			}
			accepted = append(accepted, q)
			selectedHashes[hash] = struct{}{}
			if contextKey != "" {
				selectedContexts[contextKey] = struct{}{}
			}
		}

		candidateIndex := 1000
		for len(accepted) < len(questions) {
			if candidateIndex >= 1000000 {
				return nil, replacements, fmt.Errorf("unable to generate enough new questions for %s/%.1f/%s", cell.level, cell.target, cell.typ)
			}
			q := localQuestionsRange(cell.level, cell.target, cell.typ, candidateIndex, 1)[0]
			q.ID = fmt.Sprintf("cefr-%s-%s-replacement-%06d", strings.ToLower(cell.level), cell.typ, candidateIndex)
			candidateIndex++
			hash := bankQuestionHashForTarget(cell.level, cell.target, q)
			if _, exists := existingHashes[hash]; exists {
				continue
			}
			if _, exists := selectedHashes[hash]; exists {
				continue
			}
			contextKey := ""
			if cell.typ == "reading" || cell.typ == "listening" {
				contextKey = normalizedContextKey(cell.level, cell.target, cell.typ, normalizeQuestionContext(q.Context))
				if _, exists := existingContexts[contextKey]; exists {
					continue
				}
				if _, exists := selectedContexts[contextKey]; exists {
					continue
				}
			}
			accepted = append(accepted, q)
			selectedHashes[hash] = struct{}{}
			if contextKey != "" {
				selectedContexts[contextKey] = struct{}{}
			}
		}
		out[cell] = accepted
	}
	return out, replacements, nil
}

func extendCEFRQuestions(grouped map[cefrSeedCell][]question, existingHashes, existingContexts map[string]struct{}) error {
	selectedHashes := make(map[string]struct{}, 30000)
	selectedContexts := make(map[string]struct{}, 12000)
	counts := make(map[string]int, len(localLevels)*len(cefrQuestionTypes))
	for cell, questions := range grouped {
		counts[cell.level+"|"+cell.typ] += len(questions)
		for _, q := range questions {
			selectedHashes[bankQuestionHashForTarget(cell.level, cell.target, q)] = struct{}{}
			if cell.typ == "reading" || cell.typ == "listening" {
				key := normalizedContextKey(cell.level, cell.target, cell.typ, normalizeQuestionContext(q.Context))
				selectedContexts[key] = struct{}{}
			}
		}
	}
	targetByLevel := map[string]float64{"A1": 4.0, "A2": 4.5, "B1": 5.5, "B2": 6.5, "C1": 7.5, "C2": 9.0}
	for _, level := range localLevels {
		for _, typ := range cefrQuestionTypes {
			cell := cefrSeedCell{level: level, target: targetByLevel[level], typ: typ}
			needed := cefrTargetQuota - counts[level+"|"+typ]
			candidateIndex := 1000
			for needed > 0 {
				if candidateIndex >= 100000 {
					return fmt.Errorf("unable to extend CEFR questions for %s/%.1f/%s", cell.level, cell.target, cell.typ)
				}
				q := localQuestionsRange(cell.level, cell.target, cell.typ, candidateIndex, 1)[0]
				q.ID = fmt.Sprintf("cefr-%s-%s-extra-%05d", strings.ToLower(cell.level), cell.typ, candidateIndex)
				candidateIndex++
				hash := bankQuestionHashForTarget(cell.level, cell.target, q)
				if _, exists := existingHashes[hash]; exists {
					continue
				}
				if _, exists := selectedHashes[hash]; exists {
					continue
				}
				contextKey := ""
				if cell.typ == "reading" || cell.typ == "listening" {
					contextKey = normalizedContextKey(cell.level, cell.target, cell.typ, normalizeQuestionContext(q.Context))
					if _, exists := existingContexts[contextKey]; exists {
						continue
					}
					if _, exists := selectedContexts[contextKey]; exists {
						continue
					}
				}
				grouped[cell] = append(grouped[cell], q)
				selectedHashes[hash] = struct{}{}
				if contextKey != "" {
					selectedContexts[contextKey] = struct{}{}
				}
				needed--
			}
		}
	}
	return nil
}

func verifyCEFRSeed(ctx context.Context, db *sql.DB) error {
	var cells, total, minimum, maximum int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(cell_count),0), COALESCE(MIN(cell_count),0), COALESCE(MAX(cell_count),0)
		FROM (SELECT COUNT(*) AS cell_count FROM question_bank WHERE active=TRUE AND source=? GROUP BY level,type) AS cells`, cefrQuestionSource).
		Scan(&cells, &total, &minimum, &maximum)
	if err != nil {
		return err
	}
	if cells != 30 || total != 30000 || minimum != cefrTargetQuota || maximum != cefrTargetQuota {
		return fmt.Errorf("invalid CEFR seed result: cells=%d total=%d min=%d max=%d", cells, total, minimum, maximum)
	}
	var missingExplanation, missingLearningTip int
	err = db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(TRIM(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(question_json,'$.explanation')),''))=''),0),
		COALESCE(SUM(TRIM(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(question_json,'$.learningTip')),''))=''),0)
		FROM question_bank WHERE active=TRUE AND source=?`, cefrQuestionSource).
		Scan(&missingExplanation, &missingLearningTip)
	if err != nil {
		return err
	}
	if missingExplanation != 0 || missingLearningTip != 0 {
		return fmt.Errorf("CEFR seed has %d missing explanations and %d missing learning tips", missingExplanation, missingLearningTip)
	}
	duplicates, err := normalizedContextDuplicateGroups(ctx, db)
	if err != nil {
		return err
	}
	if duplicates != 0 {
		return fmt.Errorf("database contains %d duplicate normalized reading/listening context groups after CEFR seed", duplicates)
	}
	return nil
}

func cefrQuestionBankSummary(ctx context.Context, db *sql.DB) (cells, total, minimum, maximum int, err error) {
	err = db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(cell_count),0), COALESCE(MIN(cell_count),0), COALESCE(MAX(cell_count),0)
		FROM (SELECT COUNT(*) AS cell_count FROM question_bank WHERE active=TRUE AND source=? GROUP BY level,type) AS cells`, cefrQuestionSource).
		Scan(&cells, &total, &minimum, &maximum)
	return
}
