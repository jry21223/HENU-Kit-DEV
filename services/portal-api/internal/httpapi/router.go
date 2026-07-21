package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"henukit.dev/portal-api/internal/campus"
	"henukit.dev/portal-api/internal/food"
	"henukit.dev/portal-api/internal/library"
	"henukit.dev/portal-api/internal/practice"
)

// NewRouter builds the chi router with all Portal API routes.
func NewRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(requestID)
	r.Use(cors)

	r.Get("/api/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})

	// Library
	r.Get("/api/v1/library/materials", listMaterials)
	r.Get("/api/v1/library/materials/{id}", getMaterial)

	// Food
	r.Get("/api/v1/food/posts", listFoodPosts)
	r.Get("/api/v1/food/posts/{id}", getFoodPost)
	r.Get("/api/v1/food/posts/{id}/comments", listFoodComments)

	// Practice
	r.Get("/api/v1/practice/schools", listSchools)
	r.Get("/api/v1/practice/lists/{id}", getQuizList)
	r.Get("/api/v1/practice/leaderboard", getLeaderboard)
	r.Get("/api/v1/practice/stats", getUserStats)

	// Campus
	r.Get("/api/v1/campus/items", listCampusItems)
	r.Get("/api/v1/campus/items/{id}", getCampusItem)
	r.Get("/api/v1/campus/categories", listCategories)

	return r
}

// --- Middleware ---

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = "req-" + strings.ReplaceAll(r.URL.Path, "/", "-")
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Library handlers ---

func listMaterials(w http.ResponseWriter, r *http.Request) {
	materials := library.MockMaterials()

	// Apply filters
	typeFilter := r.URL.Query().Get("type")
	subjectFilter := r.URL.Query().Get("subject")
	qFilter := r.URL.Query().Get("q")

	var filtered []library.Material
	for _, m := range materials {
		if typeFilter != "" && m.Type != typeFilter {
			continue
		}
		if subjectFilter != "" && m.Subject != subjectFilter {
			continue
		}
		if qFilter != "" && !strings.Contains(m.Title, qFilter) && !strings.Contains(m.Intro, qFilter) {
			continue
		}
		filtered = append(filtered, m)
	}
	if filtered == nil {
		filtered = []library.Material{}
	}

	writeJSON(w, 200, map[string]any{"materials": filtered, "request_id": w.Header().Get("X-Request-Id")})
}

func getMaterial(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	for _, m := range library.MockMaterials() {
		if m.ID == id {
			writeJSON(w, 200, map[string]any{"material": m, "request_id": w.Header().Get("X-Request-Id")})
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "not_found"})
}

// --- Food handlers ---

func listFoodPosts(w http.ResponseWriter, r *http.Request) {
	posts := food.MockPosts()
	campusFilter := r.URL.Query().Get("campus")

	var filtered []food.Post
	for _, p := range posts {
		if campusFilter != "" && p.Campus != campusFilter {
			continue
		}
		if p.Hidden {
			continue
		}
		filtered = append(filtered, p)
	}
	if filtered == nil {
		filtered = []food.Post{}
	}

	writeJSON(w, 200, map[string]any{"posts": filtered, "request_id": w.Header().Get("X-Request-Id")})
}

func getFoodPost(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	comments := food.MockComments()

	for _, p := range food.MockPosts() {
		if p.ID == id {
			var postComments []food.Comment
			for _, c := range comments {
				if c.PostID == id {
					postComments = append(postComments, c)
				}
			}
			if postComments == nil {
				postComments = []food.Comment{}
			}
			writeJSON(w, 200, map[string]any{"post": p, "comments": postComments, "request_id": w.Header().Get("X-Request-Id")})
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "not_found"})
}

func listFoodComments(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var result []food.Comment
	for _, c := range food.MockComments() {
		if c.PostID == id {
			result = append(result, c)
		}
	}
	if result == nil {
		result = []food.Comment{}
	}
	writeJSON(w, 200, map[string]any{"comments": result, "request_id": w.Header().Get("X-Request-Id")})
}

// --- Practice handlers ---

func listSchools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"schools": practice.MockSchools(), "request_id": w.Header().Get("X-Request-Id")})
}

func getQuizList(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	for _, school := range practice.MockSchools() {
		for _, major := range school.Majors {
			for _, subject := range major.Subjects {
				for _, list := range subject.Lists {
					if list.ID == id {
						questions := practice.MockQuestions(list.PoolKey)
						if list.Count < len(questions) {
							questions = questions[:list.Count]
						}
						writeJSON(w, 200, map[string]any{
							"list":      list,
							"questions": questions,
							"request_id": w.Header().Get("X-Request-Id"),
						})
						return
					}
				}
			}
		}
	}
	writeJSON(w, 404, map[string]string{"error": "not_found"})
}

func getLeaderboard(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "week"
	}
	rows := practice.MockLeaderboard(period)
	writeJSON(w, 200, map[string]any{"rows": rows, "request_id": w.Header().Get("X-Request-Id")})
}

func getUserStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"totalQuestions": 486,
		"accuracy":       76,
		"streakDays":     12,
		"beatPercent":    83,
		"mastery": []map[string]any{
			{"label": "数据结构", "value": 81},
			{"label": "高等数学A", "value": 78},
			{"label": "操作系统", "value": 64},
			{"label": "线性代数", "value": 52},
		},
		"weakTop5": []map[string]any{
			{"topic": "特征值与特征向量", "subject": "线性代数", "wrong": 18},
			{"topic": "泰勒展开", "subject": "高等数学A", "wrong": 15},
			{"topic": "页面置换算法", "subject": "操作系统", "wrong": 12},
			{"topic": "平衡二叉树", "subject": "数据结构", "wrong": 11},
			{"topic": "微分中值定理", "subject": "高等数学A", "wrong": 9},
		},
		"request_id": w.Header().Get("X-Request-Id"),
	})
}

// --- Campus handlers ---

func listCampusItems(w http.ResponseWriter, r *http.Request) {
	items := campus.MockItems()
	typeFilter := r.URL.Query().Get("type")
	categoryFilter := r.URL.Query().Get("category")
	qFilter := r.URL.Query().Get("q")

	var filtered []campus.Item
	for _, it := range items {
		if typeFilter != "" && it.Type != typeFilter {
			continue
		}
		if categoryFilter != "" && it.Category != categoryFilter {
			continue
		}
		if qFilter != "" && !strings.Contains(it.Title, qFilter) && !strings.Contains(it.Desc, qFilter) {
			continue
		}
		if it.Status == "hidden" {
			continue
		}
		filtered = append(filtered, it)
	}
	if filtered == nil {
		filtered = []campus.Item{}
	}

	writeJSON(w, 200, map[string]any{"items": filtered, "request_id": w.Header().Get("X-Request-Id")})
}

func getCampusItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	messages := campus.MockMessages()

	for _, it := range campus.MockItems() {
		if it.ID == id {
			var itemMsgs []campus.DealMessage
			for _, m := range messages {
				if m.ItemID == id {
					itemMsgs = append(itemMsgs, m)
				}
			}
			if itemMsgs == nil {
				itemMsgs = []campus.DealMessage{}
			}
			writeJSON(w, 200, map[string]any{"item": it, "messages": itemMsgs, "request_id": w.Header().Get("X-Request-Id")})
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "not_found"})
}

func listCategories(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"categories": campus.MockCategories(), "request_id": w.Header().Get("X-Request-Id")})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
