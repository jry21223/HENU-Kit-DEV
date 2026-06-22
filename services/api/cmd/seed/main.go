package main

import (
	"log"
	"os"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/database"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Load()
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	if err := database.EnsureExtensions(db); err != nil {
		log.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		log.Fatal(err)
	}

	school := model.School{Name: "河南大学", Slug: "henu", EmailDomains: "henu.edu.cn,stu.henu.edu.cn,example.com", Status: model.StatusPublished}
	firstOrCreate(db, &school, "slug = ?", school.Slug)

	college := model.College{SchoolID: school.ID, Name: "软件学院", Status: model.StatusPublished}
	firstOrCreate(db, &college, "school_id = ? AND name = ?", school.ID, college.Name)

	network := model.Major{SchoolID: school.ID, CollegeID: college.ID, Name: "网络工程", Slug: "network-engineering", Status: model.StatusPublished}
	software := model.Major{SchoolID: school.ID, CollegeID: college.ID, Name: "软件工程", Slug: "software-engineering", Status: model.StatusPublished}
	firstOrCreate(db, &network, "school_id = ? AND slug = ?", school.ID, network.Slug)
	firstOrCreate(db, &software, "school_id = ? AND slug = ?", school.ID, software.Slug)

	courses := []model.Course{
		{SchoolID: school.ID, CollegeID: college.ID, MajorID: network.ID, Grade: "2023级", Name: "离散数学", Slug: "discrete-math", Description: "命题逻辑、集合、关系、图论与树。", ExamScope: "命题逻辑、集合论、关系、图论、树。", Status: model.StatusPublished},
		{SchoolID: school.ID, CollegeID: college.ID, MajorID: network.ID, Grade: "2023级", Name: "概率论与数理统计A", Slug: "probability-statistics-a", Description: "概率、随机变量、估计和检验。", ExamScope: "随机事件、分布、数字特征、参数估计。", Status: model.StatusPublished},
		{SchoolID: school.ID, CollegeID: college.ID, MajorID: software.ID, Grade: "2024级", Name: "高等数学A", Slug: "advanced-math-a", Description: "函数、极限、微积分基础。", ExamScope: "极限、导数、积分、级数。", Status: model.StatusPublished},
	}
	for index := range courses {
		firstOrCreate(db, &courses[index], "major_id = ? AND grade = ? AND slug = ?", courses[index].MajorID, courses[index].Grade, courses[index].Slug)
	}

	material := model.Material{
		CourseID:       courses[0].ID,
		Title:          "离散数学重点知识点讲义",
		Type:           "knowledge_note",
		Description:    "覆盖离散数学期末高频知识点。",
		StorageKey:     "materials/discrete-math/knowledge-note.pdf",
		FileName:       "knowledge-note.pdf",
		PreviewContent: "本讲义整理离散数学期末高频知识点。",
		AccessLevel:    model.MaterialAccessLoginRequired,
		Status:         model.StatusPublished,
	}
	firstOrCreate(db, &material, "course_id = ? AND title = ?", material.CourseID, material.Title)

	question := model.QuizQuestion{
		CourseID:    courses[0].ID,
		Type:        "single_choice",
		Stem:        "命题 p -> q 为假时，p 与 q 的真值分别是？",
		Answer:      "A",
		Explanation: "蕴含命题仅在前件真、后件假时为假。",
		Difficulty:  1,
		Status:      model.StatusPublished,
	}
	firstOrCreate(db, &question, "course_id = ? AND stem = ?", question.CourseID, question.Stem)
	options := []model.QuizOption{
		{QuestionID: question.ID, Label: "A", Content: "p 真，q 假", SortOrder: 1},
		{QuestionID: question.ID, Label: "B", Content: "p 假，q 真", SortOrder: 2},
		{QuestionID: question.ID, Label: "C", Content: "p 真，q 真", SortOrder: 3},
		{QuestionID: question.ID, Label: "D", Content: "p 假，q 假", SortOrder: 4},
	}
	for index := range options {
		firstOrCreate(db, &options[index], "question_id = ? AND label = ?", question.ID, options[index].Label)
	}

	users := []model.User{
		{Email: "admin@example.com", Name: "管理员", Role: model.RoleSuperAdmin, Status: "active", EmailVerified: true},
		{Email: "reviewer@example.com", Name: "审核员", Role: model.RoleReviewer, Status: "active", EmailVerified: true},
		{Email: "creator@example.com", Name: "创作者", Role: model.RoleCreator, Status: "active", EmailVerified: true},
		{Email: "user@example.com", Name: "普通用户", Role: model.RoleUser, Status: "active", EmailVerified: true},
	}
	for index := range users {
		firstOrCreate(db, &users[index], "email = ?", users[index].Email)
	}

	log.New(os.Stdout, "", 0).Println("seed completed")
}

func firstOrCreate(db *gorm.DB, value interface{}, query interface{}, args ...interface{}) {
	if err := db.Where(query, args...).FirstOrCreate(value).Error; err != nil {
		log.Fatal(err)
	}
}
