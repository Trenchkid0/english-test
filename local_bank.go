package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const localQuestionSource = "codex_local"
const authenticQuestionSource = "authentic_curated"

var localLevels = []string{"A1", "A2", "B1", "B2", "C1", "C2"}
var localTargets = []float64{4, 4.5, 5, 5.5, 6, 6.5, 7, 7.5, 8, 8.5, 9}
var localTypes = []string{"grammar", "vocabulary", "reading", "fill_blank", "listening", "error_identification"}

var contextNumberPattern = regexp.MustCompile(`[0-9]+([:.][0-9]+)?`)
var contextWhitespacePattern = regexp.MustCompile(`[[:space:]]+`)

func normalizeQuestionContext(value string) string {
	value = strings.ToLower(value)
	value = contextNumberPattern.ReplaceAllString(value, "{number}")
	value = contextWhitespacePattern.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func normalizedContextKey(level string, target float64, typ, pattern string) string {
	return fmt.Sprintf("%s|%.1f|%s|%s", level, target, typ, strings.TrimSpace(pattern))
}

func seedCodexQuestionBank(ctx context.Context, db *sql.DB) error {
	// Clean any old questions containing artificial case or number markers
	_, _ = db.ExecContext(ctx, `DELETE FROM question_bank WHERE question_json LIKE '%#%' OR question_json LIKE '%(Case %'`)
	_, _ = db.ExecContext(ctx, `DELETE FROM review_cards WHERE question_json LIKE '%#%' OR question_json LIKE '%(Case %'`)

	if os.Getenv("RESEED_LOCAL_BANK") == "1" {
		for {
			res, err := db.ExecContext(ctx, `DELETE FROM question_bank WHERE source=? LIMIT 10000`, localQuestionSource)
			if err != nil {
				return fmt.Errorf("clear previous codex_local questions: %w", err)
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				break
			}
		}
	}

	// Seed high-quality authentic curated questions (non-template)
	if err := seedAuthenticCuratedQuestions(ctx, db); err != nil {
		return fmt.Errorf("seed authentic questions: %w", err)
	}
	if _, err := deactivateNormalizedContextDuplicates(ctx, db); err != nil {
		return fmt.Errorf("deactivate duplicate reading/listening contexts: %w", err)
	}
	if err := ensureLocalQuestionMinimum(ctx, db, 500); err != nil {
		return err
	}
	duplicates, err := normalizedContextDuplicateGroups(ctx, db)
	if err != nil {
		return fmt.Errorf("validate duplicate reading/listening contexts: %w", err)
	}
	if duplicates != 0 {
		return fmt.Errorf("question bank still contains %d duplicate normalized reading/listening context groups", duplicates)
	}
	return nil
}

func ensureLocalQuestionMinimum(ctx context.Context, db *sql.DB, minimum int) error {
	for attempt := 0; attempt < 12; attempt++ {
		existingByCell, err := localQuestionCounts(ctx, db)
		if err != nil {
			return err
		}
		complete := true
		for _, level := range localLevels {
			for _, target := range localTargets {
				for _, typ := range localTypes {
					existing := existingByCell[localCellKey(level, target, typ)]
					if existing >= minimum {
						continue
					}
					complete = false
					needed := minimum - existing
					items := localQuestionsRange(level, target, typ, attempt*500, needed)
					if _, err := insertBankQuestions(ctx, db, level, target, localQuestionSource, items); err != nil {
						return fmt.Errorf("local bank seed %s %.1f %s: %w", level, target, typ, err)
					}
				}
			}
		}
		if complete {
			return nil
		}
		if _, err := deactivateNormalizedContextDuplicates(ctx, db); err != nil {
			return fmt.Errorf("deactivate duplicate contexts after refill: %w", err)
		}
	}

	counts, err := localQuestionCounts(ctx, db)
	if err != nil {
		return err
	}
	for _, level := range localLevels {
		for _, target := range localTargets {
			for _, typ := range localTypes {
				count := counts[localCellKey(level, target, typ)]
				if count < minimum {
					return fmt.Errorf("local bank %s %.1f %s only has %d active questions; expected at least %d", level, target, typ, count, minimum)
				}
			}
		}
	}
	return nil
}

func localQuestionCounts(ctx context.Context, db *sql.DB) (map[string]int, error) {
	existingByCell := make(map[string]int, len(localLevels)*len(localTargets)*len(localTypes))
	rows, err := db.QueryContext(ctx, `SELECT level,ielts_target,type,COUNT(*) FROM question_bank WHERE active=TRUE AND source=? GROUP BY level,ielts_target,type`, localQuestionSource)
	if err != nil {
		return nil, fmt.Errorf("local bank counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var level, typ string
		var target float64
		var count int
		if err := rows.Scan(&level, &target, &typ, &count); err != nil {
			return nil, fmt.Errorf("local bank counts scan: %w", err)
		}
		existingByCell[localCellKey(level, target, typ)] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("local bank counts rows: %w", err)
	}
	return existingByCell, nil
}

// deactivateNormalizedContextDuplicates applies the same normalization used by
// the SQL audit supplied for the project. It retains curated material first,
// then the oldest local record, and makes the remaining duplicates inactive.
func deactivateNormalizedContextDuplicates(ctx context.Context, db *sql.DB) (int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM (
		SELECT id, ROW_NUMBER() OVER (
			PARTITION BY level, ielts_target, type,
				TRIM(REGEXP_REPLACE(REGEXP_REPLACE(LOWER(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context'))), '[0-9]+([:.][0-9]+)?', '{number}'), '[[:space:]]+', ' '))
			ORDER BY CASE source WHEN 'authentic_curated' THEN 0 WHEN 'codex_local' THEN 1 ELSE 2 END, id
		) AS duplicate_rank
		FROM question_bank
		WHERE active=TRUE
		  AND type IN ('reading', 'listening')
		  AND JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context')) IS NOT NULL
		  AND TRIM(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context'))) <> ''
	) AS ranked WHERE duplicate_rank > 1`)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	var total int64
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		placeholders := strings.TrimSuffix(strings.Repeat("?,", end-start), ",")
		args := make([]any, 0, end-start)
		for _, id := range ids[start:end] {
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

func normalizedContextDuplicateGroups(ctx context.Context, db *sql.DB) (int, error) {
	const audit = `WITH normalized_questions AS (
		SELECT level, ielts_target, type,
			TRIM(REGEXP_REPLACE(REGEXP_REPLACE(LOWER(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context'))), '[0-9]+([:.][0-9]+)?', '{number}'), '[[:space:]]+', ' ')) AS normalized_pattern
		FROM question_bank
		WHERE active=TRUE
		  AND type IN ('reading', 'listening')
		  AND JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context')) IS NOT NULL
		  AND TRIM(JSON_UNQUOTE(JSON_EXTRACT(question_json, '$.context'))) <> ''
	)
	SELECT COUNT(*) FROM (
		SELECT 1 FROM normalized_questions
		GROUP BY level, ielts_target, type, normalized_pattern
		HAVING COUNT(*) > 1
	) AS duplicate_groups`
	var count int
	err := db.QueryRowContext(ctx, audit).Scan(&count)
	return count, err
}

func seedAuthenticCuratedQuestions(ctx context.Context, db *sql.DB) error {
	dataDir := os.Getenv("SEED_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join("data", "questions")
	}
	allCurated, err := loadCuratedQuestions(dataDir)
	if err != nil {
		return err
	}

	byCell := make(map[string][]question)
	for i, q := range allCurated {
		key := localCellKey(q.Level, q.IELTSTarget, q.Type)
		item := curatedRecordQuestion(q, fmt.Sprintf("auth-%s-%s-%.1f-%03d", q.Type, q.Level, q.IELTSTarget, i+1))
		byCell[key] = append(byCell[key], item)
	}

	for key, items := range byCell {
		parts := strings.Split(key, "|")
		if len(parts) != 3 {
			continue
		}
		level := parts[0]
		var target float64
		_, _ = fmt.Sscanf(parts[1], "%f", &target)

		_, err := insertBankQuestions(ctx, db, level, target, authenticQuestionSource, items)
		if err != nil {
			return fmt.Errorf("seed authentic questions %s: %w", key, err)
		}
	}
	return nil
}

func localCellKey(level string, target float64, typ string) string {
	return fmt.Sprintf("%s|%.1f|%s", level, target, typ)
}

func localQuestionBankSummary(ctx context.Context, db *sql.DB) (cells, total, minimum, maximum int, err error) {
	err = db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(cell_count),0),COALESCE(MIN(cell_count),0),COALESCE(MAX(cell_count),0)
		FROM (SELECT COUNT(*) AS cell_count FROM question_bank WHERE active=TRUE AND source=? GROUP BY level,ielts_target,type) AS cells`, localQuestionSource).
		Scan(&cells, &total, &minimum, &maximum)
	return
}

func permuteChoices(choices []string, correctIdx int, seed int) ([]string, int) {
	if len(choices) != 4 || correctIdx < 0 || correctIdx >= 4 {
		return choices, correctIdx
	}
	targetPos := ((seed % 4) + 4) % 4
	if targetPos == correctIdx {
		return choices, correctIdx
	}
	newChoices := make([]string, 4)
	copy(newChoices, choices)
	newChoices[correctIdx], newChoices[targetPos] = newChoices[targetPos], newChoices[correctIdx]
	return newChoices, targetPos
}

func localQuestions(level string, target float64, typ string) []question {
	return localQuestionsRange(level, target, typ, 0, 500)
}

func localQuestionsRange(level string, target float64, typ string, start, count int) []question {
	items := make([]question, 0, count)
	for i := start; i < start+count; i++ {
		var q question
		switch typ {
		case "grammar":
			q = localGrammarQuestion(level, target, i)
		case "vocabulary":
			q = localVocabularyQuestion(level, target, i)
		case "reading":
			q = localReadingQuestion(level, target, i)
		case "fill_blank":
			q = localFillBlankQuestion(level, target, i)
		case "listening":
			q = localListeningQuestion(level, target, i)
		case "error_identification":
			q = localErrorIdentificationQuestion(level, target, i)
		}
		q.ID = fmt.Sprintf("local-%s-%s-%02.1f-%03d", typ, level, target, i+1)
		q.Prompt += fmt.Sprintf(" [Level %s · Band %.1f · Practice %03d]", level, target, i+1)
		q.Explanation = learnerFriendlyExplanation(q)
		items = append(items, q)
	}
	return items
}

func learnerFriendlyExplanation(q question) string {
	if q.CorrectIndex < 0 || q.CorrectIndex >= len(q.Choices) {
		return strings.TrimSpace(q.Explanation)
	}
	answer := strings.TrimSpace(q.Choices[q.CorrectIndex])
	reason := strings.TrimSpace(q.Explanation)
	reason = strings.TrimSuffix(reason, ".")
	if q.Type == "reading" && strings.TrimSpace(q.Evidence) != "" {
		return fmt.Sprintf("Jawaban yang tepat adalah “%s”. Bukti kunci pada teks adalah “%s”. Alasannya: %s.", answer, strings.TrimSpace(q.Evidence), reason)
	}
	return fmt.Sprintf("Jawaban yang tepat adalah “%s”. Alasannya: %s.", answer, reason)
}

var localEntities = []string{
	"the library team", "the research group", "the city council", "the student committee", "the health centre",
	"the transport office", "the museum staff", "the science club", "the housing association", "the language school",
	"the conservation team", "the university press", "the community garden", "the careers service", "the design studio",
	"the volunteer network", "the engineering faculty", "the public clinic", "the tourism board", "the neighbourhood forum",
	"the climate laboratory", "the training department", "the local archive", "the education charity", "the coastal authority",
	"the economics faculty", "the biotechnology unit", "the urban planning bureau", "the wildlife foundation", "the renewable energy council",
	"the sociology department", "the maritime institute", "the heritage society", "the robotics laboratory", "the public health syndicate",
	"the environmental agency", "the agricultural institute", "the linguistics institute", "the astrophysics council", "the medical registry",
	"the architecture studio", "the archaeological trust", "the ornithology station", "the oceanographic institute", "the renewable technology centre",
	"the behavioural science unit", "the public policy forum", "the botanical observatory", "the veterinary faculty", "the international affairs bureau",
}

var localTopics = []string{
	"digital access", "public transport", "urban gardens", "student wellbeing", "renewable energy",
	"language learning", "waste reduction", "community health", "local history", "remote work",
	"water conservation", "museum access", "food security", "adult education", "road safety",
	"coastal protection", "library services", "air quality", "affordable housing", "wildlife monitoring",
	"cognitive development", "marine biodiversity", "sustainable agriculture", "data privacy", "solar microgrids",
	"urban heat islands", "archival preservation", "vocational training", "groundwater management", "pedestrian safety",
	"microplastics pollution", "computational linguistics", "circular economy", "wetland restoration", "telemedicine adoption",
	"carbon sequestration", "smart grid resilience", "bioacoustic monitoring", "sustainable forestry", "aquifer recharge",
	"bilingual cognitive flexibility", "geothermal district heating", "riverine macroinvertebrate health", "historic masonry conservation", "urban pollinator corridors",
	"autonomous public transit", "industrial polymer recycling", "tidal energy dissipation", "pediatric nutritional ergonomics", "deep-space spectroscopic imaging",
}

var grammarLeads = []string{
	"In the recent evaluation",
	"During the pilot phase",
	"In the annual review",
	"Throughout the regional assessment",
	"According to the baseline audit",
	"In the latest monitoring cycle",
	"During the municipal appraisal",
	"In the preliminary study",
	"Across the follow-up trials",
	"In the independent investigation",
	"During the scheduled review",
	"In the comprehensive assessment",
	"Following the recent reform",
	"In the long-term observation",
	"During the collaborative initiative",
	"In the updated report",
	"Under the regional framework",
	"In the institutional project",
	"During the comparative analysis",
	"In the systematic survey",
	"Across recent field trials",
	"In the national assessment",
	"During the second phase",
	"In the standard review",
	"Throughout the multi-year study",
}

func localGrammarQuestion(level string, target float64, i int) question {
	entity := localEntities[(i*3)%len(localEntities)]
	topic := localTopics[(i*5)%len(localTopics)]
	lead := grammarLeads[(i/40)%len(grammarLeads)]
	year := 2012 + (i % 12)

	templates := []struct {
		prompt      string
		choices     []string
		correct     int
		explanation string
		tip         string
	}{
		{fmt.Sprintf("%s of %s, %s regularly _____ empirical field data to verify baseline trends.", lead, topic, entity), []string{"collects", "is collecting", "collected", "collect"}, 0, "Kata kerja 'collects' tepat karena subjek 'the team' tunggal dan kalimat menyatakan rutinitas ('regularly').", "Perhatikan adverb of frequency seperti 'regularly' atau 'annually' yang menandai kebiasaan rutin."},
		{fmt.Sprintf("%s, regarding %s, %s _____ new emission reduction standards.", lead, topic, entity), []string{"implements", "is implementing", "implemented", "has implement"}, 1, "Bentuk 'is implementing' tepat karena aksi sedang berlangsung saat ini ('at present').", "Penanda waktu seperti 'at present' atau 'currently' menunjukkan aksi yang sedang berjalan."},
		{fmt.Sprintf("%s, %s _____ significant progress in %s over the past decade.", lead, entity, topic), []string{"made", "has made", "makes", "is making"}, 1, "Bentuk 'has made' tepat karena rentang waktu 'over the past decade' menyatakan perkembangan yang dampaknya berlanjut hingga sekarang.", "Rentang waktu seperti 'over the past decade' atau 'since' selalu berpasangan dengan Present Perfect."},
		{fmt.Sprintf("%s in %d, %s _____ its first comprehensive study on %s.", lead, year, entity, topic), []string{"publishes", "published", "is publishing", "has published"}, 1, "Bentuk past simple 'published' digunakan untuk peristiwa yang sudah selesai tuntas pada tahun lampau tertentu.", "Keterangan tahun lampau spesifik seperti 'In 2018' wajib menggunakan bentuk past tense (V2)."},
		{fmt.Sprintf("%s regarding %s, if %s secures municipal backing, the project _____ next spring.", lead, topic, entity), []string{"will launch", "would launch", "launched", "launches"}, 0, "Klausa utama first conditional membutuhkan 'will + base verb' (will launch) untuk kemungkinan nyata di masa depan.", "Jika klausa 'if' memakai Present Simple, klausa utama memakai 'will + kata kerja dasar'."},
		{fmt.Sprintf("%s of the %s study, if %s had received verified data earlier, it _____ the briefing on time.", lead, topic, entity), []string{"will deliver", "would deliver", "would have delivered", "delivered"}, 2, "Klausa utama third conditional membutuhkan 'would have delivered' untuk pengandaian lampau yang tidak terwujud.", "Cari urutan waktu lampau sebelum memilih bentuk conditional."},
		{fmt.Sprintf("%s regarding %s, the statistical recommendations formulated by %s _____ by external reviewers.", lead, topic, entity), []string{"audited", "were audited", "have auditing", "was audit"}, 1, "Subjek 'recommendations' jamak dan menerima tindakan, sehingga bentuk past passive yang benar adalah 'were audited'.", "Tentukan apakah subjek melakukan atau menerima tindakan."},
		{fmt.Sprintf("%s of the %s audit, neither the principal investigator nor the data analysts at %s _____ available for interview.", lead, topic, entity), []string{"was", "were", "has", "be"}, 1, "Pada pola 'neither... nor', kata kerja mengikuti subjek terdekat ('analysts') sehingga menggunakan 'were'.", "Cocokkan verb dengan subjek terdekat pada paired conjunction."},
		{fmt.Sprintf("%s, by the time the %d fiscal cycle concludes, %s _____ all scheduled field trials on %s.", lead, year+2, entity, topic), []string{"completes", "completed", "will have completed", "has completing"}, 2, "Penanda 'by the time' di masa depan membutuhkan future perfect 'will have completed' untuk aksi yang selesai sebelum batas waktu tersebut.", "Perhatikan penanda batas waktu seperti 'by the time'."},
		{fmt.Sprintf("%s of the %s initiative, %s, which _____ the primary survey, made the dataset publicly accessible.", lead, topic, strings.Title(entity)), []string{"conducted", "conduct", "conducting", "has conduct"}, 0, "Relative clause menerangkan tindakan lampau spesifik sehingga menggunakan bentuk 'conducted'.", "Periksa fungsi klausa penjelas dan keterangan waktunya."},
		{fmt.Sprintf("%s for the %s symposium, all project delegates participating under %s _____ register their credentials before Friday.", lead, topic, entity), []string{"must", "would", "used", "ought"}, 0, "Modal 'must' diikuti bare infinitive tanpa 'to' untuk menyatakan kewajiban formal.", "Bedakan modal untuk kewajiban mutlak (must) dan saran (ought to)."},
		{fmt.Sprintf("%s concerning %s, %s produced _____ exceptionally detailed empirical report.", lead, topic, strings.Title(entity)), []string{"a", "an", "the", "no article"}, 1, "Artikel 'an' digunakan karena kata berikutnya ('exceptionally') diawali bunyi vokal.", "Pilih article berdasarkan bunyi awal kata berikutnya."},
		{fmt.Sprintf("%s, long-term progress in %s heavily depends _____ rigorous baseline measurements established by %s.", lead, topic, entity), []string{"at", "for", "on", "with"}, 2, "Kata kerja 'depend' secara baku berpasangan dengan preposisi 'on' (depend on).", "Pelajari kata kerja bersama preposisi pasangannya."},
		{fmt.Sprintf("%s regarding %s, the chief coordinator affirmed that %s _____ the conclusive findings the following week.", lead, topic, entity), []string{"will release", "would release", "releases", "has released"}, 1, "Reporting verb lampau ('affirmed') membuat kata kerja kalimat tidak langsung bergeser menjadi 'would release'.", "Perhatikan pergeseran tenses (backshift) dalam kalimat tidak langsung."},
		{fmt.Sprintf("%s in the %s project, the protocol proposed by %s proved _____ than the exploratory trial.", lead, topic, entity), []string{"more effective", "most effective", "effective", "effectively"}, 0, "Kata pembanding 'than' membutuhkan bentuk comparative 'more effective'.", "Cari kata penanda pembanding seperti than sebelum memilih adjective."},
		{fmt.Sprintf("%s of the %s review, not only _____ the primary records, but %s also digitized field archives.", lead, topic, entity), []string{"they examined", "did they examine", "have they examine", "they were examining"}, 1, "Frasa negatif 'Not only' di awal kalimat memicu pembalikan posisi auxiliary sebelum subjek ('did they examine').", "Setelah 'Not only' di awal kalimat, susun klausa seperti kalimat tanya."},
		{fmt.Sprintf("%s of the %s analysis, it was the unexpected finding _____ prompted %s to revise its operational timeline.", lead, topic, entity), []string{"that", "which", "whom", "where"}, 0, "Pola cleft sentence penekanan adalah 'It was [fokus] that prompted...'.", "Kenali pola penekanan (It was X that Y)."},
		{fmt.Sprintf("%s concerning %s, the supervisory board recommended that %s _____ its testing protocols without delay.", lead, topic, entity), []string{"standardizes", "standardize", "standardized", "has standardized"}, 1, "Setelah 'recommended that', kata kerja klausa berikutnya wajib berupa bentuk dasar murni ('standardize').", "Setelah insist, recommend, demand + that, gunakan bentuk kata kerja dasar."},
		{fmt.Sprintf("%s for the %s summit, having _____ the initial pilot phase, %s presented its framework.", lead, topic, entity), []string{"complete", "completing", "completed", "to complete"}, 2, "Bentuk perfect participle 'Having completed' menyatakan tindakan yang telah tuntas sebelum kalimat utama.", "Gunakan having + V3 untuk menyatakan urutan tindakan yang mendahului kalimat utama."},
		{fmt.Sprintf("%s in the %s expansion, had %s secured supplementary funding, the project _____ substantially further.", lead, topic, entity), []string{"progressed", "would progress", "would have progressed", "had progressed"}, 2, "Bentuk inversi 'Had %s secured...' setara dengan third conditional dan berpasangan dengan 'would have progressed'.", "Pola 'Had + subjek + V3' bermakna setara dengan 'If + subjek + had V3'."},
		{fmt.Sprintf("%s regarding %s, the methodology developed by %s is widely considered _____ remarkably robust.", lead, topic, entity), []string{"to be", "being", "be", "been"}, 0, "Pola pasif 'is considered' secara formal dilengkapi to-infinitive ('to be').", "Perhatikan pola 'be said/thought/considered + to-infinitive'."},
		{fmt.Sprintf("%s on %s, given the audit checks, %s _____ been unaware of the data discrepancy.", lead, topic, entity), []string{"cannot have", "must have", "should have", "might not"}, 0, "Bentuk 'cannot have been' menyatakan kesimpulan bahwa ketidaktahuan tersebut mustahil terjadi di masa lalu.", "Gunakan 'cannot have + V3' untuk menyimpulkan kemustahilan di masa lalu."},
		{fmt.Sprintf("%s of the %s dataset, the more rigorously %s scrutinized the records, _____ the underlying patterns became.", lead, topic, entity), []string{"the clearer", "clearer", "more clear", "the most clear"}, 0, "Pola double comparative berimbang membutuhkan 'the clearer' untuk melengkapi 'the more rigorously'.", "Pastikan kedua sisi klausa menggunakan 'The + comparative'."},
		{fmt.Sprintf("%s concerning %s, the analytical system _____ %s evaluated performance has since been adopted internationally.", lead, topic, entity), []string{"which", "by which", "whom", "whereby of"}, 1, "Frasa 'by which' adalah bentuk formal untuk menerangkan sarana atau metode evaluasi.", "Gunakan preposisi yang tepat sebelum pronoun 'which'."},
		{fmt.Sprintf("%s of the %s archive, %s succeeded in _____ the entire database ahead of schedule.", lead, topic, strings.Title(entity)), []string{"modernize", "modernizing", "to modernize", "modernized"}, 1, "Preposisi 'in' wajib diikuti bentuk gerund ('modernizing').", "Semua verb setelah preposition wajib menggunakan akhiran -ing."},
		{fmt.Sprintf("%s during the %s campaign, seldom _____ %s encountered such enthusiastic public engagement.", lead, topic, entity), []string{"has", "have", "is", "was"}, 0, "Adverbial negatif 'Seldom' memicu pembalikan kata bantu sebelum subjek ('has %s').", "Adverbial negatif di awal kalimat memicu pembalikan posisi auxiliary dan subjek."},
		{fmt.Sprintf("%s in the %s rollout, were it not for the strategic support from %s, the expansion _____ completely stalled.", lead, topic, entity), []string{"will have", "would have", "had", "has"}, 1, "Bentuk 'Were it not for...' menyatakan pengandaian kondisi dan berpasangan dengan 'would have'.", "'Were it not for' bermakna 'Jika bukan karena'."},
		{fmt.Sprintf("%s regarding %s, despite _____ unforeseen constraints, %s completed the target milestone.", lead, topic, entity), []string{"face", "facing", "faced", "to face"}, 1, "Preposisi 'despite' wajib diikuti oleh noun atau gerund ('facing'), bukan klausa utuh.", "Jangan letakkan klausa utuh (S+V) langsung setelah despite; gunakan gerund atau noun."},
		{fmt.Sprintf("%s of the %s dispatch, no sooner had %s published the preliminary data _____ inquiries arrived.", lead, topic, entity), []string{"when", "than", "then", "that"}, 1, "Konjungsi 'No sooner had...' selalu berpasangan dengan 'than'.", "Ingat bahwa 'No sooner' selalu berpasangan dengan 'than'."},
		{fmt.Sprintf("%s concerning %s, if %s had implemented the guidelines earlier, local operating costs _____ significantly lower today.", lead, topic, entity), []string{"are", "would be", "will be", "had been"}, 1, "Mixed conditional menghubungkan pengandaian lampau ('had implemented') dengan situasi saat ini ('would be lower today').", "Perhatikan kata 'today' atau 'now' di klausa utama mixed conditional."},
		{fmt.Sprintf("%s, only after %s concluded the %s assessment _____ the final consensus reached.", lead, entity, topic), []string{"was", "were", "is", "has"}, 0, "Setelah 'Only after' di awal kalimat, kata kerja bantu diletakkan sebelum subjek ('was the final consensus reached').", "Setelah 'Only after' di awal kalimat, susun klausa utama seperti kalimat tanya."},
		{fmt.Sprintf("%s, while %s emphasized rapid implementation, the independent board recommended _____ on data security in %s.", lead, entity, topic), []string{"to focus", "focusing", "focused", "focus"}, 1, "Kata kerja 'recommend' tanpa objek orang langsung diikuti gerund ('focusing').", "Kata kerja recommend dapat langsung diikuti gerund jika tanpa objek orang."},
		{fmt.Sprintf("%s, under no circumstances _____ %s authorize the public release of unverified %s findings.", lead, entity, topic), []string{"should", "ought", "must to", "had to"}, 0, "Frasa 'Under no circumstances' memicu pembalikan kata bantu sebelum subjek ('should %s authorize').", "Frasa pembatas negatif di awal kalimat selalu memicu inversi."},
		{fmt.Sprintf("%s in the %s initiative, %s had the regional monitoring equipment _____ by certified technicians.", lead, topic, entity), []string{"inspect", "inspected", "inspecting", "to inspect"}, 1, "Pola causative passive adalah 'have something + V3' (have equipment inspected).", "Pola 'have + object + V3' menyatakan objek dikenai pekerjaan oleh orang lain."},
		{fmt.Sprintf("%s, the policy recommendations _____ by %s at the %d symposium on %s have gained nationwide support.", lead, entity, year, topic), []string{"presented", "presenting", "were presented", "present"}, 0, "Reduced relative clause menyederhanakan 'which were presented' menjadi past participle 'presented'.", "Kenali relative clause pasif yang disederhanakan menjadi bentuk V3."},
		{fmt.Sprintf("%s at the %s conference, %s proposed an innovative method _____ carbon footprint across all operational facilities.", lead, topic, entity), []string{"of reducing", "to reduce", "for reduce", "in reduction"}, 1, "Noun 'method' diikuti to-infinitive ('to reduce') untuk menyatakan tujuan tindakan.", "Noun 'method' atau 'way' lazim diikuti to-infinitive untuk menyatakan tujuan."},
		{fmt.Sprintf("%s, had it not been for the comprehensive intervention by %s, the regional %s network _____ severe disruption.", lead, entity, topic), []string{"would suffer", "would have suffered", "will suffer", "had suffered"}, 1, "Bentuk 'Had it not been for' berpasangan dengan 'would have suffered' pada klausa utama.", "Bentuk 'Had it not been for' setara dengan 'If it had not been for'."},
		{fmt.Sprintf("%s regarding %s, %s is widely considered _____ one of the most reliable institutions in the province.", lead, topic, entity), []string{"to be", "being", "be", "been"}, 0, "Pola pasif 'is considered' secara formal dilengkapi to-infinitive ('to be').", "Pola 'be considered + to be' adalah bentuk formal baku dalam ragam akademik."},
		{fmt.Sprintf("%s, the research data on %s was _____ thoroughly analyzed that %s detected previously overlooked anomalies.", lead, topic, entity), []string{"so", "such", "too", "very"}, 0, "Adverb 'thoroughly' dihubungkan dengan klausa akibat menggunakan pasangan 'so... that'.", "Gunakan 'so + adj/adv + that', dan 'such + noun phrase + that'."},
		{fmt.Sprintf("%s concerning %s, the regional coordinator confirmed that %s _____ the finalized project framework.", lead, topic, entity), []string{"has released", "had released", "releases", "is releasing"}, 1, "Bentuk past perfect 'had released' tepat untuk menyatakan tindakan yang telah selesai sebelum waktu konfirmasi di masa lampau.", "Perhatikan urutan kejadian di masa lampau (past perfect mendahului past simple)."},
	}

	t := templates[i%len(templates)]
	choices, correctIdx := permuteChoices(t.choices, t.correct, i+int(target*10))
	return question{
		Type:         "grammar",
		Prompt:       t.prompt,
		Choices:      choices,
		CorrectIndex: correctIdx,
		Explanation:  t.explanation,
		LearningTip:  t.tip,
		IELTSSkill:   "Grammatical Range and Accuracy",
	}
}

type localWord struct {
	word     string
	meaning  string
	antonym  string
	wrong1   string
	wrong2   string
	colloc   string
	goodColl string
	badColl  string
}

var localWords = []localWord{
	{"substantial", "considerable", "negligible", "temporary", "uncertain", "evidence", "substantial evidence", "substantial velocity"},
	{"mitigate", "make less severe", "exacerbate", "measure precisely", "remove completely", "the risk", "mitigate the risk", "mitigate the altitude"},
	{"feasible", "practical and possible", "impractical", "legally required", "extremely expensive", "alternative", "feasible alternative", "feasible temperature"},
	{"allocate", "distribute for a purpose", "withhold", "hide permanently", "borrow informally", "resources", "allocate resources", "allocate a climate"},
	{"deteriorate", "become worse", "improve", "remain stable", "disappear suddenly", "conditions", "conditions deteriorate", "deteriorate an achievement"},
	{"ambiguous", "open to more than one meaning", "unambiguous", "supported by evidence", "easy to measure", "statement", "ambiguous statement", "ambiguous rainfall"},
	{"coherent", "logically connected", "disjointed", "briefly mentioned", "widely disputed", "argument", "coherent argument", "coherent distance"},
	{"prevalent", "widespread", "scarce", "carefully hidden", "recently invented", "practice", "prevalent practice", "prevalent speed"},
	{"adverse", "harmful or unfavourable", "beneficial", "unexpectedly useful", "financially neutral", "effects", "adverse effects", "adverse literacy"},
	{"incentive", "something that encourages action", "deterrent", "a formal prohibition", "an accidental outcome", "scheme", "financial incentive", "incentive perimeter"},
	{"consecutive", "following one after another", "intermittent", "happening occasionally", "moving in opposite directions", "days", "consecutive days", "consecutive gravity"},
	{"empirical", "based on observation or experiment", "theoretical", "based only on opinion", "written in ancient language", "data", "empirical data", "empirical altitude"},
	{"viable", "capable of working successfully", "unviable", "popular for a short time", "harmful to everyone", "solution", "economically viable", "viable hemisphere"},
	{"negligible", "too small to be important", "significant", "surprisingly expensive", "easy to predict", "impact", "negligible impact", "negligible century"},
	{"scrutinise", "examine closely", "glance over", "summarise briefly", "accept immediately", "the data", "closely scrutinise", "scrutinise the weather"},
	{"robust", "strong and reliable", "fragile", "unnecessarily complex", "newly available", "methodology", "robust methodology", "robust humidity"},
	{"constrain", "limit or restrict", "liberate", "support financially", "describe accurately", "growth", "severely constrain", "constrain an archive"},
	{"offset", "counterbalance", "intensify", "estimate roughly", "postpone indefinitely", "the cost", "offset the emissions", "offset the vocabulary"},
	{"retain", "continue to keep", "discard", "refuse to discuss", "divide equally", "control", "retain staff", "retain an earthquake"},
	{"undermine", "weaken gradually", "strengthen", "confirm completely", "measure regularly", "credibility", "undermine confidence", "undermine the altitude"},
	{"facilitate", "make easier", "hinder", "make compulsory", "make profitable", "learning", "facilitate collaboration", "facilitate an eclipse"},
	{"cumulative", "increasing through additions", "isolated", "limited to one event", "changing without direction", "effect", "cumulative effect", "cumulative latitude"},
	{"disparity", "a noticeable difference", "equality", "a shared objective", "a sudden improvement", "in income", "growing disparity", "disparity of gravity"},
	{"resilient", "able to recover from difficulty", "vulnerable", "unlikely to change", "easy to replace", "economy", "resilient infrastructure", "resilient grammar"},
	{"tentative", "not yet certain or final", "definite", "strongly enforced", "widely celebrated", "schedule", "tentative conclusion", "tentative continent"},
	{"comprehensive", "thorough and all-inclusive", "incomplete", "costly", "temporary", "review", "comprehensive review", "comprehensive velocity"},
	{"delineate", "describe or outline precisely", "obscure", "postpone", "calculate roughly", "the boundaries", "clearly delineate", "delineate a climate"},
	{"fluctuate", "rise and fall irregularly", "stabilize", "decline permanently", "increase steadily", "wildly", "prices fluctuate", "fluctuate a dictionary"},
	{"inadvertent", "unintentional", "deliberate", "carefully calculated", "legally binding", "omission", "inadvertent error", "inadvertent galaxy"},
	{"lucrative", "producing much profit", "unprofitable", "strictly regulated", "unpredictable", "market", "lucrative contract", "lucrative syntax"},
	{"paramount", "more important than anything else", "trivial", "broadly defined", "widely accepted", "importance", "of paramount importance", "paramount friction"},
	{"proliferate", "increase rapidly in numbers", "dwindle", "evolve slowly", "disintegrate", "rapidly", "cells proliferate", "proliferate an essay"},
	{"sporadic", "occurring at irregular intervals", "continuous", "heavily planned", "strictly monitored", "outbreaks", "sporadic episodes", "sporadic preposition"},
	{"tangible", "clear and definite or real", "intangible", "highly theoretical", "unproven", "benefits", "tangible benefits", "tangible syntax"},
	{"unprecedented", "never done or known before", "customary", "moderately successful", "easily replicable", "scale", "unprecedented scale", "unprecedented pronoun"},
	{"vulnerable", "exposed to harm or attack", "invulnerable", "financially stable", "well protected", "population", "highly vulnerable", "vulnerable punctuation"},
	{"warrant", "justify or necessitate", "discredit", "contradict", "estimate", "investigation", "warrant further investigation", "warrant a hemisphere"},
	{"augment", "make greater by adding to it", "diminish", "replace entirely", "categorize", "the workforce", "augment existing data", "augment an equator"},
	{"synthesize", "combine into a coherent whole", "fragment", "critique harshly", "translate literally", "findings", "synthesize information", "synthesize a tide"},
	{"plausible", "seeming reasonable or probable", "implausible", "scientifically proven", "widely condemned", "explanation", "plausible explanation", "plausible archipelago"},
	{"advent", "the arrival or creation of something", "departure", "temporary absence", "minor delay", "of technology", "the advent of digital tools", "advent velocity"},
	{"conducive", "making a certain situation likely", "detrimental", "expensive", "unrelated", "to growth", "conducive to learning", "conducive hemisphere"},
	{"disseminate", "spread information widely", "conceal", "translate poorly", "delete immediately", "findings", "disseminate research findings", "disseminate altitude"},
	{"endow", "provide with quality or asset", "deprive", "criticize publicly", "measure roughly", "with talent", "endowed with natural resources", "endow a preposition"},
	{"foster", "encourage the development of", "suppress", "delay slightly", "ignore completely", "innovation", "foster international innovation", "foster a latitude"},
	{"imperative", "of vital importance or crucial", "optional", "moderately useful", "cheaply produced", "duty", "an urgent moral imperative", "imperative velocity"},
	{"judicious", "having good judgment or sense", "reckless", "loudly spoken", "hastily organized", "use", "judicious use of resources", "judicious rainfall"},
	{"lucid", "expressed clearly and easy to understand", "confusing", "lengthy", "overly simplistic", "explanation", "a lucid and elegant explanation", "lucid gravity"},
	{"meticulous", "showing great attention to detail", "careless", "fast-paced", "widely recognized", "research", "meticulous lab research", "meticulous climate"},
	{"nuance", "a subtle distinction in meaning", "coarseness", "direct translation", "obvious fact", "in tone", "grasp the subtle nuance", "nuance altitude"},
}

var vocabVenues = []string{
	"an academic monograph",
	"a peer-reviewed journal article",
	"a published literature review",
	"an empirical research paper",
	"a scholarly treatise",
	"a university symposium report",
	"a doctoral dissertation",
	"a public policy whitepaper",
	"an institutional review",
	"a scientific journal publication",
	"a published conference paper",
	"a critical literature review",
	"an international study report",
	"a formal policy monograph",
	"a comprehensive meta-analysis",
	"a field investigation report",
	"a scholarly discussion paper",
	"an academic textbook chapter",
	"a published symposium proceeding",
	"a regional research bulletin",
}

func localVocabularyQuestion(level string, target float64, i int) question {
	archetype := i % 10
	cycle := i / 10
	w := localWords[cycle%len(localWords)]
	venue := vocabVenues[(cycle*7+i)%len(vocabVenues)]
	topic := localTopics[(cycle*11+i*3)%len(localTopics)]

	var prompt string
	var choices []string
	var correct int
	var explanation string
	var tip string

	switch archetype {
	case 0:
		prompt = fmt.Sprintf("In %s on %s, the author characterizes the evidence as ‘%s’. What is the most precise synonym of ‘%s’ in this context?", venue, topic, w.word, w.word)
		choices = []string{w.meaning, w.wrong1, w.wrong2, w.antonym}
		correct = 0
		explanation = fmt.Sprintf("Dalam wacana akademik formal, kata ‘%s’ bermakna ‘%s’.", w.word, w.meaning)
		tip = "Pelajari kata akademik bersama konteks kalimat dan padanan artinya."
	case 1:
		prompt = fmt.Sprintf("In %s concerning %s, which term represents the direct ANTONYM (opposite meaning) of ‘%s’?", venue, topic, w.word)
		choices = []string{w.meaning, w.antonym, w.wrong1, w.wrong2}
		correct = 1
		explanation = fmt.Sprintf("Lawan kata (antonym) dari ‘%s’ dalam register akademik adalah ‘%s’.", w.word, w.antonym)
		tip = "Perhatikan instruksi soal; pastikan Anda mencari lawan kata (antonym) dan bukan sinonim."
	case 2:
		prompt = fmt.Sprintf("In %s regarding %s, which phrase constitutes the most natural formal collocation for ‘%s’?", venue, topic, w.word)
		choices = []string{w.badColl, w.wrong1 + " " + w.colloc, w.goodColl, w.antonym + " perimeter"}
		correct = 2
		explanation = fmt.Sprintf("Kolokasi baku akademik yang tepat dan alami adalah ‘%s’.", w.goodColl)
		tip = "Kuasai kata bersama pasangannya (collocation) untuk meningkatkan skor Lexical Resource IELTS."
	case 3:
		prompt = fmt.Sprintf("Select the most appropriate academic word to complete the statement from %s on %s: ‘The administration enacted new guidelines to _____ emerging operational risks.’", venue, topic)
		distractorWord := localWords[(i+7)%len(localWords)].word
		anotherWord := localWords[(i+13)%len(localWords)].word
		choices = []string{distractorWord, anotherWord, "disintegrate", w.word}
		correct = 3
		explanation = fmt.Sprintf("Kata ‘%s’ paling tepat secara makna dan register akademik untuk melengkapi konteks tersebut.", w.word)
		tip = "Perhatikan makna keseluruhan kalimat sebelum memilih kata yang memiliki nuansa paling presisi."
	case 4:
		prompt = fmt.Sprintf("In %s examining %s, what is the primary communicative purpose of using the term ‘%s’?", venue, topic, w.word)
		choices = []string{
			fmt.Sprintf("To indicate that the subject matter is %s", w.meaning),
			fmt.Sprintf("To prove that the outcome was %s", w.antonym),
			"To show that the researchers lacked sufficient empirical data",
			"To dismiss the findings as completely insignificant",
		}
		correct = 0
		explanation = fmt.Sprintf("Penggunaan istilah ‘%s’ bertujuan menjelaskan bahwa konteks tersebut adalah ‘%s’.", w.word, w.meaning)
		tip = "Hubungkan makna kosakata dengan fungsi komunikatif penulis dalam wacana akademik."
	case 5:
		prompt = fmt.Sprintf("According to %s on %s, the term ‘%s’ is best defined as:", venue, topic, w.word)
		choices = []string{w.meaning, w.wrong1, w.wrong2, w.antonym}
		correct = 0
		explanation = fmt.Sprintf("Definisi kamus akademik dari ‘%s’ adalah ‘%s’.", w.word, w.meaning)
		tip = "Pahami definisi inti kata untuk membedakannya dari distraktor yang mirip."
	case 6:
		prompt = fmt.Sprintf("In the context of %s regarding %s, which word is LEAST synonymous with ‘%s’?", venue, topic, w.word)
		choices = []string{w.meaning, w.antonym, w.wrong1, w.wrong2}
		correct = 1
		explanation = fmt.Sprintf("Kata ‘%s’ adalah lawan kata (antonym), sehingga merupakan pilihan yang paling tidak sepadan dengan ‘%s’.", w.antonym, w.word)
		tip = "Cermati kata negatif seperti 'LEAST' atau 'EXCEPT' dalam pertanyaan."
	case 7:
		prompt = fmt.Sprintf("Which of the following phrases represents an incorrect or unnatural collocation with ‘%s’ in %s on %s?", w.word, venue, topic)
		choices = []string{w.goodColl, w.badColl, w.meaning + " factor", w.word + " level"}
		correct = 1
		explanation = fmt.Sprintf("Frasa ‘%s’ merupakan kolokasi yang tidak lazim (unnatural) dalam ragam baku akademik.", w.badColl)
		tip = "Hindari menerjemahkan kolokasi kata secara harafiah dari bahasa ibu."
	case 8:
		prompt = fmt.Sprintf("During the discussion in %s on %s, the committee described the proposed solution as ‘%s’. This implies that the solution is:", venue, topic, w.word)
		choices = []string{w.meaning, w.antonym, "completely rejected by all peers", "without any practical value"}
		correct = 0
		explanation = fmt.Sprintf("Pernyataan ‘%s’ mengimplikasikan bahwa solusi tersebut bersifat ‘%s’.", w.word, w.meaning)
		tip = "Fokus pada implikasi makna kata dalam wacana diskusi akademik."
	default:
		prompt = fmt.Sprintf("In %s discussing %s, which term could accurately replace ‘%s’ without altering the academic meaning?", venue, topic, w.word)
		choices = []string{w.meaning, w.antonym, w.wrong1, w.wrong2}
		correct = 0
		explanation = fmt.Sprintf("Kata ‘%s’ dapat digantikan oleh ‘%s’ tanpa mengubah makna wacana.", w.word, w.meaning)
		tip = "Latihan substitusi sinonim sangat bermanfaat untuk skor IELTS Writing dan Speaking."
	}

	permutedChoices, correctIdx := permuteChoices(choices, correct, i+int(target*10))
	return question{
		Type:         "vocabulary",
		Prompt:       prompt,
		Choices:      permutedChoices,
		CorrectIndex: correctIdx,
		Explanation:  explanation,
		LearningTip:  tip,
		IELTSSkill:   "Lexical Resource",
	}
}

func localReadingQuestion(level string, target float64, i int) question {
	cycle := i / len(readingStoryBuilders)
	entity := localEntities[(i*7+cycle*13)%len(localEntities)]
	topic := localTopics[(i*11+cycle*17)%len(localTopics)]
	year := 2012 + cycle*2 + (i % 5)
	val1 := 12 + (i % 75)
	val2 := val1 + 18 + ((i * 3) % 25)
	itemCode := i + 1

	builder := readingStoryBuilders[i%len(readingStoryBuilders)]
	contextStr, evidence, prompt, choices, explanation, tip := builder(year, entity, topic, itemCode, val1, val2)

	permutedChoices, correctIdx := permuteChoices(choices, 0, i+int(target*10))
	return question{
		Type:         "reading",
		Context:      contextStr,
		Evidence:     evidence,
		ErrorTag:     "detail",
		Prompt:       prompt,
		Choices:      permutedChoices,
		CorrectIndex: correctIdx,
		Explanation:  explanation,
		LearningTip:  tip,
		IELTSSkill:   "Reading",
	}
}

type localGap struct {
	sentence    string
	choices     []string
	correct     int
	explanation string
}

var localGaps = []localGap{
	{"Access _____ reliable digital data improved significantly", []string{"at", "to", "for", "with"}, 1, "Kolokasi yang benar adalah access to."},
	{"The research team carried _____ the comprehensive survey", []string{"on", "out", "over", "up"}, 1, "Carry out berarti melaksanakan atau mengeksekusi."},
	{"Enrolment numbers have risen _____ since the initiative began", []string{"steady", "steadily", "steadiness", "steadier"}, 1, "Verb risen diterangkan oleh adverb of manner steadily."},
	{"The empirical findings are consistent _____ previous field observations", []string{"to", "at", "with", "by"}, 2, "Kolokasi baku adalah consistent with."},
	{"The environmental directive had a measurable impact _____ regional emissions", []string{"on", "at", "from", "into"}, 0, "Kolokasi yang tepat adalah impact on."},
	{"The development plan was approved _____ spite of initial budgetary hurdles", []string{"although", "despite", "in", "even"}, 2, "Frasa baku adalah in spite of."},
	{"Researchers attributed the positive shift _____ improved public awareness", []string{"for", "with", "to", "by"}, 2, "Pola verb-preposition yang benar adalah attribute something to something."},
	{"The sustainability scheme aims _____ minimising industrial waste", []string{"at", "on", "for", "with"}, 0, "Aim at dapat diikuti gerund untuk menyatakan sasaran."},
	{"The overall outcome depends _____ how rigorously the samples are tested", []string{"of", "on", "for", "at"}, 1, "Depend on adalah pasangan verb-preposition yang tepat."},
	{"The planning committee took all community feedback _____ account", []string{"to", "for", "into", "with"}, 2, "Frasa idiomatik yang tepat adalah take into account."},
	{"The statistical audit showed a steep decline _____ operating expenses", []string{"in", "on", "at", "with"}, 0, "Noun decline diikuti oleh preposisi in untuk hal yang berkurang."},
	{"The upcoming seminar focuses _____ sustainable architectural design", []string{"in", "to", "on", "for"}, 2, "Focus on adalah kolokasi baku verb-preposition."},
	{"The modern findings differ substantially _____ historical records", []string{"from", "with", "at", "into"}, 0, "Differ from adalah pasangan yang benar."},
	{"The new bus corridor contributed _____ reduced traffic congestion", []string{"for", "to", "with", "on"}, 1, "Contribute to adalah kolokasi yang benar."},
	{"The annual report provides valuable insight _____ consumer habits", []string{"at", "into", "for", "by"}, 1, "Insight into adalah pasangan noun-preposition yang benar."},
	{"The municipal agency is responsible _____ maintaining public safety", []string{"to", "of", "for", "with"}, 2, "Responsible for diikuti oleh noun atau gerund."},
	{"The regulatory intervention resulted _____ widespread compliance", []string{"in", "from", "at", "of"}, 0, "Result in bermakna membuahkan hasil atau mengakibatkan."},
	{"Inclement weather prevented field researchers _____ completing the transect", []string{"to", "from", "for", "at"}, 1, "Prevent someone from doing something adalah pola baku."},
	{"The laboratory technicians succeeded _____ isolating the compound", []string{"to", "at", "in", "for"}, 2, "Succeed in diikuti oleh gerund (-ing)."},
	{"The clinical trial was conducted _____ accordance with ethics protocols", []string{"on", "at", "in", "for"}, 2, "Frasa bakunya adalah in accordance with."},
	{"The policy revision is likely _____ influence long-term investments", []string{"to", "for", "of", "at"}, 0, "Adjective likely diikuti to-infinitive."},
	{"The preliminary data was not sufficient _____ confirm the hypothesis", []string{"for", "to", "at", "with"}, 1, "Sufficient dapat diikuti to-infinitive."},
	{"The project yields were considerably greater _____ previously anticipated", []string{"that", "then", "than", "as"}, 2, "Comparative greater berpasangan dengan than."},
	{"Field measurements were recorded _____ a period of six months", []string{"during", "while", "since", "until"}, 0, "During diikuti oleh noun phrase yang menyatakan rentang waktu."},
	{"The sensor network was designed _____ monitor groundwater shifts", []string{"for", "to", "at", "by"}, 1, "Designed to diikuti base verb untuk menyatakan tujuan perancangan."},
	{"The steering committee reached a consensus _____ the proposed amendments", []string{"on", "with", "in", "to"}, 0, "Consensus on adalah pasangan noun-preposition yang tepat."},
	{"_____ of severe funding restrictions, the project delivered solid results", []string{"Regardless", "Although", "Despite", "Inasmuch"}, 0, "Regardless of bermakna terlepas dari atau tanpa memandang."},
	{"The recorded spectra were _____ to those documented in earlier trials", []string{"identical", "identically", "identity", "identifying"}, 0, "Adjective identical menerangkan subjek setelah linking verb were."},
	{"The updated regulations are conducive _____ sustainable industrial growth", []string{"for", "to", "with", "in"}, 1, "Conducive to adalah pasangan adjective-preposition baku."},
	{"The scientific board expressed skepticism _____ the unverified assertions", []string{"about", "towards", "against", "into"}, 0, "Skepticism about adalah kolokasi yang tepat."},
}

var fillBlankContexts = []string{
	"In research concerning",
	"According to an academic analysis of",
	"In a recent report on",
	"During a baseline evaluation of",
	"Within the scholarly framework of",
	"In empirical findings related to",
	"Following an institutional review of",
	"In an analytical study on",
	"According to field data regarding",
	"In a policy assessment on",
	"During an environmental survey of",
	"In the comprehensive evaluation of",
	"Based on findings from",
	"In the preliminary assessment of",
	"Throughout the project on",
	"In an interdisciplinary paper on",
	"Across recent publications on",
	"In the latest review of",
	"Under the regional inquiry into",
	"In an evidence-based audit of",
}

func localFillBlankQuestion(level string, target float64, i int) question {
	g := localGaps[i%len(localGaps)]
	cycle := i / len(localGaps)
	lead := fillBlankContexts[cycle%len(fillBlankContexts)]
	topic := localTopics[(i*7+cycle)%len(localTopics)]

	prompt := fmt.Sprintf("%s %s: %s.", lead, topic, g.sentence)
	permutedChoices, correctIdx := permuteChoices(g.choices, g.correct, i+int(target*10))
	return question{
		Type:         "fill_blank",
		Prompt:       prompt,
		Choices:      permutedChoices,
		CorrectIndex: correctIdx,
		Explanation:  g.explanation,
		LearningTip:  "Perhatikan kata kerja, kata sifat, atau kata benda utama untuk menentukan preposisi dan kolokasi yang tepat.",
		IELTSSkill:   "Grammatical Accuracy",
	}
}

func localListeningQuestion(level string, target float64, i int) question {
	cycle := i / len(listeningScenarioBuilders)
	place := localEntities[(i*7+cycle*11)%len(localEntities)]
	topic := localTopics[(i*13+cycle*19)%len(localTopics)]
	hour := 8 + (i % 9)
	minute := ((i * 5) % 4) * 15
	fee := 30 + (i%20)*5
	roomNum := 101 + (i % 35)
	itemCode := i + 1

	builder := listeningScenarioBuilders[i%len(listeningScenarioBuilders)]
	dialogue, prompt, choices, explanation := builder(itemCode, place, topic, roomNum, hour, minute, fee)

	permutedChoices, correctIdx := permuteChoices(choices, 0, i+int(target*10))
	return question{
		Type:         "listening",
		Context:      dialogue,
		Prompt:       prompt,
		Choices:      permutedChoices,
		CorrectIndex: correctIdx,
		Explanation:  explanation,
		LearningTip:  "Catat detail spesifik seperti angka, lokasi, atau persyaratan sebelum dan sesudah pembicara memberikan konfirmasi akhir.",
		IELTSSkill:   "Listening",
	}
}

var errorLeads = []string{
	"Regarding field work in",
	"In the archive review of",
	"In the empirical trial on",
	"Within the program for",
	"In the repository analysis of",
	"During the advisory review of",
	"At the keynote session on",
	"During the symposium regarding",
	"In the policy assessment of",
	"Across the baseline measurements of",
	"In the conservation initiative for",
	"In the analytical research on",
	"In the supervisory appraisal of",
	"Regarding institutional feedback on",
	"In the municipal survey on",
	"In the comparative study of",
	"For the regional delegation on",
	"Regarding reform proposals for",
	"Concerning the keynote presentation on",
	"In the publication review of",
	"Throughout the scheduled audit of",
	"In the multi-center study on",
	"During the preliminary examination of",
	"Across the longitudinal monitoring of",
	"In the pilot deployment of",
	"Within the strategic framework for",
	"During the secondary evaluation of",
	"In the observational trial on",
	"Concerning the formal inquiry into",
	"Under the collaborative venture for",
	"In the diagnostic appraisal of",
	"Throughout the field trials regarding",
	"In the statistical audit of",
	"During the technical inspection of",
	"Across the quarterly assessment of",
	"In the clinical evaluation of",
	"Concerning the public briefing on",
	"Within the campus initiative for",
	"In the environmental screening of",
	"During the workshop on",
	"In the exploratory investigation of",
	"Across the case evaluations of",
	"In the systematic overview of",
	"Regarding the ongoing review of",
	"During the stakeholder consultation on",
	"In the quality assurance review of",
	"Across the experimental testing of",
	"In the academic symposium on",
	"Under the regulatory assessment of",
	"Throughout the milestone review of",
	"In the scientific report on",
	"During the institutional survey regarding",
	"In the comprehensive appraisal of",
	"Regarding the developmental phase of",
	"In the follow-up investigation on",
	"Across the cohort studies on",
	"In the initial benchmarking of",
	"During the bilateral discussions on",
	"In the retrospective analysis of",
	"Concerning the final synthesis of",
}

func localLevelIndex(level string) int {
	switch level {
	case "A1":
		return 0
	case "A2":
		return 1
	case "B1":
		return 2
	case "B2":
		return 3
	case "C1":
		return 4
	case "C2":
		return 5
	default:
		return 2
	}
}

func localTargetIndex(target float64) int {
	idx := int((target-4.0)*2 + 0.5)
	if idx < 0 {
		return 0
	}
	if idx > 10 {
		return 10
	}
	return idx
}

func localErrorIdentificationQuestion(level string, target float64, i int) question {
	levelIdx := localLevelIndex(level)
	targetIdx := localTargetIndex(target)
	cellOffset := levelIdx*11 + targetIdx

	cycle := i / 48
	tmplIdx := (i + cellOffset*12) % 48

	lead := errorLeads[(cycle*7+cellOffset*13+tmplIdx*3)%len(errorLeads)]
	entity := localEntities[(i*3+cycle*17+cellOffset*19+tmplIdx*5)%len(localEntities)]
	topic := localTopics[(i*5+cycle*23+cellOffset*29+tmplIdx*7)%len(localTopics)]

	templates := []struct {
		sentence    string
		choices     []string
		correct     int
		explanation string
		tip         string
	}{
		// Group 1: Error at index 0 (Choice A) - 12 patterns
		{
			fmt.Sprintf("%s %s, the comprehensive report [provide] concrete [guidelines] for regional [planners] under [supervision].", lead, topic),
			[]string{"provide", "guidelines", "planners", "supervision"}, 0,
			"Subjek 'the comprehensive report' berbentuk tunggal, sehingga kata kerja yang benar adalah 'provides', bukan 'provide'.",
			"Cari subjek inti kalimat untuk mencocokkan singular/plural verb.",
		},
		{
			fmt.Sprintf("%s %s, [much] [participants] [attended] the opening seminar [organized] by %s.", lead, topic, entity),
			[]string{"much", "participants", "attended", "organized"}, 0,
			"Kata 'participants' adalah kata benda jamak terhitung, sehingga harus dipasangkan dengan 'many', bukan 'much'.",
			"Bedakan pemakaian much (uncountable) dan many (countable plural).",
		},
		{
			fmt.Sprintf("%s %s, [despite] %s [struggled] with budget limits, it [achieved] the key [milestone].", lead, topic, entity),
			[]string{"despite", "struggled", "achieved", "milestone"}, 0,
			"Kata 'despite' adalah preposisi yang diikuti frasa benda. Karena diikuti klausa utuh (subjek + kata kerja), kata hubung yang tepat adalah 'Although'.",
			"Gunakan although/even though untuk klausa penuh, dan despite/in spite of untuk frasa benda.",
		},
		{
			fmt.Sprintf("%s %s, [less] [researchers] [enrolled] in the workshop than in the [humanities].", lead, topic),
			[]string{"less", "researchers", "enrolled", "humanities"}, 0,
			"Kata benda jamak terhitung seperti 'researchers' harus dibandingkan menggunakan 'fewer', bukan 'less'.",
			"Gunakan fewer untuk kata benda yang dapat dihitung dan less untuk kata benda tak terhitung.",
		},
		{
			fmt.Sprintf("%s %s, [having saw] the field [measurements], %s [revised] its operational [schedule].", lead, topic, entity),
			[]string{"having saw", "measurements", "revised", "schedule"}, 0,
			"Bentuk perfect participle membutuhkan kata kerja bentuk ketiga (V3), sehingga yang benar adalah 'Having seen', bukan 'Having saw'.",
			"Ingat bahwa perfect participle selalu menggunakan pola having + V3.",
		},
		{
			fmt.Sprintf("%s %s, seldom [they had] [observed] such rapid [growth] across the regional [sector].", lead, topic),
			[]string{"they had", "observed", "growth", "sector"}, 0,
			"Ketika 'Seldom' diletakkan di awal kalimat, posisi kata kerja bantu dibalik mendahului subjek menjadi 'had they observed'.",
			"Adverbial negatif di awal kalimat (seldom, rarely, scarcely) memicu pembalikan posisi kata kerja bantu dan subjek.",
		},
		{
			fmt.Sprintf("%s %s, [him] and his [co-authors] [submitted] the final manuscript before the [deadline].", lead, topic),
			[]string{"him", "co-authors", "submitted", "deadline"}, 0,
			"Kata ganti subjek yang benar adalah 'He' ('He and his co-authors'), bukan kata ganti objek 'Him'.",
			"Gunakan subject pronoun (I, he, she, they) untuk posisi pelaku subjek kalimat.",
		},
		{
			fmt.Sprintf("%s %s, [in] Monday morning, %s [initiated] the next [phase] of the field [project].", lead, topic, entity),
			[]string{"in", "initiated", "phase", "project"}, 0,
			"Untuk keterangan waktu yang diawali nama hari spesifik ('Monday morning'), gunakan preposisi 'on', bukan 'in'.",
			"Gunakan preposisi 'on' untuk nama hari dan tanggal spesifik.",
		},
		{
			fmt.Sprintf("%s %s, [remarkably] [breakthroughs] were [reported] by %s during the second [quarter].", lead, topic, entity),
			[]string{"remarkably", "breakthroughs", "reported", "quarter"}, 0,
			"Kata benda 'breakthroughs' harus diterangkan oleh kata sifat 'remarkable', bukan adverb 'remarkably'.",
			"Gunakan adjective untuk menerangkan noun, dan adverb untuk menerangkan verb/adjective.",
		},
		{
			fmt.Sprintf("%s %s, [between] the five project [teams], %s demonstrated the [highest] degree of [innovation].", lead, topic, entity),
			[]string{"between", "teams", "highest", "innovation"}, 0,
			"Untuk perbandingan lebih dari dua pihak ('five project teams'), gunakan preposisi 'among', bukan 'between'.",
			"Gunakan 'between' untuk dua pihak dan 'among' untuk tiga pihak atau lebih.",
		},
		{
			fmt.Sprintf("%s %s, [either] %s and the municipal agency [approved] the proposed [guidelines] without [objections].", lead, topic, entity),
			[]string{"either", "approved", "guidelines", "objections"}, 0,
			"Pasangan konjungsi untuk kata sambung 'and' adalah 'both' ('Both X and Y'), bukan 'either'.",
			"Ingat pasangan correlative conjunctions: both... and, either... or, neither... nor.",
		},
		{
			fmt.Sprintf("%s %s, [another] [criteria] were [established] by %s to evaluate student [proficiency].", lead, topic, entity),
			[]string{"another", "criteria", "established", "proficiency"}, 0,
			"Kata 'criteria' adalah bentuk jamak dari 'criterion'. Pasangan yang tepat adalah 'other criteria' atau 'another criterion'.",
			"Perhatikan bentuk tunggal/jamak serapan Latin/Yunani (criterion/criteria, phenomenon/phenomena).",
		},

		// Group 2: Error at index 1 (Choice B) - 12 patterns
		{
			fmt.Sprintf("%s %s, %s [completed] the survey [remarkable] quickly despite [adverse] weather [conditions].", lead, topic, entity),
			[]string{"completed", "remarkable", "adverse", "conditions"}, 1,
			"Kata 'quickly' adalah adverb, sehingga kata penjelasnya harus berupa adverb ('remarkably'), bukan kata sifat 'remarkable'.",
			"Gunakan adverb berakhiran -ly untuk menerangkan kata sifat atau kata keterangan lainnya.",
		},
		{
			fmt.Sprintf("%s %s, %s succeeded [in] [isolate] the active [compound] from regional [specimens].", lead, topic, entity),
			[]string{"in", "isolate", "compound", "specimens"}, 1,
			"Kata kerja setelah preposisi 'in' harus berbentuk gerund (-ing), yaitu 'isolating', bukan 'isolate'.",
			"Semua kata kerja setelah preposisi wajib berakhiran -ing.",
		},
		{
			fmt.Sprintf("The %s repository maintained by %s contains [detailed] [researches] and valuable [data] on regional [trends].", topic, entity),
			[]string{"detailed", "researches", "data", "trends"}, 1,
			"Kata 'research' dalam konteks formal merupakan kata benda tak terhitung (uncountable) dan tidak memakai akhiran jamak ('researches'). Bentuk yang benar adalah 'research'.",
			"Perhatikan kata benda tak terhitung (uncountable) seperti research, evidence, advice, information.",
		},
		{
			fmt.Sprintf("The advisory board for %s [insisted] that %s [submits] its [updated] protocols [without] delay.", topic, entity),
			[]string{"insisted", "submits", "updated", "without"}, 1,
			"Setelah kata kerja tuntutan seperti 'insisted that', kata kerja klausa berikutnya harus berupa kata kerja dasar murni tanpa akhiran -s, yaitu 'submit'.",
			"Setelah kata kerja mandasif (insist/recommend/suggest that), gunakan kata kerja bentuk dasar murni.",
		},
		{
			fmt.Sprintf("At the %s keynote, the senior [investigator] [which] [directed] the %s project [received] widespread acclaim.", topic, entity),
			[]string{"investigator", "which", "directed", "received"}, 1,
			"Subjek 'the senior investigator' merujuk pada orang, sehingga kata ganti penghubung yang tepat adalah 'who', bukan 'which'.",
			"Gunakan 'who' untuk manusia dan 'which' untuk benda mati atau konsep.",
		},
		{
			fmt.Sprintf("During the %s symposium, the newly appointed [director] [were] confident that %s would [reach] its [targets].", topic, entity),
			[]string{"director", "were", "reach", "targets"}, 1,
			"Subjek 'the newly appointed director' berbentuk tunggal, sehingga to be lampau yang tepat adalah 'was', bukan 'were'.",
			"Kenali inti subjek sebelum preposisi untuk menentukan apakah tunggal atau jamak.",
		},
		{
			fmt.Sprintf("%s %s, if %s [completes] the study, it [will publishes] the [summary] next [month].", lead, topic, entity),
			[]string{"completes", "will publishes", "summary", "month"}, 1,
			"Setelah modal auxiliary 'will', kata kerja harus berbentuk dasar murni ('publish'), bukan 'publishes'.",
			"Modal auxiliary selalu diikuti bare infinitive (kata kerja bentuk pertama tanpa akhiran -s).",
		},
		{
			fmt.Sprintf("%s %s, the [revised] estimate proved to be [more closer] to the [actual] count than the preliminary [figure].", lead, topic),
			[]string{"revised", "more closer", "actual", "figure"}, 1,
			"Kata 'closer' sudah merupakan bentuk komparatif; penambahan 'more' menciptakan double comparative yang tidak baku ('closer', bukan 'more closer').",
			"Hindari penggunaan double comparative (jangan gabungkan more dengan akhiran -er).",
		},
		{
			fmt.Sprintf("%s %s, %s spent several [months] [to develop] a comprehensive [curriculum] for local [institutions].", lead, topic, entity),
			[]string{"months", "to develop", "curriculum", "institutions"}, 1,
			"Pola ekspresi waktu 'spend time' diikuti oleh gerund ('developing'), bukan to-infinitive ('to develop').",
			"Ingat pola: spend + time/money + V-ing.",
		},
		{
			fmt.Sprintf("%s %s, the research [paper] [publishing] by %s [won] first prize at the national [symposium].", lead, topic, entity),
			[]string{"paper", "publishing", "won", "symposium"}, 1,
			"Makna kalimat membutuhkan past participle pasif 'published' ('yang diterbitkan'), bukan present participle aktif 'publishing'.",
			"Bedakan V-ing (aktif/yang melakukan) dan V3 (pasif/yang dikenai) pada reduced relative clause.",
		},
		{
			fmt.Sprintf("%s %s, the executive committee resolved the [dispute] by [themself] before [consulting] external [advisors].", lead, topic),
			[]string{"dispute", "themself", "consulting", "advisors"}, 1,
			"Bentuk reflexive pronoun untuk subjek jamak 'the executive committee' adalah 'themselves', bukan 'themself'.",
			"Reflexive pronoun untuk third person plural adalah themselves.",
		},
		{
			fmt.Sprintf("%s %s, the team is looking [forward] to [receive] official [feedback] from %s this [week].", lead, topic, entity),
			[]string{"forward", "to receive", "feedback", "week"}, 1,
			"Dalam frasa idiomatis 'look forward to', kata 'to' berfungsi sebagai preposisi sehingga wajib diikuti gerund ('receiving'), bukan 'receive'.",
			"Catat ekspresi di mana 'to' adalah preposisi (look forward to, be used to, object to + V-ing).",
		},

		// Group 3: Error at index 2 (Choice C) - 12 patterns
		{
			fmt.Sprintf("%s %s, if the policy [changes], market [prices] will [fell] drastically next [month].", lead, topic),
			[]string{"changes", "prices", "fell", "month"}, 2,
			"Setelah kata bantu modal 'will', kata kerja harus berbentuk dasar ('fall'), bukan bentuk lampau ('fell').",
			"Modal auxiliary (will/can/must/should) selalu diikuti kata kerja bentuk dasar tanpa to.",
		},
		{
			fmt.Sprintf("Long-term [progress] in %s heavily [depends] [of] rigorous baseline [measurements] by %s.", topic, entity),
			[]string{"progress", "depends", "of", "measurements"}, 2,
			"Kata kerja 'depend' berpasangan baku dengan preposisi 'on' ('depend on'), bukan 'depend of'.",
			"Catat dependent prepositions sebagai satu kesatuan kosakata.",
		},
		{
			fmt.Sprintf("%s %s, %s focuses [on] reducing [waste], conserving [energy], and [to promote] green [technology].", lead, topic, entity),
			[]string{"on", "waste", "to promote", "technology"}, 2,
			"Dalam deret aktivitas yang sejajar setelah preposisi 'on', bentuk kata kerjanya harus konsisten berupa gerund ('promoting'), bukan 'to promote'.",
			"Pastikan semua elemen dalam susunan paralel konjungsi memiliki struktur gramatikal yang setara.",
		},
		{
			fmt.Sprintf("%s %s, the [more] rigorously analysts [examined] the dataset, the [most] reliable the [findings] became.", lead, topic),
			[]string{"more", "examined", "most", "findings"}, 2,
			"Pada struktur perbandingan berimbang (the more...), kedua sisi harus memakai bentuk comparative, yaitu 'the more reliable', bukan bentuk superlative 'the most'.",
			"Pastikan kedua sisi perbandingan menggunakan bentuk comparative (the more... the more...).",
		},
		{
			fmt.Sprintf("%s %s, the supervisory panel [made] the [team] [to rewrite] the entire preliminary [draft].", lead, topic),
			[]string{"made", "team", "to rewrite", "draft"}, 2,
			"Kata kerja 'make' dalam kalimat aktif ('made the team...') diikuti kata kerja dasar tanpa to ('rewrite'), bukan 'to rewrite'.",
			"Kata kerja causative make/let diikuti bare infinitive (V1 tanpa to).",
		},
		{
			fmt.Sprintf("%s %s, each of the [participating] institutions [has] submitted [they] official feedback to the [secretariat].", lead, topic),
			[]string{"participating", "has", "they", "secretariat"}, 2,
			"Sebelum kata benda 'official feedback', harus menggunakan kata ganti kepemilikan 'their', bukan kata ganti subjek 'they'.",
			"Gunakan possessive adjective (their/its/his/her) sebelum kata benda.",
		},
		{
			fmt.Sprintf("%s %s, %s announced that all [delegates] [must] submit credentials, but no one [have] done so [yet].", lead, topic, entity),
			[]string{"delegates", "must", "have", "yet"}, 2,
			"Subjek tak tentu 'no one' berbentuk tunggal (singular), sehingga kata kerja yang tepat adalah 'has done', bukan 'have done'.",
			"Indefinite pronouns seperti everyone, someone, no one selalu membutuhkan singular verb.",
		},
		{
			fmt.Sprintf("%s %s, all water [samples] were carefully [labeled] and [collect] by trained technicians during the [expedition].", lead, topic),
			[]string{"samples", "labeled", "collect", "expedition"}, 2,
			"Bentuk pasif lampau yang sejajar dengan 'labeled' membutuhkan past participle 'collected', bukan bentuk dasar 'collect'.",
			"Pastikan bentuk kata kerja pasif konsisten (were labeled and collected).",
		},
		{
			fmt.Sprintf("%s %s, the proposed [system] is not only [efficient] [and] also remarkably sustainable across the [region].", lead, topic),
			[]string{"system", "efficient", "and", "region"}, 2,
			"Pasangan baku untuk 'not only' adalah 'but' atau 'but also' ('not only efficient but also remarkably sustainable'), bukan 'and also'.",
			"Ingat pasangan konjungsi correlative: not only... but also.",
		},
		{
			fmt.Sprintf("%s %s, the newly installed [turbine] generated [more] electricity [as] the previous model during field [trials].", lead, topic),
			[]string{"turbine", "more", "as", "trials"}, 2,
			"Bentuk comparative 'more' berpasangan dengan konjungsi pembanding 'than', bukan 'as' ('more electricity than...').",
			"Gunakan than setelah comparative adjective/adverb.",
		},
		{
			fmt.Sprintf("%s %s, %s dedicated [substantial] [resources] [to modernize] the outdated IT [infrastructure].", lead, topic, entity),
			[]string{"substantial", "resources", "to modernize", "infrastructure"}, 2,
			"Kata kerja 'dedicate ... to' diikuti gerund ('to modernizing'), karena 'to' di sini adalah preposisi tujuan.",
			"Pola: dedicate time/resources to + V-ing.",
		},
		{
			fmt.Sprintf("%s %s, the university [requires] that every [applicant] [submits] three official letters of [recommendation].", lead, topic),
			[]string{"requires", "applicant", "submits", "recommendation"}, 2,
			"Setelah kata kerja tuntutan formal 'requires that', kata kerja klausa berikutnya harus berbentuk dasar (bare subjunctive 'submit'), tanpa akhiran -s.",
			"Subjunctive mood (require/demand/order that) memakai base form (V1 tanpa -s).",
		},

		// Group 4: Error at index 3 (Choice D) - 12 patterns
		{
			fmt.Sprintf("%s %s, the survey [revealed] that local [residents] were extremely [satisfied] with public [transportations].", lead, topic),
			[]string{"revealed", "residents", "satisfied", "transportations"}, 3,
			"Kata 'transportation' merupakan kata benda tak terhitung (uncountable) dan tidak memakai akhiran jamak -s. Bentuk yang tepat adalah 'transportation'.",
			"Perhatikan kata benda massa yang tidak boleh diberi akhiran jamak -s.",
		},
		{
			fmt.Sprintf("%s %s, the board [agreed] that the proposed [initiative] was [more] cost-effective [as] the previous trial.", lead, topic),
			[]string{"agreed", "initiative", "more", "as"}, 3,
			"Bentuk perbandingan lebih ('more cost-effective') harus dipasangkan dengan kata sambung pembanding 'than', bukan 'as'.",
			"Setelah comparative (more/-er), gunakan kata sambung than.",
		},
		{
			fmt.Sprintf("%s %s, the delegation [arrived] at the %s [facility] [promptly] [in] Tuesday morning.", lead, topic, entity),
			[]string{"arrived", "facility", "promptly", "in"}, 3,
			"Untuk waktu yang diawali nama hari spesifik ('Tuesday morning'), preposisi yang tepat adalah 'on', bukan 'in'.",
			"Gunakan on untuk hari dan tanggal spesifik (on Monday, on Tuesday morning).",
		},
		{
			fmt.Sprintf("%s %s, %s [considers] the proposed [reform] to be [beneficial] for public [healthy].", lead, topic, entity),
			[]string{"considers", "reform", "beneficial", "healthy"}, 3,
			"Setelah kata sifat 'public', dibutuhkan kata benda yaitu 'health', bukan kata sifat 'healthy'.",
			"Tentukan kelas kata (part of speech) yang dibutuhkan kalimat: noun, verb, adjective, atau adverb.",
		},
		{
			fmt.Sprintf("%s %s, the keynote [speaker] gave an [inspiring] presentation that [had] a profound [affect] on attendees.", lead, topic),
			[]string{"speaker", "inspiring", "had", "affect"}, 3,
			"Frasa 'had a profound...' membutuhkan kata benda yang berarti dampak atau akibat, yaitu 'effect', bukan 'affect' (kata kerja).",
			"Ingat: affect biasanya adalah kata kerja (verb), sedangkan effect adalah kata benda (noun).",
		},
		{
			fmt.Sprintf("%s %s, neither the [faculty] dean nor the [department] chairs [were] aware of the policy [changeable].", lead, topic),
			[]string{"faculty", "department", "were", "changeable"}, 3,
			"Sebagai objek dari 'policy...', dibutuhkan kata benda 'change' (perubahan), bukan kata sifat 'changeable'.",
			"Perhatikan imbuhan akhiran kata (suffix): -able menandai adjective, bukan noun.",
		},
		{
			fmt.Sprintf("%s %s, the theoretical [framework] proposed by %s [seemed] entirely [reasonable] and [logically].", lead, topic, entity),
			[]string{"framework", "seemed", "reasonable", "logically"}, 3,
			"Setelah linking verb 'seemed', kata keterangan yang mengikuti harus berupa kata sifat predicative 'logical', bukan adverb 'logically'.",
			"Linking verbs (seem, appear, look, sound) diikuti adjective, bukan adverb.",
		},
		{
			fmt.Sprintf("%s %s, the institute [earned] widespread [acclaim] and a solid [reputation] [about] academic integrity.", lead, topic),
			[]string{"earned", "acclaim", "reputation", "about"}, 3,
			"Kata benda 'reputation' berkolokasi dengan preposisi 'for' ('reputation for academic integrity'), bukan 'about'.",
			"Pelajari noun + preposition collocations baku dalam ragam akademik.",
		},
		{
			fmt.Sprintf("%s %s, the research [fellow] has been [analyzing] regional weather [patterns] [since] five years.", lead, topic),
			[]string{"fellow", "analyzing", "patterns", "since"}, 3,
			"Untuk menyatakan durasi rentang waktu ('five years'), preposisi yang tepat adalah 'for', bukan 'since' (yang digunakan untuk titik awal waktu).",
			"Gunakan 'for' untuk periode durasi dan 'since' untuk titik awal waktu tertentu.",
		},
		{
			fmt.Sprintf("%s %s, the security team [implemented] strict [protocols] to prevent unauthorized [visitors] [to] entering the lab.", lead, topic),
			[]string{"implemented", "protocols", "visitors", "to"}, 3,
			"Pola kata kerja 'prevent' adalah 'prevent someone from doing something', bukan 'to entering'.",
			"Ingat pola: prevent/stop/discourage someone from + V-ing.",
		},
		{
			fmt.Sprintf("%s %s, the committee [convened] yesterday [afternoon] to [discuss] [about] the proposed budget.", lead, topic),
			[]string{"convened", "afternoon", "discuss", "about"}, 3,
			"Kata kerja transitif 'discuss' langsung diikuti objek tanpa preposisi 'about' ('discuss the proposed budget').",
			"Kata kerja seperti discuss, consider, mention tidak membutuhkan preposisi tambahan.",
		},
		{
			fmt.Sprintf("%s %s, the newly appointed [coordinator] [spoke] [enthusiastically] [regarding of] the upcoming initiative.", lead, topic),
			[]string{"coordinator", "spoke", "enthusiastically", "regarding of"}, 3,
			"Bentuk yang baku adalah 'regarding' atau 'with regard to', bukan 'regarding of'.",
			"Perhatikan prepositional phrases baku: regarding / with regard to / in regard to.",
		},
	}

	t := templates[tmplIdx]
	return question{
		Type:         "error_identification",
		Prompt:       fmt.Sprintf("Identify the error: %s", t.sentence),
		Choices:      t.choices,
		CorrectIndex: t.correct,
		Explanation:  t.explanation,
		LearningTip:  t.tip,
		IELTSSkill:   "Grammatical Range and Accuracy",
	}
}
