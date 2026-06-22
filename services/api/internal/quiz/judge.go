package quiz

import (
	"regexp"
	"sort"
	"strings"
)

const (
	TypeSingleChoice   = "single_choice"
	TypeMultipleChoice = "multiple_choice"
	TypeTrueFalse      = "true_false"
	TypeFillBlank      = "fill_blank"
	TypeShortAnswer    = "short_answer"
)

type JudgeResult struct {
	IsCorrect bool    `json:"isCorrect"`
	Score     float64 `json:"score"`
}

func Judge(questionType string, expected string, submitted string) JudgeResult {
	switch questionType {
	case TypeSingleChoice, TypeTrueFalse:
		return exactJudge(expected, submitted)
	case TypeMultipleChoice:
		return setJudge(expected, submitted)
	case TypeFillBlank:
		return exactJudge(normalizeBlank(expected), normalizeBlank(submitted))
	case TypeShortAnswer:
		return shortAnswerJudge(expected, submitted)
	default:
		return JudgeResult{IsCorrect: false, Score: 0}
	}
}

func exactJudge(expected string, submitted string) JudgeResult {
	correct := strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(submitted))
	if correct {
		return JudgeResult{IsCorrect: true, Score: 1}
	}
	return JudgeResult{IsCorrect: false, Score: 0}
}

func setJudge(expected string, submitted string) JudgeResult {
	expectedSet := splitAnswerSet(expected)
	submittedSet := splitAnswerSet(submitted)
	if len(expectedSet) == 0 || len(expectedSet) != len(submittedSet) {
		return JudgeResult{IsCorrect: false, Score: 0}
	}
	for index := range expectedSet {
		if expectedSet[index] != submittedSet[index] {
			return JudgeResult{IsCorrect: false, Score: 0}
		}
	}
	return JudgeResult{IsCorrect: true, Score: 1}
}

func shortAnswerJudge(expected string, submitted string) JudgeResult {
	expected = strings.TrimSpace(expected)
	submitted = strings.TrimSpace(submitted)
	if expected == "" || submitted == "" {
		return JudgeResult{IsCorrect: false, Score: 0}
	}
	if strings.EqualFold(expected, submitted) {
		return JudgeResult{IsCorrect: true, Score: 1}
	}
	return JudgeResult{IsCorrect: false, Score: 0}
}

func splitAnswerSet(value string) []string {
	parts := regexp.MustCompile(`[,\s;，；、]+`).Split(value, -1)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.ToUpper(strings.TrimSpace(part))
		if item != "" {
			result = append(result, item)
		}
	}
	sort.Strings(result)
	return result
}

func normalizeBlank(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	value = strings.NewReplacer(
		"　", "",
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
	).Replace(value)
	return value
}
