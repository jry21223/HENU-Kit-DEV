package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"henukit.dev/portal-api/internal/campus"
	"henukit.dev/portal-api/internal/db"
	"henukit.dev/portal-api/internal/food"
	"henukit.dev/portal-api/internal/library"
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

	studyConn, err := db.Connect("STUDY_DATABASE_URL")
	if err != nil {
		return nil, fmt.Errorf("study db: %w", err)
	}
	portalConn, err := db.Connect("PORTAL_DATABASE_URL")
	if err != nil {
		return nil, fmt.Errorf("portal db: %w", err)
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
	r.Get("/api/v1/food/posts/{id}/images/{position}", func(w http.ResponseWriter, req *http.Request) {
		getFoodPostImage(w, req, foodSource)
	})

	// Practice reads were removed by ADR-0036: the Portal read path now runs
	// through Portal Gateway's exact routes to the QuizCraft Go core contract.
	// portal-api no longer connects to the quizcraft database.

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

	if src.studyDB != nil {
		material, err := src.studyDB.GetMaterialByID(id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "内容不存在或已下架"})
				return
			}
			writeServiceUnavailable(w, "study_database_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"material": material, "request_id": requestIDOf(w)})
		return
	}

	if mode == db.ModeLive {
		writeServiceUnavailable(w, "study_database_unavailable", "STUDY_DATABASE_URL not connected")
		return
	}

	for _, m := range library.MockMaterials() {
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

// getFoodPostImage serves a photo stored alongside its post.
//
// Photos are immutable once written — an edit inserts a new position — so the
// response is cacheable indefinitely and carries the content hash as its ETag
// for revalidation. Mock mode has no stored photos and simply 404s.
func getFoodPostImage(w http.ResponseWriter, r *http.Request, src foodSource) {
	if src.portalDB == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "图片不存在"})
		return
	}

	position, err := strconv.Atoi(chi.URLParam(r, "position"))
	if err != nil || position < 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "图片不存在"})
		return
	}

	image, err := src.portalDB.GetImage(chi.URLParam(r, "id"), position)
	if err != nil {
		writeServiceUnavailable(w, "portal_database_error", err.Error())
		return
	}
	if image == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "图片不存在"})
		return
	}

	etag := `"` + image.SHA256 + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", image.ContentType)
	// Photos are decorative user uploads: never let a browser interpret one as
	// markup, and do not offer it as a download.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")

	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, image.SHA256) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(image.Bytes)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(image.Bytes); err != nil {
		log.Printf("food image write failed: %v", err)
	}
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
// Removed with ADR-0036: practice reads are owned by the QuizCraft Go core
// and reach the browser through Portal Gateway exact routes. portal-api no
// longer serves /api/v1/practice/{banks,schools,lists,leaderboard,stats}.

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
