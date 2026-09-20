package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// ieltsTargetedQuota is the minimum unique question count per
// (level × ielts_target × type) cell in the IELTS-targeted bank.
// With 4 levels × 9 bands × 6 types this equates to 216,000 active questions.
const ieltsTargetedQuota = 1000

var ieltsTargetedLevels = []string{"A1", "A2", "B1", "B2", "C1", "C2"}
var ieltsTargetedTargets = []float64{5, 5.5, 6, 6.5, 7, 7.5, 8, 8.5, 9}
var ieltsTargetedTypes = []string{"grammar", "vocabulary", "reading", "fill_blank", "listening", "error_identification"}

// seedIELTSTargetedQuestionBank builds the exact matrix requested for current
// IELTS practice: 6 levels × 9 bands × 6 types × 1000 questions = 324,000.
func seedIELTSTargetedQuestionBank(ctx context.Context, db *sql.DB) error {
	if _, err := deactivateNormalizedContextDuplicates(ctx, db); err != nil {
		return fmt.Errorf("clean normalized reading/listening context duplicates: %w", err)
	}
	if _, err := trimIELTSTargetedCells(ctx, db); err != nil {
		return fmt.Errorf("trim IELTS cells to quota: %w", err)
	}
	if err := ensureLocalQuestionMatrixWithDBCheck(ctx, db, ieltsTargetedLevels, ieltsTargetedTargets, ieltsTargetedTypes, ieltsTargetedQuota); err != nil {
		return err
	}
	return verifyIELTSTargetedQuestionBank(ctx, db)
}

// trimIELTSTargetedCells is recoverable: surplus rows are deactivated, not
// deleted. This makes repeated seeds converge to the configured active quota.
func trimIELTSTargetedCells(ctx context.Context, db *sql.DB) (int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, level, ielts_target, type FROM question_bank WHERE active=TRUE AND source=? ORDER BY id ASC`, localQuestionSource)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	var toDeactivate []int64

	for rows.Next() {
		var (
			id     int64
			level  string
			target float64
			typ    string
		)
		if err := rows.Scan(&id, &level, &target, &typ); err != nil {
			return 0, err
		}
		key := localCellKey(level, target, typ)
		counts[key]++
		if counts[key] > ieltsTargetedQuota {
			toDeactivate = append(toDeactivate, id)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()

	var total int64
	for start := 0; start < len(toDeactivate); start += 500 {
		end := min(start+500, len(toDeactivate))
		placeholders := strings.TrimSuffix(strings.Repeat("?,", end-start), ",")
		args := make([]any, 0, end-start)
		for _, id := range toDeactivate[start:end] {
			args = append(args, id)
		}
		result, err := db.ExecContext(ctx, `UPDATE question_bank SET active=FALSE WHERE id IN (`+placeholders+`)`, args...)
		if err != nil {
			return total, err
		}
		affected, _ := result.RowsAffected()
		total += affected
	}
	return total, nil
}

// normalizedQuestionPattern returns the normalized pattern for a question,
// using context for reading/listening and prompt for other types.
func normalizedQuestionPattern(q question) string {
	src := strings.TrimSpace(q.Context)
	if src == "" {
		src = strings.TrimSpace(visiblePromptKey(q.Prompt))
	}
	return normalizeQuestionContext(src)
}

// ensureLocalQuestionMatrixWithDBCheck fills every (level × target × type) cell
// up to quota unique questions. Before inserting a batch it loads existing
// normalized patterns from the DB and discards any candidate whose pattern
// already exists, guaranteeing zero duplicate normalized patterns across all
// active questions regardless of source.
func ensureLocalQuestionMatrixWithDBCheck(ctx context.Context, db *sql.DB, levels []string, targets []float64, types []string, quota int) error {
	const maxSearchRange = 100000

	existingByCell, err := localQuestionCounts(ctx, db)
	if err != nil {
		return err
	}

	// Pre-load all patterns across all active questions in one single query
	rows, err := db.QueryContext(ctx, `SELECT level, ielts_target, type,
		COALESCE(
			NULLIF(TRIM(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context'))), ''),
			TRIM(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.prompt')))
		) AS raw_text
		FROM question_bank
		WHERE active=TRUE`)
	if err != nil {
		return fmt.Errorf("load active patterns: %w", err)
	}
	patternsByCell := make(map[string]map[string]struct{}, len(levels)*len(targets)*len(types))
	for rows.Next() {
		var (
			level  string
			target float64
			typ    string
			raw    sql.NullString
		)
		if err := rows.Scan(&level, &target, &typ, &raw); err != nil {
			rows.Close()
			return err
		}
		if raw.Valid && raw.String != "" {
			pat := normalizeQuestionContext(raw.String)
			if pat != "" {
				cellKey := localCellKey(level, target, typ)
				if patternsByCell[cellKey] == nil {
					patternsByCell[cellKey] = make(map[string]struct{}, quota)
				}
				patternsByCell[cellKey][pat] = struct{}{}
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	totalCells := len(levels) * len(targets) * len(types)
	cellIndex := 0

	for _, level := range levels {
		for _, target := range targets {
			for _, typ := range types {
				cellIndex++
				cellKey := localCellKey(level, target, typ)
				existing := existingByCell[cellKey]
				if existing >= quota {
					continue
				}

				sessionPatterns := patternsByCell[cellKey]
				if sessionPatterns == nil {
					sessionPatterns = make(map[string]struct{}, quota)
					patternsByCell[cellKey] = sessionPatterns
				}

				needed := quota - existing
				accepted := make([]question, 0, needed)

				searchStart := existing
				for len(accepted) < needed && searchStart < maxSearchRange {
					batchToFetch := min(needed-len(accepted), 500)
					candidates := localQuestionsRange(level, target, typ, searchStart, batchToFetch)
					searchStart += batchToFetch
					if len(candidates) == 0 {
						break
					}
					for _, q := range candidates {
						pat := normalizedQuestionPattern(q)
						if pat == "" {
							continue
						}
						if _, dup := sessionPatterns[pat]; dup {
							continue
						}
						sessionPatterns[pat] = struct{}{}
						accepted = append(accepted, q)
						if len(accepted) >= needed {
							break
						}
					}
				}

				if len(accepted) == 0 {
					continue
				}

				if err := validateQuestions(accepted, len(accepted), []string{typ}); err != nil {
					return fmt.Errorf("validate %s %.1f %s: %w", level, target, typ, err)
				}
				if _, err := insertBankQuestions(ctx, db, level, target, localQuestionSource, accepted); err != nil {
					return fmt.Errorf("seed %s %.1f %s: %w", level, target, typ, err)
				}
				existingByCell[cellKey] += len(accepted)
				if cellIndex%18 == 0 || existingByCell[cellKey] >= quota {
					log.Printf("Seeding progress: [%d/%d cells] %s %.1f %s (count: %d/%d)", cellIndex, totalCells, level, target, typ, existingByCell[cellKey], quota)
				}
			}
		}
	}

	// Clean any context duplicates once after all cells are inserted
	if _, err := deactivateNormalizedContextDuplicates(ctx, db); err != nil {
		return fmt.Errorf("clean normalized contexts: %w", err)
	}

	// Final count verification.
	counts, err := localQuestionCounts(ctx, db)
	if err != nil {
		return err
	}
	for _, level := range levels {
		for _, target := range targets {
			for _, typ := range types {
				count := counts[localCellKey(level, target, typ)]
				if count < quota {
					return fmt.Errorf("local bank %s %.1f %s only has %d active questions; expected at least %d", level, target, typ, count, quota)
				}
			}
		}
	}
	return nil
}

// ensureLocalQuestionMatrix is preserved for backward compatibility (used by
// the non-IELTS seeding path that does not require the full DB pattern check).
func ensureLocalQuestionMatrix(ctx context.Context, db *sql.DB, levels []string, targets []float64, types []string, quota int) error {
	for attempt := 0; attempt < 12; attempt++ {
		existingByCell, err := localQuestionCounts(ctx, db)
		if err != nil {
			return err
		}
		complete := true
		for _, level := range levels {
			for _, target := range targets {
				for _, typ := range types {
					existing := existingByCell[localCellKey(level, target, typ)]
					if existing >= quota {
						continue
					}
					complete = false
					questions := localQuestionsRange(level, target, typ, attempt*quota, quota-existing)
					if err := validateQuestions(questions, len(questions), []string{typ}); err != nil {
						return fmt.Errorf("validate %s %.1f %s: %w", level, target, typ, err)
					}
					if _, err := insertBankQuestions(ctx, db, level, target, localQuestionSource, questions); err != nil {
						return fmt.Errorf("seed %s %.1f %s: %w", level, target, typ, err)
					}
				}
			}
		}
		if complete {
			break
		}
		if _, err := deactivateNormalizedContextDuplicates(ctx, db); err != nil {
			return fmt.Errorf("clean normalized contexts after refill: %w", err)
		}
	}
	counts, err := localQuestionCounts(ctx, db)
	if err != nil {
		return err
	}
	for _, level := range levels {
		for _, target := range targets {
			for _, typ := range types {
				count := counts[localCellKey(level, target, typ)]
				if count < quota {
					return fmt.Errorf("local bank %s %.1f %s only has %d active questions; expected at least %d", level, target, typ, count, quota)
				}
			}
		}
	}
	return nil
}

func verifyIELTSTargetedQuestionBank(ctx context.Context, db *sql.DB) error {
	levelArgs := make([]any, len(ieltsTargetedLevels))
	for index, level := range ieltsTargetedLevels {
		levelArgs[index] = level
	}
	targetArgs := make([]any, len(ieltsTargetedTargets))
	for index, target := range ieltsTargetedTargets {
		targetArgs[index] = target
	}
	typeArgs := make([]any, len(ieltsTargetedTypes))
	for index, typ := range ieltsTargetedTypes {
		typeArgs[index] = typ
	}
	args := append(append(append([]any{localQuestionSource}, levelArgs...), targetArgs...), typeArgs...)
	query := `SELECT COUNT(*), COALESCE(SUM(cell_count),0), COALESCE(MIN(cell_count),0), COALESCE(MAX(cell_count),0)
		FROM (
			SELECT COUNT(*) AS cell_count
			FROM question_bank
			WHERE active=TRUE AND source=?
			  AND level IN (` + sqlPlaceholders(len(levelArgs)) + `)
			  AND ielts_target IN (` + sqlPlaceholders(len(targetArgs)) + `)
			  AND type IN (` + sqlPlaceholders(len(typeArgs)) + `)
			GROUP BY level, ielts_target, type
		) AS cells`
	var cells, total, minimum, maximum int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&cells, &total, &minimum, &maximum); err != nil {
		return err
	}
	expectedCells := len(ieltsTargetedLevels) * len(ieltsTargetedTargets) * len(ieltsTargetedTypes)
	expectedTotal := expectedCells * ieltsTargetedQuota
	if cells != expectedCells || total != expectedTotal || minimum != ieltsTargetedQuota || maximum != ieltsTargetedQuota {
		return fmt.Errorf("invalid IELTS matrix: cells=%d/%d total=%d/%d min=%d max=%d", cells, expectedCells, total, expectedTotal, minimum, maximum)
	}

	missingQuery := `SELECT COUNT(*) FROM question_bank
		WHERE active=TRUE AND source=?
		  AND level IN (` + sqlPlaceholders(len(levelArgs)) + `)
		  AND ielts_target IN (` + sqlPlaceholders(len(targetArgs)) + `)
		  AND type IN (` + sqlPlaceholders(len(typeArgs)) + `)
		  AND (TRIM(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(question_json,'$.explanation')),''))=''
		    OR JSON_EXTRACT(question_json,'$.correctIndex') IS NULL)`
	var missing int
	if err := db.QueryRowContext(ctx, missingQuery, args...).Scan(&missing); err != nil {
		return err
	}
	if missing != 0 {
		return fmt.Errorf("IELTS matrix contains %d questions without a correct answer or explanation", missing)
	}
	duplicates, err := normalizedContextDuplicateGroups(ctx, db)
	if err != nil {
		return err
	}
	if duplicates != 0 {
		return fmt.Errorf("database contains %d duplicate normalized reading/listening context groups", duplicates)
	}
	return nil
}

func sqlPlaceholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

