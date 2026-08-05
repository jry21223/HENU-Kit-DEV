package library

// Material matches the frontend Material interface exactly.
type Material struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"` // note|exam|mock|path|lab
	Subject      string     `json:"subject"`
	Title        string     `json:"title"`
	Author       string     `json:"author"`
	Intro        string     `json:"intro"`
	TOC          []string   `json:"toc"`
	Pages        [][]string `json:"pages"`
	Price        int        `json:"price"`
	PreviewPages int        `json:"previewPages"`
	Rating       float64    `json:"rating"`
	Downloads    int        `json:"downloads"`
	Favs         int        `json:"favs"`
	// Where the file is served from. Empty when the owner has no file for this
	// material, in which case the Portal offers no download.
	DownloadURL string `json:"downloadUrl,omitempty"`
	FileName    string `json:"fileName,omitempty"`
	FileSize    int64  `json:"fileSize,omitempty"`
}

// Course is the portal-gateway catalog course card.
type Course struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Subject       string `json:"subject"`
	MaterialCount int    `json:"material_count"`
}

// MockCourses aggregates mock materials into course cards (mock mode only).
func MockCourses() []Course {
	counts := map[string]int{}
	for _, m := range MockMaterials() {
		counts[m.Subject]++
	}
	courses := make([]Course, 0, len(counts))
	for subject, n := range counts {
		courses = append(courses, Course{
			ID:            "mock-" + subject,
			Name:          subject,
			Subject:       subject,
			MaterialCount: n,
		})
	}
	return courses
}

// MockMaterials returns all mock materials matching the frontend data.
func MockMaterials() []Material {
	return []Material{
		{
			ID: "free-limit-note", Type: "note", Subject: "高等数学A",
			Title: "极限与连续 · 学霸笔记", Author: "21 级-李学长",
			Intro: "期末 92 分学长的一轮复习笔记：两个重要极限、等价无穷小替换表、连续与间断点判定的全部套路，附 12 道易错例题。",
			TOC:   []string{"极限的定义与运算法则", "两个重要极限", "等价无穷小替换表", "连续性判定", "间断点分类", "易错例题 12 则"},
			Pages: [][]string{
				{"§1 极限的定义：∀ε>0，∃δ>0，当 0<|x-x₀|<δ 时 |f(x)-A|<ε。考试不考 ε-δ 证明大题，但选择常考概念辨析。", "运算法则核心一条：和差积商的极限 = 极限的和差积商，前提是各部分极限都存在（分母不为 0）。"},
				{"§2 两个重要极限：lim(x→0) sin x / x = 1；lim(x→∞) (1+1/x)^x = e。", "使用要点：必须凑出完全一致的形式。sin(2x)/x 要先配系数 2；(1+2/x)^x 要凑成 [(1+2/x)^(x/2)]² → e²。"},
				{"§3 常用等价无穷小（x→0）：sin x ~ x，tan x ~ x，1-cos x ~ x²/2，ln(1+x) ~ x，e^x-1 ~ x，(1+x)^a-1 ~ ax。", "替换原则：乘除因子可直接换，加减项不能乱换（精度不够时改用泰勒）。"},
				{"§4 连续性：f 在 x₀ 连续 ⟺ lim(x→x₀) f(x) = f(x₀)。分段函数重点看分段点：分别求左右极限再与函数值比对。"},
				{"§5 间断点分类：左右极限都存在为第一类（相等=可去，不等=跳跃）；至少一个不存在为第二类（无穷/振荡）。", "真题套路：给出含参函数求 a、b 使函数连续——列'左极限=右极限=函数值'两个方程解两个参数。"},
				{"§6 易错例：lim(x→0) (tan x - sin x)/x³。错解：tan x ~ x、sin x ~ x 得 0。", "正解：tan x - sin x = sin x(1-cos x)/cos x ~ x · x²/2 = x³/2，答案 1/2。加减项直接替换必错。"},
				{"§7 易错例：求 lim(x→∞) (√(x²+x) - x)。有理化：分子乘共轭得 x/(√(x²+x)+x) → 1/2。", "∞-∞ 型先有理化或通分，是期末填空高频。"},
				{"§8 考前 checklist：重要极限 2 个、等价无穷小 6 组、连续性定义 1 条、间断点 4 类。", "本章在全卷占 15-20 分，性价比最高，务必拿满。"},
			},
			Price: 0, PreviewPages: 0, Rating: 4.8, Downloads: 2103, Favs: 486,
		},
		{
			ID: "free-ds-exam24", Type: "exam", Subject: "数据结构",
			Title: "数据结构 · 2024 期末试卷 A 卷", Author: "王助教",
			Intro: "2024 秋季学期真题（含参考答案要点）：选择 20 分、填空 20 分、应用题 40 分、算法设计 20 分。树与图占比最高。",
			TOC:   []string{"一、选择题", "二、填空题", "三、应用题", "四、算法设计题", "参考答案要点"},
			Pages: [][]string{
				{"一、选择题（每题 2 分，共 20 分）", "1. 下列结构中属于非线性结构的是（ ）。A. 栈 B. 队列 C. 二叉树 D. 串"},
				{"二、填空题（每空 2 分，共 20 分）", "1. 含 n 个结点的二叉链表中有 ____ 个空指针域。"},
				{"三、应用题（共 40 分）", "1.（10 分）已知某二叉树先序遍历为 ABDGCEHF，中序遍历为 DGBAHECF，画出该二叉树并写出后序序列。"},
				{"四、算法设计题（共 20 分）", "1.（10 分）设计算法判断单链表是否递增有序。"},
				{"参考答案要点：选择 1.C 2.B 3.C；填空 1. n+1。"},
			},
			Price: 0, PreviewPages: 0, Rating: 4.9, Downloads: 3567, Favs: 812,
		},
		{
			ID: "free-la-cards", Type: "note", Subject: "线性代数",
			Title: "矩阵运算公式手卡", Author: "22 级-赵同学",
			Intro: "A4 单页可打印：矩阵乘法、转置、逆、伴随、秩的全部高频公式与 6 个经典反例。",
			TOC:   []string{"乘法规则", "转置与逆", "伴随矩阵", "秩的不等式", "经典反例"},
			Pages: [][]string{
				{"乘法规则：AB 的第 (i,j) 元 = A 第 i 行 · B 第 j 列。"},
				{"转置：(AB)ᵀ = BᵀAᵀ（顺序反转）。逆：(AB)⁻¹ = B⁻¹A⁻¹。"},
				{"伴随矩阵：AA* = A*A = |A|E。"},
				{"秩的不等式：r(A+B) ≤ r(A)+r(B)；r(AB) ≤ min{r(A),r(B)}。"},
				{"经典反例：AB=0 推不出 A=0 或 B=0。"},
				{"考前默写清单：(AB)ᵀ、(AB)⁻¹、AA*、|A*|、r(AB) 上限，共 5 条。"},
			},
			Price: 0, PreviewPages: 0, Rating: 4.6, Downloads: 1588, Favs: 302,
		},
	}
}
