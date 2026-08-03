package food

// PostBlock matches the frontend PostBlock interface.
type PostBlock struct {
	Type  string   `json:"type"` // h2|p|quote|list|img
	Text  *string  `json:"text,omitempty"`
	Items []string `json:"items,omitempty"`
	Src   *string  `json:"src,omitempty"`
	Ref   *int     `json:"ref,omitempty"`
}

// Shop matches the frontend Shop interface.
type Shop struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// Post matches the frontend Post interface.
type Post struct {
	ID      string      `json:"id"`
	Campus  string      `json:"campus"` // minglun|jinming|longzihu
	Title   string      `json:"title"`
	Excerpt string      `json:"excerpt"`
	Blocks  []PostBlock `json:"blocks"`
	Author  string      `json:"author"`
	Likes   int         `json:"likes"`
	Stars   int         `json:"stars"`
	Tags    []string    `json:"tags"`
	Shop    Shop        `json:"shop"`
	Time    string      `json:"time"`
	Hidden  bool        `json:"hidden"`
	Images  []string    `json:"images,omitempty"`
}

// Venue is a food venue card derived from real posts (never invented shops).
type Venue struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Rating float64 `json:"rating"`
	Tier   string  `json:"tier"`
	Campus string  `json:"campus"`
}

// Comment matches the frontend Comment interface.
type Comment struct {
	ID     string `json:"id"`
	PostID string `json:"postId"`
	Author string `json:"author"`
	Time   string `json:"time"`
	Text   string `json:"text"`
}

func p(s string) *string { return &s }

// MockPosts returns all mock food posts matching the frontend data.
func MockPosts() []Post {
	return []Post{
		{
			ID: "ml-01", Campus: "minglun", Title: "老碗面：十年不换配方的汤头",
			Excerpt: "西门外的续命面馆，期末周排队的全是熟面孔。",
			Author:  "楼下的猫", Likes: 214, Stars: 96,
			Tags: []string{"面食", "西门", "夯"},
			Shop: Shop{Name: "老碗面（西门店）", Lat: 34.8201, Lng: 114.3512},
			Time: "07-18 12:40", Hidden: false,
			Blocks: []PostBlock{
				{Type: "p", Text: p("明伦西门外走到头那家老碗面，开了快十年，汤头是每天现熬的牛骨汤，这个在学生街属于异类。")},
				{Type: "h2", Text: p("点什么")},
				{Type: "list", Items: []string{"招牌牛肉面 ¥14：肉给得实在，汤能喝完", "油泼扯面 ¥10：辣子香，面是现扯的", "卤蛋 ¥2：浸得很透"}},
				{Type: "quote", Text: p("锐评：饭点排队 15 分钟起步，但出餐快。打分：夯。")},
				{Type: "p", Text: p("避雷点：下午两点后基本卖完，别跑空。")},
			},
		},
		{
			ID: "ml-02", Campus: "minglun", Title: "鸡公煲的微辣是谎言",
			Excerpt: "南门鸡公煲，点单请自觉降一档辣度。",
			Author:  "干饭组组长", Likes: 187, Stars: 74,
			Tags: []string{"鸡公煲", "南门", "夯"},
			Shop: Shop{Name: "重庆鸡公煲（南门店）", Lat: 34.8158, Lng: 114.3556},
			Time: "07-17 19:05", Hidden: false,
			Blocks: []PostBlock{
				{Type: "p", Text: p("南门这家鸡公煲是宿舍聚餐默认选项，分量够两个人，人均 ¥25 左右。")},
				{Type: "quote", Text: p("锐评：微辣约等于外面的中辣，第一次来点微微辣，别硬撑。打分：夯。")},
				{Type: "list", Items: []string{"鸡公煲小份 ¥38：加土豆和宽粉是标配", "酸梅汤 ¥4：解辣刚需"}},
			},
		},
		{
			ID: "jm-01", Campus: "jinming", Title: "商业街手打柠檬茶：冰块比茶多",
			Excerpt: "夏天还是得靠它续命。",
			Author:  "柠檬精本精", Likes: 156, Stars: 63,
			Tags: []string{"饮品", "商业街", "夯"},
			Shop: Shop{Name: "手打柠檬茶（金明商业街）", Lat: 34.8240, Lng: 114.3105},
			Time: "07-18 15:20", Hidden: false,
			Blocks: []PostBlock{
				{Type: "p", Text: p("金明商业街新开的柠檬茶，现切现打，茶香和酸度都在线，¥12 一杯。")},
				{Type: "quote", Text: p("锐评：冰给得比茶多是事实，点少冰刚好。打分：夯。")},
			},
		},
		{
			ID: "jm-02", Campus: "jinming", Title: "北门胡辣汤：河南人的早八仪式感",
			Excerpt: "配两块钱的油馍头，满血进教室。",
			Author:  "胡辣汤守卫者", Likes: 203, Stars: 88,
			Tags: []string{"早餐", "北门", "夯"},
			Shop: Shop{Name: "方中山胡辣汤（北门）", Lat: 34.8251, Lng: 114.3062},
			Time: "07-17 07:50", Hidden: false,
			Blocks: []PostBlock{
				{Type: "p", Text: p("北门的胡辣汤早上六点半就开，牛肉片给得不抠门，辣度自己加。")},
				{Type: "list", Items: []string{"优质胡辣汤 ¥8", "油馍头 ¥2/份", "茶叶蛋 ¥1.5"}},
				{Type: "quote", Text: p("锐评：早八前来一碗，一上午不饿。打分：夯。")},
			},
		},
		{
			ID: "lz-01", Campus: "longzihu", Title: "南门灌汤包：先开窗后喝汤",
			Excerpt: "皮薄汤足，小心烫嘴。",
			Author:  "汤包猎人", Likes: 178, Stars: 71,
			Tags: []string{"灌汤包", "南门", "夯"},
			Shop: Shop{Name: "第一楼灌汤包（南门）", Lat: 34.8156, Lng: 113.8301},
			Time: "07-18 11:30", Hidden: false,
			Blocks: []PostBlock{
				{Type: "p", Text: p("南门的灌汤包是开封做法，一笼八只 ¥16，先咬小口喝汤再吃肉。")},
				{Type: "quote", Text: p("锐评：刚出笼的能烫掉一层嘴皮，等三分钟。打分：夯。")},
			},
		},
		{
			ID: "lz-02", Campus: "longzihu", Title: "西区食堂的隐藏窗口：烩面",
			Excerpt: "本地人认证，汤是羊骨熬的。",
			Author:  "你", Likes: 145, Stars: 60,
			Tags: []string{"烩面", "食堂", "夯"},
			Shop: Shop{Name: "龙子湖西区食堂二楼", Lat: 34.8180, Lng: 113.8275},
			Time: "07-17 12:20", Hidden: false,
			Blocks: []PostBlock{
				{Type: "p", Text: p("西区食堂二楼最里面的烩面窗口，¥10 一碗，汤头奶白，海带丝和千张丝给得足。")},
				{Type: "quote", Text: p("锐评：比校外 ¥18 的强。打分：夯。")},
			},
		},
	}
}

// MockComments returns mock comments matching the frontend data.
func MockComments() []Comment {
	return []Comment{
		{ID: "c1", PostID: "ml-01", Author: "干饭组组长", Time: "07-18 13:02", Text: "油泼扯面 +1，辣子是真的香。"},
		{ID: "c2", PostID: "ml-01", Author: "早八不迟到", Time: "07-18 14:20", Text: "昨天下午两点去果然卖完了，血泪教训。"},
		{ID: "c3", PostID: "ml-02", Author: "柠檬精本精", Time: "07-17 20:11", Text: "微微辣选手报到，上次点微辣喝了三瓶酸梅汤。"},
		{ID: "c5", PostID: "jm-02", Author: "你", Time: "07-17 08:30", Text: "油馍头泡进去十秒再吃，懂的都懂。"},
		{ID: "c7", PostID: "lz-01", Author: "汤包猎人", Time: "07-18 12:05", Text: "配一碗蛋花汤，人均 ¥20 封顶。"},
	}
}
