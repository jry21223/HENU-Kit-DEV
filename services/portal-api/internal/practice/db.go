package practice

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
)

// QuizCraftDB reads from the real QuizCraft database.
type QuizCraftDB struct {
	conn *sql.DB
}

// NewQuizCraftDB creates a QuizCraftDB.
func NewQuizCraftDB(conn *sql.DB) *QuizCraftDB {
	return &QuizCraftDB{conn: conn}
}

// GetSchools returns the school/major/subject/list hierarchy from QuizCraft banks.
// Since QuizCraft doesn't have a school/major hierarchy, we derive it from bank_key patterns.
func (db *QuizCraftDB) GetSchools() ([]School, error) {
	// Get all sealed banks with their question counts
	rows, err := db.conn.Query(`
		SELECT b.id, b.bank_key, b.name, bv.id as version_id, COUNT(bvq.question_id) as q_count
		FROM quizcraft_banks b
		JOIN quizcraft_bank_versions bv ON bv.bank_id = b.id AND bv.id = b.active_version_id
		LEFT JOIN quizcraft_bank_version_questions bvq ON bvq.bank_version_id = bv.id
		WHERE bv.sealed_at IS NOT NULL
		GROUP BY b.id, b.bank_key, b.name, bv.id
		ORDER BY b.bank_key
	`)
	if err != nil {
		return nil, fmt.Errorf("query banks: %w", err)
	}
	defer rows.Close()

	type bankRow struct {
		BankID     string
		BankKey    string
		Name       string
		VersionID  string
		QuestionCT int
	}

	var banks []bankRow
	for rows.Next() {
		var b bankRow
		if err := rows.Scan(&b.BankID, &b.BankKey, &b.Name, &b.VersionID, &b.QuestionCT); err != nil {
			return nil, fmt.Errorf("scan bank: %w", err)
		}
		banks = append(banks, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate banks: %w", err)
	}

	// Group banks into a school/major/subject hierarchy
	// For now, put all banks under one school with one major
	var lists []QuizListMeta
	for _, b := range banks {
		// Get completion rate from question stats
		completion := db.getCompletion(b.BankID)
		lists = append(lists, QuizListMeta{
			ID:         b.BankKey,
			Name:       b.Name,
			Creator:    "HENU Kit",
			Tags:       []string{},
			PoolKey:    b.BankKey,
			Count:      b.QuestionCT,
			Completion: completion,
		})
	}

	if len(lists) == 0 {
		return []School{}, nil
	}

	return []School{
		{
			ID:   "henu",
			Name: "河南大学",
			Majors: []Major{
				{
					ID:   "all",
					Name: "全部题库",
					Subjects: []Subject{
						{
							ID:    "all-sub",
							Name:  "全部科目",
							Lists: lists,
						},
					},
				},
			},
		},
	}, nil
}

// GetQuestions returns questions for a given bank_key.
func (db *QuizCraftDB) GetQuestions(bankKey string) ([]Question, error) {
	rows, err := db.conn.Query(`
		SELECT q.source_question_id, qv.chapter_name, qv.content, qv.options, qv.answer, qv.analysis,
			   COALESCE(qs.attempt_count, 0), COALESCE(qs.correct_count, 0)
		FROM quizcraft_banks b
		JOIN quizcraft_bank_versions bv ON bv.bank_id = b.id AND bv.id = b.active_version_id
		JOIN quizcraft_bank_version_questions bvq ON bvq.bank_version_id = bv.id
		JOIN quizcraft_questions q ON q.id = bvq.question_id
		JOIN quizcraft_question_versions qv ON qv.question_id = q.id AND qv.bank_id = b.id
		LEFT JOIN quizcraft_question_stats qs ON qs.question_id = q.id
		WHERE b.bank_key = $1 AND bv.sealed_at IS NOT NULL
		ORDER BY bvq.position
	`, bankKey)
	if err != nil {
		return nil, fmt.Errorf("query questions: %w", err)
	}
	defer rows.Close()

	var questions []Question
	seq := 0
	for rows.Next() {

		var (
			sourceID    string
			chapter     sql.NullString
			content     string
			optionsJSON []byte
			answerJSON  []byte
			analysis    sql.NullString
			attemptCT   int
			correctCT   int
		)
		if err := rows.Scan(&sourceID, &chapter, &content, &optionsJSON, &answerJSON, &analysis, &attemptCT, &correctCT); err != nil {
			return nil, fmt.Errorf("scan question: %w", err)
		}

		// Parse options (jsonb array of strings)
		var options []string
		if err := json.Unmarshal(optionsJSON, &options); err != nil {
			options = []string{"A", "B", "C", "D"}
		}
		// Pad or truncate to 4
		for len(options) < 4 {
			options = append(options, "")
		}
		var fixed [4]string
		copy(fixed[:], options[:4])

		// Parse answer (jsonb - could be int index or array)
		var answerIdx int
		if err := json.Unmarshal(answerJSON, &answerIdx); err != nil {
			// Try array format
			var answerArr []int
			if err2 := json.Unmarshal(answerJSON, &answerArr); err2 == nil && len(answerArr) > 0 {
				answerIdx = answerArr[0]
			}
		}

		// Compute difficulty from accuracy (inverse relationship)
		accuracy := 0
		if attemptCT > 0 {
			accuracy = int(math.Round(float64(correctCT) / float64(attemptCT) * 100))
		}
		difficulty := 10.0 - float64(accuracy)/10.0*8.0
		if difficulty < 1.0 {
			difficulty = 1.0
		}
		if difficulty > 10.0 {
			difficulty = 10.0
		}

		chapterName := ""
		if chapter.Valid {
			chapterName = chapter.String
		}
		explanation := ""
		if analysis.Valid {
			explanation = analysis.String
		}

		seq++
		questions = append(questions, Question{
			ID:          fmt.Sprintf("%s-%02d", bankKey, seq),
			Subject:     bankKey,
			Chapter:     chapterName,
			Difficulty:  math.Round(difficulty*10) / 10,
			Stem:        content,
			Options:     fixed,
			Answer:      answerIdx,
			Explanation: explanation,
			Accuracy:    accuracy,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate questions: %w", err)
	}

	if questions == nil {
		questions = []Question{}
	}
	return questions, nil
}

// getCompletion returns the average completion percentage for a bank's questions.
func (db *QuizCraftDB) getCompletion(bankID string) int {
	var avg sql.NullFloat64
	err := db.conn.QueryRow(`
		SELECT AVG(CASE WHEN qs.attempt_count > 0 THEN 100.0 ELSE 0.0 END)
		FROM quizcraft_bank_version_questions bvq
		JOIN quizcraft_bank_versions bv ON bv.id = bvq.bank_version_id
		LEFT JOIN quizcraft_question_stats qs ON qs.question_id = bvq.question_id
		WHERE bv.bank_id = $1 AND bv.id = (SELECT active_version_id FROM quizcraft_banks WHERE id = $1)
	`, bankID).Scan(&avg)
	if err != nil || !avg.Valid {
		return 0
	}
	return int(math.Round(avg.Float64))
}

// GetLeaderboard returns ranking data from QuizCraft.
// For now returns mock since the ranking tables need a running QuizCraft service to populate.
func (db *QuizCraftDB) GetLeaderboard(period string) []LeaderboardRow {
	// Try to read from quizcraft_ranking_settlement_events
	rows, err := db.conn.Query(`
		SELECT standings FROM quizcraft_ranking_settlement_events
		WHERE scope = 'overall'
		ORDER BY created_at DESC LIMIT 1
	`)
	if err != nil {
		return MockLeaderboard(period)
	}
	defer rows.Close()

	if rows.Next() {
		var standingsJSON []byte
		if err := rows.Scan(&standingsJSON); err == nil {
			var standings []struct {
				UserID   string `json:"user_id"`
				Nickname string `json:"nickname"`
				Score    int    `json:"score"`
			}
			if json.Unmarshal(standingsJSON, &standings) == nil && len(standings) > 0 {
				var result []LeaderboardRow
				for _, s := range standings {
					result = append(result, LeaderboardRow{
						Name:      s.Nickname,
						Questions: s.Score,
						Accuracy:  80,
						Streak:    0,
					})
				}
				return result
			}
		}
	}

	return MockLeaderboard(period)
}
