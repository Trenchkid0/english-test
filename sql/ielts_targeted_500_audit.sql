-- Expected matrix: B1-C2 x IELTS 5.0-9.0 x 6 types x 500 = 108,000.
SELECT level, ielts_target, type, COUNT(*) AS total
FROM english_practice.question_bank
WHERE active = TRUE
  AND source = 'codex_local'
  AND level IN ('B1', 'B2', 'C1', 'C2')
  AND ielts_target IN (5, 5.5, 6, 6.5, 7, 7.5, 8, 8.5, 9)
  AND type IN ('grammar', 'vocabulary', 'reading', 'fill_blank', 'listening', 'error_identification')
GROUP BY level, ielts_target, type
HAVING COUNT(*) <> 500
ORDER BY level, ielts_target, type;

-- Context exists separately only for reading/listening. This query must return 0 rows.
WITH normalized_questions AS (
    SELECT
        id,
        level,
        ielts_target,
        type,
        source,
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

-- Must return 0: every question has a valid answer index and explanation.
SELECT id, level, ielts_target, type
FROM english_practice.question_bank
WHERE active = TRUE
  AND source = 'codex_local'
  AND level IN ('B1', 'B2', 'C1', 'C2')
  AND ielts_target >= 5
  AND (
    JSON_EXTRACT(question_json, '$.correctIndex') IS NULL
    OR TRIM(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.explanation')), '')) = ''
  );

-- Inspect the latest hand-curated records separately from generated codex_local rows.
SELECT id, level, type, created_at, question_json
FROM english_practice.question_bank
WHERE source = 'authentic_curated'
ORDER BY created_at DESC, id DESC
LIMIT 10;

SELECT id, name, email, role
FROM english_practice.users
WHERE role = 'admin'
ORDER BY id
LIMIT 1;
