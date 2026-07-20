package quizcraft

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

type legacyRankingEntry struct {
	Rank    int    `json:"rank"`
	Name    string `json:"name"`
	Correct int64  `json:"correct"`
	Total   int64  `json:"total"`
}

func (service *practiceHTTP) legacyRanking(writer http.ResponseWriter, request *http.Request) {
	var raw []byte
	var capturedAt time.Time
	var contentSHA256 string
	err := service.database.QueryRow(request.Context(), `SELECT standings,captured_at,content_sha256 FROM quizcraft_legacy_ranking_snapshots ORDER BY captured_at DESC,id DESC LIMIT 1`).Scan(&raw, &capturedAt, &contentSHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: map[string]any{"captured_at": nil, "content_sha256": "", "entries": []legacyRankingEntry{}}})
		return
	}
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft legacy ranking is temporarily unavailable")
		return
	}
	entries, err := publicLegacyRankingEntries(raw)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "invalid_legacy_snapshot", "QuizCraft legacy ranking snapshot is invalid")
		return
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: map[string]any{"captured_at": capturedAt, "content_sha256": contentSHA256, "entries": entries}})
}

func publicLegacyRankingEntries(raw []byte) ([]legacyRankingEntry, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0)
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if entry, ok := item.(map[string]any); ok {
				items = append(items, entry)
			}
		}
	case map[string]any:
		if users, ok := typed["users"].(map[string]any); ok {
			keys := make([]string, 0, len(users))
			for key := range users {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if entry, ok := users[key].(map[string]any); ok {
					items = append(items, entry)
				}
			}
		}
	default:
		return nil, errors.New("legacy ranking snapshot must be an array or users object")
	}
	entries := make([]legacyRankingEntry, 0, len(items))
	for _, item := range items {
		name, _ := item["name"].(string)
		correct, correctOK := jsonNumberInt64(item["correct"])
		total, totalOK := jsonNumberInt64(item["total"])
		if name == "" || !correctOK || !totalOK || correct < 0 || total < correct {
			return nil, errors.New("legacy ranking entry is invalid")
		}
		entries = append(entries, legacyRankingEntry{Name: name, Correct: correct, Total: total})
	}
	sort.SliceStable(entries, func(left, right int) bool {
		if entries[left].Correct != entries[right].Correct {
			return entries[left].Correct > entries[right].Correct
		}
		leftAccuracy := float64(entries[left].Correct) / float64(max(entries[left].Total, 1))
		rightAccuracy := float64(entries[right].Correct) / float64(max(entries[right].Total, 1))
		if leftAccuracy != rightAccuracy {
			return leftAccuracy > rightAccuracy
		}
		return entries[left].Name < entries[right].Name
	})
	for index := range entries {
		entries[index].Rank = index + 1
	}
	return entries, nil
}

func jsonNumberInt64(value any) (int64, bool) {
	number, ok := value.(float64)
	if !ok || number != float64(int64(number)) {
		return 0, false
	}
	return int64(number), true
}
