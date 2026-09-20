-- AI-original target: B1-C2 x IELTS 5.0-9.0 x 6 types x 1,000 = 216,000.
-- Must return 216 cells, total 216,000, and min/max 1,000.
SELECT
    COUNT(*) AS cells,
    SUM(cell_count) AS total_questions,
    MIN(cell_count) AS minimum_per_cell,
    MAX(cell_count) AS maximum_per_cell
FROM (
    SELECT level, ielts_target, type, COUNT(*) AS cell_count
    FROM english_practice.question_bank
    WHERE active = TRUE
      AND source = 'authentic_curated'
      AND JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.id')) LIKE 'ai-original-%'
      AND level IN ('B1', 'B2', 'C1', 'C2')
      AND ielts_target IN (5, 5.5, 6, 6.5, 7, 7.5, 8, 8.5, 9)
      AND type IN ('grammar', 'vocabulary', 'reading', 'fill_blank', 'listening', 'error_identification')
    GROUP BY level, ielts_target, type
) AS cells;

-- Must return 0 rows: every matrix cell has exactly 1,000 active questions.
SELECT level, ielts_target, type, COUNT(*) AS total
FROM english_practice.question_bank
WHERE active = TRUE
  AND source = 'authentic_curated'
  AND JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.id')) LIKE 'ai-original-%'
GROUP BY level, ielts_target, type
HAVING COUNT(*) <> 1000
ORDER BY level, ielts_target, type;

-- Context deduplication applies only to reading/listening. Must return 0 rows.
WITH normalized_questions AS (
    SELECT
        id,
        level,
        ielts_target,
        type,
        JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context')) AS original_context,
        TRIM(
            REGEXP_REPLACE(
                REGEXP_REPLACE(
                    LOWER(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context'))),
                    '[0-9]+([:.][0-9]+)?',
                    '{number}'
                ),
                '[[:space:]]+',
                ' '
            )
        ) AS normalized_pattern
    FROM english_practice.question_bank
    WHERE active = TRUE
      AND type IN ('reading', 'listening')
      AND JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context')) IS NOT NULL
      AND TRIM(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context'))) <> ''
)
SELECT
    level,
    ielts_target,
    type,
    normalized_pattern,
    COUNT(*) AS total_same_pattern,
    GROUP_CONCAT(id ORDER BY id) AS question_ids,
    MIN(original_context) AS sample_context
FROM normalized_questions
GROUP BY level, ielts_target, type, normalized_pattern
HAVING COUNT(*) > 1
ORDER BY total_same_pattern DESC, level, ielts_target, type;

-- Must return 0: answer/explanation is missing or explanation is too weak.
SELECT id, level, ielts_target, type
FROM english_practice.question_bank
WHERE active = TRUE
  AND source = 'authentic_curated'
  AND JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.id')) LIKE 'ai-original-%'
  AND (
      JSON_EXTRACT(question_json, '$.correctIndex') IS NULL
      OR TRIM(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.explanation')), '')) = ''
      OR LOWER(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.explanation'))) NOT LIKE '%karena%'
  );

SELECT id, level, ielts_target, type, created_at, question_json
FROM english_practice.question_bank
WHERE source = 'authentic_curated'
  AND JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.id')) LIKE 'ai-original-%'
ORDER BY created_at DESC, id DESC
LIMIT 10;

SELECT id, name, email, role
FROM english_practice.users
WHERE role = 'admin'
ORDER BY id
LIMIT 1;
