package httpapi

import (
	"encoding/json"
	"fmt"
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
// In live mode missing DSN / connect failures return an error (caller should fatal).
// Mock data is only used when mode=mock.
func NewRouter() (http.Handler, error) {
	mode := db.Mode()

	if mode == db.ModeLive {
		origin := strings.TrimSpace(os.Getenv("PORTAL_ORIGIN"))
		if origin == "" || origin == "*" {
			return nil, fmt.Errorf("PORTAL_ORIGIN is required in live mode and must not be *")
		}
	}

	quizcraftConn, err := db.Connect("QUIZCRAFT_DATABASE_URL")
	if err != nil {
		return nil, fmt.Errorf("quizcraft db: %w", err)
	}
	studyConn, err := db.Connect("STUDY_DATABASE_URL")
	if err != nil {
		return nil, fmt.Errorf("study db: %w", err)
	}
	portalConn, err := db.Connect("PORTAL_DATABASE_URL")
	if err != nil {
		return nil, fmt.Errorf("portal db: %w", err)
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

	r := chi.NewRouter()
	r.Use(requestID)
	r.Use(cors(mode))

	r.Get("/api/v1/healthz", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": mode})
	})

	// Library
	r.Get("/api/v1/library/courses", func(w http.ResponseWriter, req *http.Request) {
		listCourses(w, req, librarySource, mode)
	})
	r.Get("/api/v1/library/materials", func(w http.ResponseWriter, req *http.Request) {
		listMaterials(w, req, librarySource, mode)
	})
	r.Get("/api/v1/library/materials/{id}", func(w http.ResponseWriter, req *http.Request) {
		getMaterial(w, req, librarySource, mode)
	})

	// Food
	r.Get("/api/v1/food/venues", func(w http.ResponseWriter, req *http.Request) {
		listFoodVenues(w, req, foodSource, mode)
	})
	r.Get("/api/v1/food/posts", func(w http.ResponseWriter, req *http.Request) {
		listFoodPosts(w, req, foodSource, mode)
	})
	r.Get("/api/v1/food/posts/{id}", func(w http.ResponseWriter, req *http.Request) {
		getFoodPost(w, req, foodSource, mode)
	})
	r.Get("/api/v1/food/posts/{id}/comments", func(w http.ResponseWriter, req *http.Request) {
		listFoodComments(w, req, foodSource, mode)
	})

	// Practice
	r.Get("/api/v1/practice/banks", func(w http.ResponseWriter, req *http.Request) {
		listBanks(w, req, practiceSource, mode)
	})
	r.Get("/api/v1/practice/schools", func(w http.ResponseWriter, req *http.Request) {
		listSchools(w, req, practiceSource, mode)
	})
	r.Get("/api/v1/practice/lists/{id}", func(w http.ResponseWriter, req *http.Request) {
		getQuizList(w, req, practiceSource, mode)
	})
	r.Get("/api/v1/practice/leaderboard", func(w http.ResponseWriter, req *http.Request) {
		getLeaderboard(w, req, practiceSource, mode)
	})
	r.Get("/api/v1/practice/stats", func(w http.ResponseWriter, req *http.Request) {
		getUserStats(w, req, mode)
	})

	// Campus
	r.Get("/api/v1/campus/items", func(w http.ResponseWriter, req *http.Request) {
		listCampusItems(w, req, campusSource, mode)
	})
	r.Get("/api/v1/campus/items/{id}", func(w http.ResponseWriter, req *http.Request) {
		getCampusItem(w, req, campusSource, mode)
	})
	r.Get("/api/v1/campus/categories", func(w http.ResponseWriter, req *http.Request) {
		listCategories(w, req, campusSource, mode)
	})

	// Notices — no notice DB wired; live returns empty real list, never fakes.
	r.Get("/api/v1/notices", func(w http.ResponseWriter, req *http.Request) {
		listNotices(w, req)
	})

	return r, nil
}

// --- Data sources (nil = mock only when mode=mock) ---

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

// cors never sets Access-Control-Allow-Origin: * together with credentials.
// Live mode requires PORTAL_ORIGIN (validated at router construction).
func cors(mode string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(os.Getenv("PORTAL_ORIGIN"))
			switch {
			case origin != "" && origin != "*":
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			case mode != db.ModeLive:
				// Reflect request origin in non-live mode; never emit * + credentials.
				if reqOrigin := r.Header.Get("Origin"); reqOrigin != "" {
					w.Header().Set("Access-Control-Allow-Origin", reqOrigin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Vary", "Origin")
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-Id")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- Library handlers ---

func listCourses(w http.ResponseWriter, r *http.Request, src librarySource, mode string) {
	if src.studyDB != nil {
		courses, err := src.studyDB.GetCourses()
		if err != nil {
			writeServiceUnavailable(w, "study_database_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"courses": courses, "request_id": requestIDOf(w)})
		return
	}
	if mode == db.ModeLive {
		writeServiceUnavailable(w, "study_database_unavailable", "STUDY_DATABASE_URL not connected")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"courses": library.MockCourses(), "request_id": requestIDOf(w)})
}

func listMaterials(w http.ResponseWriter, r *http.Request, src librarySource, mode string) {
	var materials []library.Material
	var err error

	if src.studyDB != nil {
		materials, err = src.studyDB.GetMaterials()
		if err != nil {
			writeServiceUnavailable(w, "study_database_error", err.Error())
			return
		}
	} else if mode == db.ModeLive {
		writeServiceUnavailable(w, "study_database_unavailable", "STUDY_DATABASE_URL not connected")
		return
	} else {
		materials = library.MockMaterials()
	}

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

	writeJSON(w, http.StatusOK, map[string]any{"materials": filtered, "request_id": requestIDOf(w)})
}

func getMaterial(w http.ResponseWriter, r *http.Request, src librarySource, mode string) {
	id := chi.URLParam(r, "id")

	var materials []library.Material
	if src.studyDB != nil {
		var err error
		materials, err = src.studyDB.GetMaterials()
		if err != nil {
			writeServiceUnavailable(w, "study_database_error", err.Error())
			return
		}
	} else if mode == db.ModeLive {
		writeServiceUnavailable(w, "study_database_unavailable", "STUDY_DATABASE_URL not connected")
		return
	} else {
		materials = library.MockMaterials()
	}

	for _, m := range materials {
		if m.ID == id {
			writeJSON(w, http.StatusOK, map[string]any{"material": m, "request_id": requestIDOf(w)})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "内容不存在或已下架"})
}

// --- Food handlers ---

func listFoodVenues(w http.ResponseWriter, r *http.Request, src foodSource, mode string) {
	campusFilter := r.URL.Query().Get("campus")

	if src.portalDB != nil {
		venues, err := src.portalDB.GetVenues(campusFilter)
		if err != nil {
			writeServiceUnavailable(w, "portal_database_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"campus":     campusFilter,
			"venues":     venues,
			"request_id": requestIDOf(w),
		})
		return
	}
	if mode == db.ModeLive {
		writeServiceUnavailable(w, "portal_database_unavailable", "PORTAL_DATABASE_URL not connected")
		return
	}
	// Mock mode without DB: empty venues (do not invent shops from mock posts as venues).
	writeJSON(w, http.StatusOK, map[string]any{
		"campus":     campusFilter,
		"venues":     []food.Venue{},
		"request_id": requestIDOf(w),
	})
}

func listFoodPosts(w http.ResponseWriter, r *http.Request, src foodSource, mode string) {
	campusFilter := r.URL.Query().Get("campus")

	var posts []food.Post
	var err error

	if src.portalDB != nil {
		posts, err = src.portalDB.GetPosts(campusFilter)
		if err != nil {
			writeServiceUnavailable(w, "portal_database_error", err.Error())
			return
		}
	} else if mode == db.ModeLive {
		writeServiceUnavailable(w, "portal_database_unavailable", "PORTAL_DATABASE_URL not connected")
		return
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

	writeJSON(w, http.StatusOK, map[string]any{"posts": posts, "request_id": requestIDOf(w)})
}

func getFoodPost(w http.ResponseWriter, r *http.Request, src foodSource, mode string) {
	id := chi.URLParam(r, "id")

	if src.portalDB != nil {
		post, err := src.portalDB.GetPost(id)
		if err != nil {
			writeServiceUnavailable(w, "portal_database_error", err.Error())
			return
		}
		if post == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "内容不存在或已下架"})
			return
		}
		comments, err := src.portalDB.GetComments(id)
		if err != nil {
			writeServiceUnavailable(w, "portal_database_error", err.Error())
			return
		}
		if comments == nil {
			comments = []food.Comment{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"post": post, "comments": comments, "request_id": requestIDOf(w)})
		return
	}

	if mode == db.ModeLive {
		writeServiceUnavailable(w, "portal_database_unavailable", "PORTAL_DATABASE_URL not connected")
		return
	}

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
			writeJSON(w, http.StatusOK, map[string]any{"post": p, "comments": postComments, "request_id": requestIDOf(w)})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "内容不存在或已下架"})
}

func listFoodComments(w http.ResponseWriter, r *http.Request, src foodSource, mode string) {
	id := chi.URLParam(r, "id")

	if src.portalDB != nil {
		comments, err := src.portalDB.GetComments(id)
		if err != nil {
			writeServiceUnavailable(w, "portal_database_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"comments": comments, "request_id": requestIDOf(w)})
		return
	}

	if mode == db.ModeLive {
		writeServiceUnavailable(w, "portal_database_unavailable", "PORTAL_DATABASE_URL not connected")
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
	writeJSON(w, http.StatusOK, map[string]any{"comments": result, "request_id": requestIDOf(w)})
}

// --- Practice handlers ---

func listBanks(w http.ResponseWriter, r *http.Request, src practiceSource, mode string) {
	if src.quizcraftDB != nil {
		banks, err := src.quizcraftDB.GetBanks()
		if err != nil {
			writeServiceUnavailable(w, "quizcraft_database_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"banks": banks, "request_id": requestIDOf(w)})
		return
	}
	if mode == db.ModeLive {
		writeServiceUnavailable(w, "quizcraft_database_unavailable", "QUIZCRAFT_DATABASE_URL not connected")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"banks": practice.MockBanks(), "request_id": requestIDOf(w)})
}

func listSchools(w http.ResponseWriter, r *http.Request, src practiceSource, mode string) {
	if src.quizcraftDB != nil {
		schools, err := src.quizcraftDB.GetSchools()
		if err != nil {
			writeServiceUnavailable(w, "quizcraft_database_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"schools": schools, "request_id": requestIDOf(w)})
		return
	}
	if mode == db.ModeLive {
		writeServiceUnavailable(w, "quizcraft_database_unavailable", "QUIZCRAFT_DATABASE_URL not connected")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schools": practice.MockSchools(), "request_id": requestIDOf(w)})
}

func getQuizList(w http.ResponseWriter, r *http.Request, src practiceSource, mode string) {
	id := chi.URLParam(r, "id")

	if src.quizcraftDB != nil {
		questions, err := src.quizcraftDB.GetQuestions(id)
		if err != nil {
			writeServiceUnavailable(w, "quizcraft_database_error", err.Error())
			return
		}
		schools, err := src.quizcraftDB.GetSchools()
		if err != nil {
			writeServiceUnavailable(w, "quizcraft_database_error", err.Error())
			return
		}
		for _, s := range schools {
			for _, m := range s.Majors {
				for _, sub := range m.Subjects {
					for _, l := range sub.Lists {
						if l.ID == id {
							writeJSON(w, http.StatusOK, map[string]any{"list": l, "questions": questions, "request_id": requestIDOf(w)})
							return
						}
					}
				}
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "内容不存在或已下架"})
		return
	}

	if mode == db.ModeLive {
		writeServiceUnavailable(w, "quizcraft_database_unavailable", "QUIZCRAFT_DATABASE_URL not connected")
		return
	}

	for _, school := range practice.MockSchools() {
		for _, major := range school.Majors {
			for _, subject := range major.Subjects {
				for _, list := range subject.Lists {
					if list.ID == id {
						questions := practice.MockQuestions(list.PoolKey)
						if list.Count < len(questions) {
							questions = questions[:list.Count]
						}
						writeJSON(w, http.StatusOK, map[string]any{"list": list, "questions": questions, "request_id": requestIDOf(w)})
						return
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "内容不存在或已下架"})
}

func getLeaderboard(w http.ResponseWriter, r *http.Request, src practiceSource, mode string) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "week"
	}

	if src.quizcraftDB != nil {
		rows, err := src.quizcraftDB.GetLeaderboard(period)
		if err != nil {
			writeServiceUnavailable(w, "quizcraft_database_error", err.Error())
			return
		}
		if rows == nil {
			rows = []practice.LeaderboardRow{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "request_id": requestIDOf(w)})
		return
	}
	if mode == db.ModeLive {
		writeServiceUnavailable(w, "quizcraft_database_unavailable", "QUIZCRAFT_DATABASE_URL not connected")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": practice.MockLeaderboard(period), "request_id": requestIDOf(w)})
}

func getUserStats(w http.ResponseWriter, r *http.Request, mode string) {
	// No real user-stats source is wired yet. Live must not invent metrics.
	if mode == db.ModeLive {
		log.Printf("portal-api user stats source is not configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":      "stats_unavailable",
			"message":    "学习统计暂时不可用，请稍后再来",
			"request_id": requestIDOf(w),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
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
		"request_id": requestIDOf(w),
	})
}

// --- Campus handlers ---

func listCampusItems(w http.ResponseWriter, r *http.Request, src campusSource, mode string) {
	typeFilter := r.URL.Query().Get("type")
	categoryFilter := r.URL.Query().Get("category")
	qFilter := r.URL.Query().Get("q")

	if src.portalDB != nil {
		items, err := src.portalDB.GetItems(typeFilter, categoryFilter, qFilter)
		if err != nil {
			writeServiceUnavailable(w, "portal_database_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "request_id": requestIDOf(w)})
		return
	}

	if mode == db.ModeLive {
		writeServiceUnavailable(w, "portal_database_unavailable", "PORTAL_DATABASE_URL not connected")
		return
	}

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
	writeJSON(w, http.StatusOK, map[string]any{"items": filtered, "request_id": requestIDOf(w)})
}

func getCampusItem(w http.ResponseWriter, r *http.Request, src campusSource, mode string) {
	id := chi.URLParam(r, "id")

	if src.portalDB != nil {
		item, err := src.portalDB.GetItem(id)
		if err != nil {
			writeServiceUnavailable(w, "portal_database_error", err.Error())
			return
		}
		if item == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "内容不存在或已下架"})
			return
		}
		msgs, err := src.portalDB.GetMessages(id)
		if err != nil {
			writeServiceUnavailable(w, "portal_database_error", err.Error())
			return
		}
		if msgs == nil {
			msgs = []campus.DealMessage{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item, "messages": msgs, "request_id": requestIDOf(w)})
		return
	}

	if mode == db.ModeLive {
		writeServiceUnavailable(w, "portal_database_unavailable", "PORTAL_DATABASE_URL not connected")
		return
	}

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
			writeJSON(w, http.StatusOK, map[string]any{"item": it, "messages": itemMsgs, "request_id": requestIDOf(w)})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "内容不存在或已下架"})
}

func listCategories(w http.ResponseWriter, r *http.Request, src campusSource, mode string) {
	// No categories table is wired yet.
	// Live without portal DB: 503. Live with DB connected: empty real list (never fake taxonomy).
	// Mock mode: fixtures for local UI work.
	if mode == db.ModeLive {
		if src.portalDB == nil {
			writeServiceUnavailable(w, "portal_database_unavailable", "PORTAL_DATABASE_URL not connected")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"categories": []campus.Category{},
			"request_id": requestIDOf(w),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": campus.MockCategories(), "request_id": requestIDOf(w)})
}

func listNotices(w http.ResponseWriter, r *http.Request) {
	// Notice product DB is not wired into portal-api. Always return a real empty list.
	writeJSON(w, http.StatusOK, map[string]any{
		"notices":    []any{},
		"request_id": requestIDOf(w),
	})
}

// --- Helpers ---

func requestIDOf(w http.ResponseWriter) string {
	return w.Header().Get("X-Request-Id")
}

func writeServiceUnavailable(w http.ResponseWriter, code, detail string) {
	// Internal detail stays in the log; the client only ever sees a friendly message.
	log.Printf("portal-api service unavailable code=%s detail=%s request_id=%s", code, detail, requestIDOf(w))
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{
		"error":      code,
		"message":    "服务暂时不可用，请稍后再来",
		"request_id": requestIDOf(w),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
