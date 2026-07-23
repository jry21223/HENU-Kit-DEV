package httpapi

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	"henukit.dev/portal-api/internal/campus"
	"henukit.dev/portal-api/internal/db"
	"henukit.dev/portal-api/internal/food"
	"henukit.dev/portal-api/internal/library"
	"henukit.dev/portal-api/internal/practice"
)

// NewRouter builds the chi router with all Portal API routes.
// Connects to real databases when env vars are set, falls back to mock data.
func NewRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(requestID)
	r.Use(cors)

	// Database connections (nil = mock mode)
	quizcraftConn, err := db.Connect("QUIZCRAFT_DATABASE_URL")
	if err != nil {
		log.Printf("WARN: QuizCraft DB not connected, using mock data: %v", err)
	}
	studyConn, err := db.Connect("STUDY_DATABASE_URL")
	if err != nil {
		log.Printf("WARN: Study API DB not connected, using mock data: %v", err)
	}
	portalConn, err := db.Connect("PORTAL_DATABASE_URL")
	if err != nil {
		log.Printf("WARN: Portal DB not connected, using mock data: %v", err)
	}

	var practiceSource practiceSource
	if quizcraftConn != nil {
		practiceSource.quizcraftDB = practice.NewQuizCraftDB(quizcraftConn)
	}

	var librarySource librarySource
	if studyConn != nil {
		librarySource.studyDB = library.NewStudyDB(studyConn)
	}

	var foodSource foodSource
	var campusSource campusSource
	if portalConn != nil {
		foodSource.portalDB = food.NewPortalDB(portalConn)
		campusSource.portalDB = campus.NewPortalDB(portalConn)
	}

	r.Get("/api/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})

	// Library
	r.Get("/api/v1/library/materials", func(w http.ResponseWriter, r *http.Request) {
		listMaterials(w, r, librarySource)
	})
	r.Get("/api/v1/library/materials/{id}", func(w http.ResponseWriter, r *http.Request) {
		getMaterial(w, r, librarySource)
	})

	// Food
	r.Get("/api/v1/food/posts", func(w http.ResponseWriter, r *http.Request) {
		listFoodPosts(w, r, foodSource)
	})
	r.Get("/api/v1/food/posts/{id}", func(w http.ResponseWriter, r *http.Request) {
		getFoodPost(w, r, foodSource)
	})
	r.Get("/api/v1/food/posts/{id}/comments", func(w http.ResponseWriter, r *http.Request) {
		listFoodComments(w, r, foodSource)
	})

	// Practice
	r.Get("/api/v1/practice/schools", func(w http.ResponseWriter, r *http.Request) {
		listSchools(w, r, practiceSource)
	})
	r.Get("/api/v1/practice/lists/{id}", func(w http.ResponseWriter, r *http.Request) {
		getQuizList(w, r, practiceSource)
	})
	r.Get("/api/v1/practice/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		getLeaderboard(w, r, practiceSource)
	})
	r.Get("/api/v1/practice/stats", func(w http.ResponseWriter, r *http.Request) {
		getUserStats(w, r)
	})

	// Campus
	r.Get("/api/v1/campus/items", func(w http.ResponseWriter, r *http.Request) {
		listCampusItems(w, r, campusSource)
	})
	r.Get("/api/v1/campus/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		getCampusItem(w, r, campusSource)
	})
	r.Get("/api/v1/campus/categories", func(w http.ResponseWriter, r *http.Request) {
		listCategories(w, r)
	})

	return r
}

// --- Data sources (nil = use mock) ---

type practiceSource struct {
	quizcraftDB *practice.QuizCraftDB
}

type librarySource struct {
	studyDB *library.StudyDB
}

type foodSource struct {
	portalDB *food.PortalDB
}

type campusSource struct {
	portalDB *campus.PortalDB
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
		origin := os.Getenv("PORTAL_ORIGIN")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-Id")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Library handlers ---

func listMaterials(w http.ResponseWriter, r *http.Request, src librarySource) {
	var materials []library.Material
	var err error

	if src.studyDB != nil {
		materials, err = src.studyDB.GetMaterials()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "database_error", "detail": err.Error()})
			return
		}
	} else {
		materials = library.MockMaterials()
	}

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

func getMaterial(w http.ResponseWriter, r *http.Request, src librarySource) {
	id := chi.URLParam(r, "id")

	var materials []library.Material
	var err error
	if src.studyDB != nil {
		materials, err = src.studyDB.GetMaterials()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "database_error"})
			return
		}
	} else {
		materials = library.MockMaterials()
	}

	for _, m := range materials {
		if m.ID == id {
			writeJSON(w, 200, map[string]any{"material": m, "request_id": w.Header().Get("X-Request-Id")})
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "not_found"})
}

// --- Food handlers ---

func listFoodPosts(w http.ResponseWriter, r *http.Request, src foodSource) {
	campusFilter := r.URL.Query().Get("campus")

	var posts []food.Post
	var err error

	if src.portalDB != nil {
		posts, err = src.portalDB.GetPosts(campusFilter)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "database_error", "detail": err.Error()})
			return
		}
	} else {
		for _, p := range food.MockPosts() {
			if campusFilter != "" && p.Campus != campusFilter {
				continue
			}
			if p.Hidden {
				continue
			}
			posts = append(posts, p)
		}
	}
	if posts == nil {
		posts = []food.Post{}
	}

	writeJSON(w, 200, map[string]any{"posts": posts, "request_id": w.Header().Get("X-Request-Id")})
}

func getFoodPost(w http.ResponseWriter, r *http.Request, src foodSource) {
	id := chi.URLParam(r, "id")

	if src.portalDB != nil {
		post, err := src.portalDB.GetPost(id)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "database_error"})
			return
		}
		if post == nil {
			writeJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		comments, _ := src.portalDB.GetComments(id)
		if comments == nil {
			comments = []food.Comment{}
		}
		writeJSON(w, 200, map[string]any{"post": post, "comments": comments, "request_id": w.Header().Get("X-Request-Id")})
		return
	}

	// Mock fallback
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

func listFoodComments(w http.ResponseWriter, r *http.Request, src foodSource) {
	id := chi.URLParam(r, "id")

	if src.portalDB != nil {
		comments, err := src.portalDB.GetComments(id)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "database_error"})
			return
		}
		writeJSON(w, 200, map[string]any{"comments": comments, "request_id": w.Header().Get("X-Request-Id")})
		return
	}

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

func listSchools(w http.ResponseWriter, r *http.Request, src practiceSource) {
	if src.quizcraftDB != nil {
		schools, err := src.quizcraftDB.GetSchools()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "database_error", "detail": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"schools": schools, "request_id": w.Header().Get("X-Request-Id")})
		return
	}
	writeJSON(w, 200, map[string]any{"schools": practice.MockSchools(), "request_id": w.Header().Get("X-Request-Id")})
}

func getQuizList(w http.ResponseWriter, r *http.Request, src practiceSource) {
	id := chi.URLParam(r, "id")

	if src.quizcraftDB != nil {
		questions, err := src.quizcraftDB.GetQuestions(id)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "database_error", "detail": err.Error()})
			return
		}
		// Find the list metadata from schools
		schools, _ := src.quizcraftDB.GetSchools()
		for _, s := range schools {
			for _, m := range s.Majors {
				for _, sub := range m.Subjects {
					for _, l := range sub.Lists {
						if l.ID == id {
							writeJSON(w, 200, map[string]any{"list": l, "questions": questions, "request_id": w.Header().Get("X-Request-Id")})
							return
						}
					}
				}
			}
		}
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}

	// Mock fallback
	for _, school := range practice.MockSchools() {
		for _, major := range school.Majors {
			for _, subject := range major.Subjects {
				for _, list := range subject.Lists {
					if list.ID == id {
						questions := practice.MockQuestions(list.PoolKey)
						if list.Count < len(questions) {
							questions = questions[:list.Count]
						}
						writeJSON(w, 200, map[string]any{"list": list, "questions": questions, "request_id": w.Header().Get("X-Request-Id")})
						return
					}
				}
			}
		}
	}
	writeJSON(w, 404, map[string]string{"error": "not_found"})
}

func getLeaderboard(w http.ResponseWriter, r *http.Request, src practiceSource) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "week"
	}

	if src.quizcraftDB != nil {
		rows := src.quizcraftDB.GetLeaderboard(period)
		writeJSON(w, 200, map[string]any{"rows": rows, "request_id": w.Header().Get("X-Request-Id")})
		return
	}
	writeJSON(w, 200, map[string]any{"rows": practice.MockLeaderboard(period), "request_id": w.Header().Get("X-Request-Id")})
}

func getUserStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"totalQuestions": 486, "accuracy": 76, "streakDays": 12, "beatPercent": 83,
		"mastery": []map[string]any{
			{"label": "数据结构", "value": 81}, {"label": "高等数学A", "value": 78},
			{"label": "操作系统", "value": 64}, {"label": "线性代数", "value": 52},
		},
		"weakTop5": []map[string]any{
			{"topic": "特征值与特征向量", "subject": "线性代数", "wrong": 18},
			{"topic": "泰勒展开", "subject": "高等数学A", "wrong": 15},
			{"topic": "页面置换算法", "subject": "操作系统", "wrong": 12},
		},
		"request_id": w.Header().Get("X-Request-Id"),
	})
}

// --- Campus handlers ---

func listCampusItems(w http.ResponseWriter, r *http.Request, src campusSource) {
	typeFilter := r.URL.Query().Get("type")
	categoryFilter := r.URL.Query().Get("category")
	qFilter := r.URL.Query().Get("q")

	if src.portalDB != nil {
		items, err := src.portalDB.GetItems(typeFilter, categoryFilter, qFilter)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "database_error", "detail": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"items": items, "request_id": w.Header().Get("X-Request-Id")})
		return
	}

	// Mock fallback
	items := campus.MockItems()
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

func getCampusItem(w http.ResponseWriter, r *http.Request, src campusSource) {
	id := chi.URLParam(r, "id")

	if src.portalDB != nil {
		item, err := src.portalDB.GetItem(id)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "database_error"})
			return
		}
		if item == nil {
			writeJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		msgs, _ := src.portalDB.GetMessages(id)
		if msgs == nil {
			msgs = []campus.DealMessage{}
		}
		writeJSON(w, 200, map[string]any{"item": item, "messages": msgs, "request_id": w.Header().Get("X-Request-Id")})
		return
	}

	// Mock fallback
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
