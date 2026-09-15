-- Index untuk query pemilihan soal dari question_bank.
CREATE INDEX idx_question_bank_lookup
    ON question_bank (active, level, ielts_target, type);

-- Cari data question_bank dengan prompt yang duplikat pada
-- level, target IELTS, dan tipe soal yang sama.
SELECT
    level,
    ielts_target,
    type,
    JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.prompt')) AS prompt,
    COUNT(*) AS duplicate_count,
    GROUP_CONCAT(id ORDER BY id) AS duplicate_ids
FROM question_bank
GROUP BY
    level,
    ielts_target,
    type,
    JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.prompt'))
HAVING COUNT(*) > 1
ORDER BY duplicate_count DESC;

-- Cek soal lama yang masih mengandung penomoran artifisial seperti (Case #217, 2017) atau tanda pagar (#)
SELECT id, level, ielts_target, type, JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.prompt')) AS prompt
FROM question_bank
WHERE question_json LIKE '%#%' OR question_json LIKE '%(Case %'
LIMIT 50;

-- Hapus soal lama yang mengandung tanda pagar (#) atau (Case ...) agar digantikan dengan bank soal alami yang baru
DELETE FROM question_bank
WHERE question_json LIKE '%#%' OR question_json LIKE '%(Case %';

-- Hapus kartu review lama yang juga mengandung tanda pagar (#) atau (Case ...)
DELETE FROM review_cards
WHERE question_json LIKE '%#%' OR question_json LIKE '%(Case %';

