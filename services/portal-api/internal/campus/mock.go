package campus

// Item matches the frontend Item interface.
type Item struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"` // help|sell
	Category  string   `json:"category"`
	Title     string   `json:"title"`
	Desc      string   `json:"desc"`
	Price     float64  `json:"price"`
	Seller    string   `json:"seller"`
	Credit    int      `json:"credit"`
	DealsDone int      `json:"dealsDone"`
	Wants     int      `json:"wants"`
	Place     string   `json:"place"`
	Deadline  *string  `json:"deadline,omitempty"`
	Status    string   `json:"status"` // open|ongoing|done|hidden
	Time      string   `json:"time"`
	Images    []string `json:"images,omitempty"`
}

// Category matches the frontend Category interface.
type Category struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// DealMessage matches the frontend DealMessage interface.
type DealMessage struct {
	ID     string `json:"id"`
	ItemID string `json:"itemId"`
	Author string `json:"author"`
	Time   string `json:"time"`
	Text   string `json:"text"`
}

func strp(s string) *string { return &s }

// MockCategories returns categories matching the frontend.
func MockCategories() []Category {
	return []Category{
		{Key: "errand", Name: "跑腿代办", Code: "RUN"},
		{Key: "express", Name: "代取快递", Code: "EXP"},
		{Key: "luggage", Name: "搬运行李", Code: "LUG"},
		{Key: "seat", Name: "占座打卡", Code: "SEA"},
		{Key: "skill", Name: "技能服务", Code: "SKI"},
		{Key: "flea", Name: "闲置出售", Code: "FLE"},
	}
}

// MockItems returns items matching the frontend data.
func MockItems() []Item {
	return []Item{
		{ID: "h-01", Type: "help", Category: "express", Title: "代取中通快递 3 件到 6 号楼", Desc: "快递在明伦西门菜鸟驿站，三个小件，取件码私发。", Price: 3, Seller: "取快递困难户", Credit: 86, DealsDone: 12, Wants: 45, Place: "明伦校区 · 西门驿站", Deadline: strp("今天 18:00 前"), Status: "open", Time: "07-19 10:20"},
		{ID: "h-02", Type: "help", Category: "luggage", Title: "开学搬行李上六楼（无电梯）", Desc: "两个 28 寸行李箱 + 一个编织袋，从校门口搬到桃李园 6 楼。", Price: 15, Seller: "你", Credit: 91, DealsDone: 3, Wants: 28, Place: "金明校区 · 桃李园", Deadline: strp("本周五下午"), Status: "open", Time: "07-19 09:05"},
		{ID: "h-03", Type: "help", Category: "skill", Title: "代做数据结构课程小项目（哈夫曼编码）", Desc: "课程小项目：实现哈夫曼编码/解码，要求有注释和测试用例。", Price: 80, Seller: "DDL 战士", Credit: 79, DealsDone: 31, Wants: 67, Place: "线上交付", Deadline: strp("下周五 23:59 前"), Status: "ongoing", Time: "07-18 22:40"},
		{ID: "s-01", Type: "sell", Category: "flea", Title: "九成新机械键盘 青轴 87 键", Desc: "用了三个月，无打油无暗病，箱说全。", Price: 120, Seller: "你", Credit: 91, DealsDone: 3, Wants: 58, Place: "金明校区 · 可送到楼下", Status: "open", Time: "07-18 15:30"},
		{ID: "s-02", Type: "sell", Category: "flea", Title: "考研英语红宝书 全新未拆封", Desc: "25 版红宝书，买重复了，全新塑封未拆。", Price: 25, Seller: "考研上岸ing", Credit: 95, DealsDone: 27, Wants: 41, Place: "明伦校区", Status: "open", Time: "07-18 11:10"},
		{ID: "s-03", Type: "sell", Category: "flea", Title: "宿舍小冰箱 46L 用了一年", Desc: "制冷正常，无异味，毕业出。", Price: 180, Seller: "毕业清仓中", Credit: 88, DealsDone: 19, Wants: 73, Place: "明伦校区 · 自提", Status: "done", Time: "07-17 18:25"},
	}
}

// MockMessages returns messages matching the frontend data.
func MockMessages() []DealMessage {
	return []DealMessage{
		{ID: "m1", ItemID: "h-01", Author: "顺路侠", Time: "07-19 10:35", Text: "我中午正好要去驿站，可以接。"},
		{ID: "m2", ItemID: "h-01", Author: "取快递困难户", Time: "07-19 10:40", Text: "好！我私你取件码。"},
		{ID: "m3", ItemID: "s-01", Author: "键盘侠", Time: "07-18 16:02", Text: "什么牌子的？轴体是樱桃青吗？"},
		{ID: "m4", ItemID: "s-01", Author: "你", Time: "07-18 16:20", Text: "国产轴，手感接近青轴，介意勿拍。"},
	}
}
