package practice

// Question matches the frontend Question interface.
type Question struct {
	ID          string    `json:"id"`
	Subject     string    `json:"subject"`
	Chapter     string    `json:"chapter"`
	Difficulty  float64   `json:"difficulty"`
	Stem        string    `json:"stem"`
	Options     [4]string `json:"options"`
	Answer      int       `json:"answer"`
	Explanation string    `json:"explanation"`
	Accuracy    int       `json:"accuracy"`
}

// QuizListMeta matches the frontend QuizListMeta interface.
type QuizListMeta struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Creator    string   `json:"creator"`
	Tags       []string `json:"tags"`
	PoolKey    string   `json:"poolKey"`
	Count      int      `json:"count"`
	Completion int      `json:"completion"`
}

// Subject matches the frontend Subject interface.
type Subject struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	Lists []QuizListMeta `json:"lists"`
}

// Major matches the frontend Major interface.
type Major struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Subjects []Subject `json:"subjects"`
}

// School matches the frontend School interface.
type School struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Majors []Major `json:"majors"`
}

// LeaderboardRow matches the frontend LeaderboardRow interface.
type LeaderboardRow struct {
	Name      string `json:"name"`
	Questions int    `json:"questions"`
	Accuracy  int    `json:"accuracy"`
	Streak    int    `json:"streak"`
	IsYou     bool   `json:"isYou,omitempty"`
}

// Bank is the portal-gateway practice bank card.
type Bank struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Subject       string `json:"subject"`
	QuestionCount int    `json:"question_count"`
}

// MockBanks flattens mock school hierarchy into bank cards (mock mode only).
func MockBanks() []Bank {
	var banks []Bank
	for _, school := range MockSchools() {
		for _, major := range school.Majors {
			for _, subject := range major.Subjects {
				for _, list := range subject.Lists {
					banks = append(banks, Bank{
						ID:            list.ID,
						Name:          list.Name,
						Subject:       subject.Name,
						QuestionCount: list.Count,
					})
				}
			}
		}
	}
	if banks == nil {
		banks = []Bank{}
	}
	return banks
}

// MockSchools returns the full hierarchy matching the frontend SCHOOLS.
func MockSchools() []School {
	return []School{
		{
			ID: "cs", Name: "计算机与信息工程学院",
			Majors: []Major{
				{
					ID: "se", Name: "软件工程",
					Subjects: []Subject{
						{
							ID: "se-ds", Name: "数据结构",
							Lists: []QuizListMeta{
								{ID: "ds-final", Name: "数据结构 · 期末冲刺 50 题", Creator: "王助教", Tags: []string{"期末", "高频考点"}, PoolKey: "ds", Count: 10, Completion: 68},
								{ID: "ds-tree", Name: "数据结构 · 树与图专题", Creator: "21 级-李学长", Tags: []string{"专题", "考研"}, PoolKey: "ds", Count: 8, Completion: 24},
							},
						},
						{
							ID: "se-os", Name: "操作系统",
							Lists: []QuizListMeta{
								{ID: "os-core", Name: "操作系统 · 核心概念精选", Creator: "王助教", Tags: []string{"期末", "概念"}, PoolKey: "os", Count: 8, Completion: 41},
							},
						},
					},
				},
				{
					ID: "cst", Name: "计算机科学与技术",
					Subjects: []Subject{
						{
							ID: "cst-ds", Name: "数据结构",
							Lists: []QuizListMeta{
								{ID: "ds-kaoyan", Name: "数据结构 · 考研基础题", Creator: "20 级-陈学姐", Tags: []string{"考研", "基础"}, PoolKey: "ds", Count: 8, Completion: 12},
							},
						},
					},
				},
			},
		},
		{
			ID: "maths", Name: "数学与统计学院",
			Majors: []Major{
				{
					ID: "am", Name: "数学与应用数学",
					Subjects: []Subject{
						{
							ID: "am-math", Name: "高等数学A",
							Lists: []QuizListMeta{
								{ID: "math-limit", Name: "高等数学A · 极限与连续", Creator: "刘助教", Tags: []string{"期中", "基础"}, PoolKey: "math", Count: 10, Completion: 83},
								{ID: "math-mvt", Name: "高等数学A · 中值定理专题", Creator: "22 级-赵同学", Tags: []string{"专题", "难点"}, PoolKey: "math", Count: 8, Completion: 36},
							},
						},
						{
							ID: "am-la", Name: "线性代数",
							Lists: []QuizListMeta{
								{ID: "la-matrix", Name: "线性代数 · 矩阵与行列式", Creator: "刘助教", Tags: []string{"期末", "计算"}, PoolKey: "la", Count: 8, Completion: 57},
							},
						},
					},
				},
			},
		},
	}
}

// MockQuestions returns questions for a given pool key.
func MockQuestions(poolKey string) []Question {
	pools := map[string][]Question{
		"ds": {
			{ID: "ds-01", Subject: "数据结构", Chapter: "排序", Difficulty: 2.5, Stem: "下列排序算法中，最坏情况下时间复杂度为 O(n²) 的是？", Options: [4]string{"归并排序", "快速排序", "堆排序", "基数排序"}, Answer: 1, Explanation: "快速排序在序列已有序时划分极不平衡，退化为 O(n²)。", Accuracy: 82},
			{ID: "ds-02", Subject: "数据结构", Chapter: "链表", Difficulty: 3.0, Stem: "在带头结点的单链表中，删除首元结点的时间复杂度是？", Options: [4]string{"O(1)", "O(log n)", "O(n)", "O(n log n)"}, Answer: 0, Explanation: "只需把头结点的 next 指向第二个结点即可。", Accuracy: 78},
			{ID: "ds-03", Subject: "数据结构", Chapter: "栈与队列", Difficulty: 3.5, Stem: "对后缀表达式求值，最适合使用的数据结构是？", Options: [4]string{"队列", "栈", "单链表", "哈希表"}, Answer: 1, Explanation: "扫描到操作数入栈，遇到运算符弹出栈顶两个操作数运算后压回。", Accuracy: 75},
			{ID: "ds-04", Subject: "数据结构", Chapter: "图", Difficulty: 4.0, Stem: "对图进行广度优先搜索时，通常需要借助哪种数据结构？", Options: [4]string{"栈", "队列", "二叉树", "并查集"}, Answer: 1, Explanation: "BFS 按层次访问，先进先出，故用队列。", Accuracy: 72},
			{ID: "ds-05", Subject: "数据结构", Chapter: "树", Difficulty: 5.0, Stem: "含 n 个结点的二叉链表中，空指针域的个数为？", Options: [4]string{"n - 1", "n", "n + 1", "2n"}, Answer: 2, Explanation: "指针域共 2n 个，非空指针等于分支数 n - 1，故空指针域 = n + 1。", Accuracy: 65},
		},
		"math": {
			{ID: "math-01", Subject: "高等数学A", Chapter: "极限", Difficulty: 2.0, Stem: "极限 lim(x→0) (sin x) / x 的值为？", Options: [4]string{"0", "1", "e", "不存在"}, Answer: 1, Explanation: "重要极限之一：x→0 时 sin x 与 x 为等价无穷小。", Accuracy: 88},
			{ID: "math-02", Subject: "高等数学A", Chapter: "极限", Difficulty: 3.0, Stem: "极限 lim(x→∞) (1 + 1/x)^x 的值为？", Options: [4]string{"1", "1/e", "e", "+∞"}, Answer: 2, Explanation: "第二个重要极限，结果为自然对数的底 e。", Accuracy: 82},
			{ID: "math-03", Subject: "高等数学A", Chapter: "连续", Difficulty: 3.5, Stem: "函数 f(x) 在点 x₀ 处连续的充要条件是？", Options: [4]string{"f(x) 在 x₀ 处有定义", "f(x) 在 x₀ 处左右极限存在", "lim(x→x₀) f(x) = f(x₀)", "f(x) 在 x₀ 处可导"}, Answer: 2, Explanation: "连续的定义即极限值等于函数值。", Accuracy: 76},
		},
		"la": {
			{ID: "la-01", Subject: "线性代数", Chapter: "行列式", Difficulty: 3.0, Stem: "互换行列式的两行，行列式的值？", Options: [4]string{"不变", "变号", "变为 0", "变为原来的 2 倍"}, Answer: 1, Explanation: "行列式基本性质：互换两行，行列式变号。", Accuracy: 80},
			{ID: "la-02", Subject: "线性代数", Chapter: "矩阵", Difficulty: 3.5, Stem: "关于矩阵乘法，下列说法正确的是？", Options: [4]string{"满足交换律", "一般不满足交换律", "AB = 0 则必有 A = 0 或 B = 0", "满足消去律"}, Answer: 1, Explanation: "矩阵乘法一般 AB ≠ BA。", Accuracy: 74},
		},
		"os": {
			{ID: "os-01", Subject: "操作系统", Chapter: "进程管理", Difficulty: 2.5, Stem: "进程与程序的本质区别是？", Options: [4]string{"进程占用内存，程序不占内存", "进程是动态的，程序是静态的", "进程可以并发，程序不能并发", "二者没有区别"}, Answer: 1, Explanation: "程序是指令的静态集合，进程是程序的一次动态执行过程。", Accuracy: 85},
			{ID: "os-02", Subject: "操作系统", Chapter: "进程管理", Difficulty: 3.5, Stem: "进程的基本三态模型不包括下列哪个状态？", Options: [4]string{"就绪态", "运行态", "阻塞态", "挂起态"}, Answer: 3, Explanation: "基本三态为就绪、运行、阻塞；挂起态属于扩展的五态模型。", Accuracy: 77},
		},
	}
	if qs, ok := pools[poolKey]; ok {
		return qs
	}
	return pools["ds"]
}

// MockLeaderboard returns leaderboard data matching the frontend.
func MockLeaderboard(period string) []LeaderboardRow {
	switch period {
	case "month":
		return []LeaderboardRow{
			{Name: "图书馆常驻人口", Questions: 1286, Accuracy: 86, Streak: 30},
			{Name: "卷王本王", Questions: 1194, Accuracy: 87, Streak: 21},
			{Name: "考研上岸ing", Questions: 1053, Accuracy: 80, Streak: 26},
			{Name: "代码炼丹师", Questions: 967, Accuracy: 78, Streak: 16},
			{Name: "早八不迟到", Questions: 912, Accuracy: 81, Streak: 13},
			{Name: "你", Questions: 845, Accuracy: 80, Streak: 12, IsYou: true},
		}
	case "all":
		return []LeaderboardRow{
			{Name: "卷王本王", Questions: 8642, Accuracy: 89, Streak: 61},
			{Name: "图书馆常驻人口", Questions: 8217, Accuracy: 87, Streak: 45},
			{Name: "考研上岸ing", Questions: 7980, Accuracy: 84, Streak: 58},
			{Name: "你", Questions: 6531, Accuracy: 82, Streak: 34, IsYou: true},
			{Name: "代码炼丹师", Questions: 6104, Accuracy: 80, Streak: 29},
		}
	default: // week
		return []LeaderboardRow{
			{Name: "卷王本王", Questions: 342, Accuracy: 88, Streak: 21},
			{Name: "图书馆常驻人口", Questions: 317, Accuracy: 85, Streak: 14},
			{Name: "早八不迟到", Questions: 295, Accuracy: 82, Streak: 9},
			{Name: "代码炼丹师", Questions: 268, Accuracy: 79, Streak: 16},
			{Name: "你", Questions: 231, Accuracy: 81, Streak: 12, IsYou: true},
		}
	}
}
