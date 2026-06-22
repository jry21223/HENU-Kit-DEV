package main

import (
	"log"
	"os"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/database"
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

	school := seedSchool(db)
	college := seedCollege(db, school)
	network := seedMajor(db, school, college, "网络工程", "network-engineering")
	software := seedMajor(db, school, college, "软件工程", "software-engineering")
	users := seedUsers(db, school, network)
	courses := seedCourses(db, school, college, network, software)
	seedMaterials(db, courses)
	seedCoursePackages(db, courses)
	seedQuestions(db, courses)
	seedWikiAndCommunity(db, users, courses)
	seedPointsAndMembership(db, users)
	seedAIAndSystem(db, users, courses)

	log.New(os.Stdout, "", 0).Println("seed completed")
}

func seedSchool(db *gorm.DB) model.School {
	school := model.School{
		Name:         "河南大学",
		Slug:         "henu",
		EmailDomains: "henu.edu.cn,stu.henu.edu.cn,example.com",
		Status:       model.StatusPublished,
	}
	firstOrCreate(db, &school, "slug = ?", school.Slug)
	db.Model(&school).Updates(map[string]interface{}{
		"name":          school.Name,
		"email_domains": school.EmailDomains,
		"status":        school.Status,
	})
	return school
}

func seedCollege(db *gorm.DB, school model.School) model.College {
	college := model.College{SchoolID: school.ID, Name: "软件学院", Status: model.StatusPublished}
	firstOrCreate(db, &college, "school_id = ? AND name = ?", school.ID, college.Name)
	return college
}

func seedMajor(db *gorm.DB, school model.School, college model.College, name string, slug string) model.Major {
	major := model.Major{SchoolID: school.ID, CollegeID: college.ID, Name: name, Slug: slug, Status: model.StatusPublished}
	firstOrCreate(db, &major, "school_id = ? AND slug = ?", school.ID, slug)
	db.Model(&major).Updates(map[string]interface{}{
		"college_id": college.ID,
		"name":       name,
		"status":     model.StatusPublished,
	})
	return major
}

func seedUsers(db *gorm.DB, school model.School, major model.Major) map[string]model.User {
	users := []model.User{
		{Email: "admin@example.com", Name: "超级管理员", Role: model.RoleSuperAdmin, Status: "active", SchoolID: &school.ID, MajorID: &major.ID, Grade: "2023级", EmailVerified: true, PointsBalance: 2000},
		{Email: "reviewer@example.com", Name: "审核员", Role: model.RoleReviewer, Status: "active", SchoolID: &school.ID, MajorID: &major.ID, Grade: "2023级", EmailVerified: true, PointsBalance: 800},
		{Email: "creator@example.com", Name: "创作者", Role: model.RoleCreator, Status: "active", SchoolID: &school.ID, MajorID: &major.ID, Grade: "2023级", EmailVerified: true, PointsBalance: 1200},
		{Email: "user@example.com", Name: "普通用户", Role: model.RoleUser, Status: "active", SchoolID: &school.ID, MajorID: &major.ID, Grade: "2023级", EmailVerified: true, PointsBalance: 300},
	}
	result := map[string]model.User{}
	for index := range users {
		firstOrCreate(db, &users[index], "email = ?", users[index].Email)
		db.Model(&users[index]).Updates(map[string]interface{}{
			"name":           users[index].Name,
			"role":           users[index].Role,
			"status":         users[index].Status,
			"school_id":      users[index].SchoolID,
			"major_id":       users[index].MajorID,
			"grade":          users[index].Grade,
			"email_verified": true,
			"points_balance": users[index].PointsBalance,
		})
		result[users[index].Role] = users[index]
	}
	return result
}

func seedCourses(db *gorm.DB, school model.School, college model.College, network model.Major, software model.Major) map[string]model.Course {
	definitions := []model.Course{
		{SchoolID: school.ID, CollegeID: college.ID, MajorID: network.ID, Grade: "2023级", Name: "离散数学", Slug: "discrete-math", Description: "命题逻辑、集合、关系、图论与树的期末复习课程。", ExamScope: "命题逻辑、集合论、关系、图论、树。", Status: model.StatusPublished},
		{SchoolID: school.ID, CollegeID: college.ID, MajorID: network.ID, Grade: "2023级", Name: "概率论与数理统计A", Slug: "probability-statistics-a", Description: "概率、随机变量、估计和检验基础。", ExamScope: "随机事件、分布、数字特征、参数估计。", Status: model.StatusPublished},
		{SchoolID: school.ID, CollegeID: college.ID, MajorID: network.ID, Grade: "2024级", Name: "大学物理", Slug: "college-physics", Description: "力学、电磁学与基础实验复习。", ExamScope: "质点力学、刚体、电磁学重点。", Status: model.StatusPublished},
		{SchoolID: school.ID, CollegeID: college.ID, MajorID: software.ID, Grade: "2024级", Name: "高等数学A", Slug: "advanced-math-a", Description: "函数、极限、微积分与级数基础。", ExamScope: "极限、导数、积分、级数。", Status: model.StatusPublished},
		{SchoolID: school.ID, CollegeID: college.ID, MajorID: software.ID, Grade: "2023级", Name: "软件工程", Slug: "software-engineering", Description: "软件过程、需求、设计、测试和项目管理。", ExamScope: "需求分析、概要设计、测试、UML。", Status: model.StatusPublished},
		{SchoolID: school.ID, CollegeID: college.ID, MajorID: software.ID, Grade: "2023级", Name: "移动开发", Slug: "mobile-development", Description: "移动端 UI、网络、存储与工程化实践。", ExamScope: "Activity 生命周期、网络请求、本地存储。", Status: model.StatusPublished},
	}
	result := map[string]model.Course{}
	for index := range definitions {
		course := definitions[index]
		firstOrCreate(db, &course, "major_id = ? AND grade = ? AND slug = ?", course.MajorID, course.Grade, course.Slug)
		db.Model(&course).Updates(map[string]interface{}{
			"name":        definitions[index].Name,
			"description": definitions[index].Description,
			"exam_scope":  definitions[index].ExamScope,
			"status":      definitions[index].Status,
			"school_id":   definitions[index].SchoolID,
			"college_id":  definitions[index].CollegeID,
		})
		result[definitions[index].Slug] = course
	}
	return result
}

func seedMaterials(db *gorm.DB, courses map[string]model.Course) {
	materials := []model.Material{
		material(courses["discrete-math"], "离散数学样例资料", "other", "免费样例，验证下载链路。", "materials/discrete-math/sample.txt", "sample.txt", model.MaterialAccessFree, "这是免费样例资料。"),
		material(courses["discrete-math"], "离散数学重点知识点讲义", "knowledge_note", "覆盖命题逻辑、集合、关系、图论、树等期末重点。", "materials/discrete-math/knowledge-note.pdf", "knowledge-note.pdf", model.MaterialAccessLoginRequired, "本讲义整理离散数学期末高频知识点。"),
		material(courses["discrete-math"], "离散数学模拟卷一", "mock_paper", "按期末题型整理的第一套模拟卷。", "materials/discrete-math/mock-1.pdf", "mock-1.pdf", model.MaterialAccessPaid, "包含选择题、判断题、证明题和综合应用题。"),
		material(courses["discrete-math"], "离散数学答案解析", "answer", "模拟卷配套答案与详细解析。", "materials/discrete-math/answers.pdf", "answers.pdf", model.MaterialAccessPaid, "提供关键步骤和易错点说明。"),
		material(courses["probability-statistics-a"], "概率论重点知识点讲义", "knowledge_note", "概率论与数理统计A的核心概念整理。", "materials/probability-statistics-a/knowledge-note.pdf", "knowledge-note.pdf", model.MaterialAccessLoginRequired, "覆盖随机变量、分布和参数估计。"),
		material(courses["probability-statistics-a"], "概率论模拟卷一", "mock_paper", "概率论期末模拟卷。", "materials/probability-statistics-a/mock-1.pdf", "mock-1.pdf", model.MaterialAccessPaid, "贴近期末题型的综合练习。"),
		material(courses["college-physics"], "大学物理电磁学重点整理", "knowledge_note", "大学物理电磁学复习重点。", "materials/college-physics/electromagnetism-note.pdf", "electromagnetism-note.pdf", model.MaterialAccessLoginRequired, "整理电场、磁场与电磁感应。"),
		material(courses["advanced-math-a"], "高等数学A考前速背版", "quick_review", "高数A公式、题型和易错点。", "materials/advanced-math-a/quick-review.pdf", "quick-review.pdf", model.MaterialAccessLoginRequired, "考前快速过公式和典型题。"),
	}
	for index := range materials {
		firstOrCreate(db, &materials[index], "course_id = ? AND title = ?", materials[index].CourseID, materials[index].Title)
	}
}

func seedCoursePackages(db *gorm.DB, courses map[string]model.Course) {
	discreteCourse := courses["discrete-math"]
	courseID := discreteCourse.ID
	coursePackage := model.CoursePackage{
		SchoolID:    discreteCourse.SchoolID,
		CollegeID:   discreteCourse.CollegeID,
		MajorID:     discreteCourse.MajorID,
		CourseID:    &courseID,
		Grade:       discreteCourse.Grade,
		Title:       "Discrete Math Final Review Package",
		Slug:        "henu-software-2023-discrete-math-final",
		Description: "Seed package for course-package permission checks. Paid materials still require a package grant.",
		PriceFen:    1990,
		Currency:    "CNY",
		Status:      model.StatusPublished,
	}
	firstOrCreate(db, &coursePackage, "slug = ?", coursePackage.Slug)
	db.Model(&coursePackage).Updates(map[string]interface{}{
		"school_id":   coursePackage.SchoolID,
		"college_id":  coursePackage.CollegeID,
		"major_id":    coursePackage.MajorID,
		"course_id":   coursePackage.CourseID,
		"grade":       coursePackage.Grade,
		"title":       coursePackage.Title,
		"description": coursePackage.Description,
		"price_fen":   coursePackage.PriceFen,
		"currency":    coursePackage.Currency,
		"status":      coursePackage.Status,
	})

	var paidMaterials []model.Material
	if err := db.Where("course_id = ? AND access_level = ? AND status = ?", discreteCourse.ID, model.MaterialAccessPaid, model.StatusPublished).
		Order("created_at asc").
		Find(&paidMaterials).Error; err != nil {
		log.Fatal(err)
	}
	for index := range paidMaterials {
		item := model.CoursePackageItem{
			PackageID:    coursePackage.ID,
			ResourceType: "material",
			ResourceID:   paidMaterials[index].ID,
			SortOrder:    index + 1,
		}
		firstOrCreate(db, &item, "package_id = ? AND resource_type = ? AND resource_id = ?", item.PackageID, item.ResourceType, item.ResourceID)
	}
}

func seedQuestions(db *gorm.DB, courses map[string]model.Course) {
	seedQuestion(db, courses["discrete-math"], "single_choice", "命题 p -> q 为假时，p 与 q 的真值分别是？", "A", "蕴含命题仅在前件真、后件假时为假。", []model.QuizOption{
		{Label: "A", Content: "p 真，q 假", SortOrder: 1},
		{Label: "B", Content: "p 假，q 真", SortOrder: 2},
		{Label: "C", Content: "p 真，q 真", SortOrder: 3},
		{Label: "D", Content: "p 假，q 假", SortOrder: 4},
	})
	seedQuestion(db, courses["discrete-math"], "true_false", "任意简单图中所有顶点度数之和一定为偶数。", "true", "握手定理说明度数和等于边数的两倍。", nil)
	seedQuestion(db, courses["discrete-math"], "multiple_choice", "下列属于等价关系性质的是哪些？", "A,B,C", "等价关系满足自反、对称和传递。", []model.QuizOption{
		{Label: "A", Content: "自反性", SortOrder: 1},
		{Label: "B", Content: "对称性", SortOrder: 2},
		{Label: "C", Content: "传递性", SortOrder: 3},
		{Label: "D", Content: "单调性", SortOrder: 4},
	})
	seedQuestion(db, courses["probability-statistics-a"], "fill_blank", "若 A 与 B 独立，则 P(AB)=____。", "P(A)P(B)", "独立事件乘法公式。", nil)
	seedQuestion(db, courses["software-engineering"], "short_answer", "简述软件需求分析的主要目标。", "明确用户需求并形成可验证的软件需求规格说明。", "简答题当前使用人工参考答案，后续接 AI 语义评分。", nil)
}

func seedQuestion(db *gorm.DB, course model.Course, questionType string, stem string, answer string, explanation string, options []model.QuizOption) {
	question := model.QuizQuestion{
		CourseID:    course.ID,
		Type:        questionType,
		Stem:        stem,
		Answer:      answer,
		Explanation: explanation,
		Difficulty:  1,
		Status:      model.StatusPublished,
	}
	firstOrCreate(db, &question, "course_id = ? AND stem = ?", course.ID, stem)
	for index := range options {
		option := options[index]
		option.QuestionID = question.ID
		firstOrCreate(db, &option, "question_id = ? AND label = ?", question.ID, option.Label)
	}
}

func seedWikiAndCommunity(db *gorm.DB, users map[string]model.User, courses map[string]model.Course) {
	creator := users[model.RoleCreator]
	reviewer := users[model.RoleReviewer]
	user := users[model.RoleUser]
	discreteCourse := courses["discrete-math"]
	now := time.Now()

	wiki := model.WikiEntry{
		ReviewFields: model.ReviewFields{Status: model.StatusPublished, ReviewerID: &reviewer.ID, ReviewedAt: &now},
		ContentStats: model.ContentStats{Visibility: "public", LikeCount: 8, CommentCount: 1, CollectCount: 3},
		AuthorID:     creator.ID,
		CourseID:     &discreteCourse.ID,
		Title:        "命题逻辑速览",
		Slug:         "propositional-logic-overview",
		Content:      "命题逻辑复习时优先掌握真值表、等价式和推理规则。",
		Version:      1,
	}
	firstOrCreate(db, &wiki, "slug = ?", wiki.Slug)

	history := model.WikiEditHistory{EntryID: wiki.ID, EditorID: creator.ID, Version: 1, Content: wiki.Content, Summary: "创建词条"}
	firstOrCreate(db, &history, "entry_id = ? AND version = ?", wiki.ID, 1)

	application := model.WikiCreatorApplication{
		ReviewFields: model.ReviewFields{Status: model.StatusApproved, ReviewerID: &reviewer.ID, ReviewedAt: &now, ReviewReason: "示例创作者申请通过"},
		UserID:       creator.ID,
		Reason:       "希望维护离散数学复习词条。",
		SampleTitle:  "集合基础",
		SampleBody:   "集合运算、关系和映射是后续图论学习的基础。",
	}
	firstOrCreate(db, &application, "user_id = ? AND sample_title = ?", creator.ID, application.SampleTitle)

	post := model.BlogPost{
		ReviewFields: model.ReviewFields{Status: model.StatusPublished, ReviewerID: &reviewer.ID, ReviewedAt: &now},
		ContentStats: model.ContentStats{Visibility: "public", LikeCount: 5, CommentCount: 1, CollectCount: 2},
		AuthorID:     creator.ID,
		Title:        "离散数学期末复习顺序",
		Slug:         "discrete-math-review-order",
		Content:      "建议先过命题逻辑，再过集合与关系，最后集中做图论题。",
	}
	firstOrCreate(db, &post, "slug = ?", post.Slug)
	comment := model.BlogComment{AuthorID: user.ID, PostID: post.ID, Content: "这个顺序很适合考前一周。", Status: model.StatusPublished}
	firstOrCreate(db, &comment, "post_id = ? AND author_id = ?", post.ID, user.ID)

	board := model.ForumBoard{Name: "课程互助", Slug: "course-help", Description: "课程复习问题和资料反馈。", Status: model.StatusPublished}
	firstOrCreate(db, &board, "slug = ?", board.Slug)
	forumPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public", LikeCount: 3, CommentCount: 1, CollectCount: 0},
		AuthorID:     user.ID,
		BoardID:      board.ID,
		Title:        "离散数学图论部分怎么复习？",
		Content:      "树、最短路和欧拉图这几个点总是混。",
		Type:         "normal",
		Status:       model.StatusPublished,
	}
	firstOrCreate(db, &forumPost, "board_id = ? AND title = ?", board.ID, forumPost.Title)
	reply := model.ForumReply{AuthorID: creator.ID, PostID: forumPost.ID, Content: "先把定义和判定条件列成表，再做历年题。", IsBest: true, Status: model.StatusPublished}
	firstOrCreate(db, &reply, "post_id = ? AND author_id = ?", forumPost.ID, creator.ID)

	moment := model.Moment{
		ContentStats: model.ContentStats{Visibility: "public", LikeCount: 4, CommentCount: 1, CollectCount: 0},
		AuthorID:     user.ID,
		Content:      "今天刷完离散数学第一套模拟卷，错题集中在关系和图论。",
		Images:       jsonData(`[]`),
		Status:       model.StatusPublished,
	}
	firstOrCreate(db, &moment, "author_id = ? AND content = ?", user.ID, moment.Content)
	momentComment := model.MomentComment{AuthorID: creator.ID, MomentID: moment.ID, Content: "可以先看关系闭包那一节。", Status: model.StatusPublished}
	firstOrCreate(db, &momentComment, "moment_id = ? AND author_id = ?", moment.ID, creator.ID)
}

func seedPointsAndMembership(db *gorm.DB, users map[string]model.User) {
	rules := []model.PointsRule{
		{Code: "wiki_approved", Description: "Wiki 词条审核通过", Delta: 100, Enabled: true},
		{Code: "blog_liked", Description: "博客被点赞", Delta: 2, Enabled: true},
		{Code: "forum_best_answer", Description: "帖子最佳回答", Delta: 50, Enabled: true},
	}
	for index := range rules {
		firstOrCreate(db, &rules[index], "code = ?", rules[index].Code)
	}

	user := users[model.RoleUser]
	pointsLog := model.PointsLog{UserID: user.ID, Delta: 300, BalanceAfter: 300, Reason: "seed_initial_points", ReferenceType: "seed", ReferenceID: "initial", IdempotencyKey: "seed:user-initial-points"}
	firstOrCreate(db, &pointsLog, "idempotency_key = ?", pointsLog.IdempotencyKey)

	plans := []model.MembershipPlan{
		{Code: "tier1", Name: "基础会员", PriceFen: 990, Benefits: jsonData(`{"ai_wrong_analysis":true,"ai_discount":0.8}`), Status: model.StatusPublished},
		{Code: "tier2", Name: "高级会员", PriceFen: 1990, Benefits: jsonData(`{"ai_chat":true,"ai_papers":true,"ai_wrong_analysis":true}`), Status: model.StatusPublished},
	}
	for index := range plans {
		firstOrCreate(db, &plans[index], "code = ?", plans[index].Code)
	}

	expiresAt := time.Now().AddDate(0, 1, 0)
	membership := model.Membership{UserID: user.ID, PlanCode: "tier1", Status: "active", Source: "seed", ExpiresAt: &expiresAt}
	firstOrCreate(db, &membership, "user_id = ? AND plan_code = ? AND source = ?", user.ID, membership.PlanCode, membership.Source)
}

func seedAIAndSystem(db *gorm.DB, users map[string]model.User, courses map[string]model.Course) {
	user := users[model.RoleUser]
	course := courses["discrete-math"]
	task := model.AITask{
		UserID:   &user.ID,
		CourseID: &course.ID,
		Type:     "wrong_question_analysis",
		Status:   model.AITaskCompleted,
		Input:    jsonData(`{"source":"seed","wrongQuestionCount":3}`),
		Result:   jsonData(`{"summary":"关系与图论是当前薄弱点"}`),
	}
	firstOrCreate(db, &task, "user_id = ? AND course_id = ? AND type = ?", user.ID, course.ID, task.Type)

	draft := model.AIDraft{
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		TaskID:       task.ID,
		CourseID:     &course.ID,
		OutputType:   "quick_review",
		DraftContent: jsonData(`{"title":"离散数学考前速背草稿","note":"AI 草稿必须审核后发布"}`),
	}
	firstOrCreate(db, &draft, "task_id = ? AND output_type = ?", task.ID, draft.OutputType)

	usage := model.AIUsageLog{UserID: &user.ID, TaskID: &task.ID, Model: "mock-llm", TokensIn: 120, TokensOut: 80, CostFen: 0}
	firstOrCreate(db, &usage, "task_id = ? AND model = ?", task.ID, usage.Model)

	notification := model.Notification{UserID: user.ID, Type: "ai_task_completed", Title: "AI 错题分析已完成", Body: "离散数学错题分析草稿已生成，等待审核。", Data: jsonData(`{"source":"seed"}`)}
	firstOrCreate(db, &notification, "user_id = ? AND type = ? AND title = ?", user.ID, notification.Type, notification.Title)

	systemConfig := model.SystemConfig{Key: "seed_version", Value: jsonData(`{"version":"v2-demo-1"}`), Description: "V2 demo seed marker"}
	firstOrCreate(db, &systemConfig, "key = ?", systemConfig.Key)
}

func material(course model.Course, title string, materialType string, description string, storageKey string, fileName string, accessLevel string, preview string) model.Material {
	return model.Material{
		CourseID:       course.ID,
		Title:          title,
		Type:           materialType,
		Description:    description,
		StorageKey:     storageKey,
		FileName:       fileName,
		FileSize:       0,
		PreviewContent: preview,
		AccessLevel:    accessLevel,
		Status:         model.StatusPublished,
	}
}

func jsonData(value string) datatypes.JSON {
	return datatypes.JSON([]byte(value))
}

func firstOrCreate(db *gorm.DB, value interface{}, query interface{}, args ...interface{}) {
	if err := db.Where(query, args...).FirstOrCreate(value).Error; err != nil {
		log.Fatal(err)
	}
}
