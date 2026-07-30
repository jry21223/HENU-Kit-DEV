/**
 * 美食子站 mock 数据层 + foodStore（useSyncExternalStore 单例，同 accountStore 模式）。
 * 字面量数据，SSR/客户端一致；点赞/收藏/评论/发布/编辑/隐藏/删除仅会话内存态，刷新还原。
 */

// ---------------------------------------------------------------- 校区

export type CampusKey = "minglun" | "jinming" | "longzihu";

export const CAMPUSES: Record<
  CampusKey,
  { name: string; index: string; lat: number; lng: number }
> = {
  minglun: { name: "明伦校区", index: "01", lat: 34.8186, lng: 114.3544 },
  jinming: { name: "金明校区", index: "02", lat: 34.8225, lng: 114.3079 },
  longzihu: { name: "龙子湖校区", index: "03", lat: 34.8173, lng: 113.8293 },
};

export const CAMPUS_KEYS = Object.keys(CAMPUSES) as CampusKey[];

// ---------------------------------------------------------------- 类型

export interface PostBlock {
  type: "h2" | "p" | "quote" | "list" | "img";
  text?: string;
  items?: string[];
  src?: string;
  ref?: number; // 编辑器附件序号（发布时解析为 src）
}

export interface Shop {
  name: string;
  lat: number;
  lng: number;
}

export interface Post {
  id: string;
  campus: CampusKey;
  title: string;
  excerpt: string;
  blocks: PostBlock[];
  author: string;
  likes: number;
  stars: number;
  tags: string[];
  shop: Shop;
  time: string;
  hidden: boolean;
  isMine?: boolean;
  liked?: boolean;
  starred?: boolean;
  /** 首图为封面/列表缩略图，其余可作正文插图 */
  images?: string[];
}

export interface Comment {
  id: string;
  postId: string;
  author: string;
  time: string;
  text: string;
}

// ---------------------------------------------------------------- mock 文章

function post(
  id: string,
  campus: CampusKey,
  title: string,
  excerpt: string,
  author: string,
  likes: number,
  stars: number,
  tags: string[],
  shop: Shop,
  time: string,
  blocks: PostBlock[],
  isMine?: boolean
): Post {
  return { id, campus, title, excerpt, author, likes, stars, tags, shop, time, blocks, hidden: false, isMine };
}

const INITIAL_POSTS: Post[] = [
  // ---- 明伦校区 ----
  post("ml-01", "minglun", "老碗面：十年不换配方的汤头", "西门外的续命面馆，期末周排队的全是熟面孔。", "楼下的猫", 214, 96, ["面食", "西门", "夯"],
    { name: "老碗面（西门店）", lat: 34.8201, lng: 114.3512 }, "07-18 12:40",
    [
      { type: "p", text: "明伦西门外走到头那家老碗面，开了快十年，汤头是每天现熬的牛骨汤，这个在学生街属于异类。" },
      { type: "h2", text: "点什么" },
      { type: "list", items: ["招牌牛肉面 ¥14：肉给得实在，汤能喝完", "油泼扯面 ¥10：辣子香，面是现扯的", "卤蛋 ¥2：浸得很透"] },
      { type: "quote", text: "锐评：饭点排队 15 分钟起步，但出餐快。打分：夯。" },
      { type: "p", text: "避雷点：下午两点后基本卖完，别跑空。" },
    ]),
  post("ml-02", "minglun", "鸡公煲的微辣是谎言", "南门鸡公煲，点单请自觉降一档辣度。", "干饭组组长", 187, 74, ["鸡公煲", "南门", "夯"],
    { name: "重庆鸡公煲（南门店）", lat: 34.8158, lng: 114.3556 }, "07-17 19:05",
    [
      { type: "p", text: "南门这家鸡公煲是宿舍聚餐默认选项，分量够两个人，人均 ¥25 左右。" },
      { type: "quote", text: "锐评：微辣约等于外面的中辣，第一次来点微微辣，别硬撑。打分：夯。" },
      { type: "list", items: ["鸡公煲小份 ¥38：加土豆和宽粉是标配", "酸梅汤 ¥4：解辣刚需"] },
    ]),
  post("ml-03", "minglun", "食堂三楼烤盘饭：看阿姨心情", "排队二十分钟吃饭五分钟，肉量随机。", "虾仁猪心", 96, 21, ["食堂", "烤盘饭", "拉完了"],
    { name: "明伦食堂三楼", lat: 34.8180, lng: 114.3548 }, "07-16 12:15",
    [
      { type: "p", text: "烤盘饭窗口永远排长队，但出餐速度其实不慢，问题在于肉量全看阿姨当天手感。" },
      { type: "quote", text: "锐评：同样的价格，周一是烤肉饭，周五是烤土豆饭。打分：拉。" },
      { type: "p", text: "建议错开 12:00–12:30 高峰，11:40 前到基本不排队。" },
    ]),
  post("ml-04", "minglun", "东门麻辣烫的称重玄学", "同样的菜，每次价格都不一样。", "开封菜研究僧", 78, 18, ["麻辣烫", "东门", "拉完了"],
    { name: "张亮麻辣烫（东门）", lat: 34.8193, lng: 114.3583 }, "07-15 18:30",
    [
      { type: "p", text: "东门麻辣烫味道过得去，但称重一直是玄学——建议自己先掂量再拿盆。" },
      { type: "quote", text: "锐评：选菜一时爽，称重火葬场。打分：拉。" },
      { type: "list", items: ["必拿：宽粉、鹌鹑蛋", "慎拿：冻豆腐（巨吸水，压秤）"] },
    ]),
  post("ml-05", "minglun", "夜自习后的救星：西门烤冷面", "晚上十点还亮着灯的小摊。", "你", 132, 55, ["夜宵", "西门", "夯"],
    { name: "东北烤冷面（西门夜市）", lat: 34.8204, lng: 114.3518 }, "07-14 22:10",
    [
      { type: "p", text: "图书馆闭馆出来，西门夜市的烤冷面摊子还亮着灯，加蛋加肠 ¥8，是冬夜的救赎。" },
      { type: "quote", text: "锐评：酱给得足，洋葱免费加。打分：夯。" },
    ], true),

  // ---- 金明校区 ----
  post("jm-01", "jinming", "商业街手打柠檬茶：冰块比茶多", "夏天还是得靠它续命。", "柠檬精本精", 156, 63, ["饮品", "商业街", "夯"],
    { name: "手打柠檬茶（金明商业街）", lat: 34.8240, lng: 114.3105 }, "07-18 15:20",
    [
      { type: "p", text: "金明商业街新开的柠檬茶，现切现打，茶香和酸度都在线，¥12 一杯。" },
      { type: "quote", text: "锐评：冰给得比茶多是事实，点少冰刚好。打分：夯。" },
    ]),
  post("jm-02", "jinming", "北门胡辣汤：河南人的早八仪式感", "配两块钱的油馍头，满血进教室。", "胡辣汤守卫者", 203, 88, ["早餐", "北门", "夯"],
    { name: "方中山胡辣汤（北门）", lat: 34.8251, lng: 114.3062 }, "07-17 07:50",
    [
      { type: "p", text: "北门的胡辣汤早上六点半就开，牛肉片给得不抠门，辣度自己加。" },
      { type: "list", items: ["优质胡辣汤 ¥8", "油馍头 ¥2/份", "茶叶蛋 ¥1.5"] },
      { type: "quote", text: "锐评：早八前来一碗，一上午不饿。打分：夯。" },
    ]),
  post("jm-03", "jinming", "金明食堂一楼的糖醋里脊去哪了", "换了厨师之后一落千丈。", "你", 64, 12, ["食堂", "拉完了"],
    { name: "金明食堂一楼", lat: 34.8215, lng: 114.3088 }, "07-16 12:02",
    [
      { type: "p", text: "上学期的糖醋里脊是一楼招牌，这学期换了厨师，外壳软塌、酱汁发齁。" },
      { type: "quote", text: "锐评：从夯到拉只需要换一个厨师。打分：拉。" },
      { type: "p", text: "目前一楼能打的只剩早餐窗口。" },
    ], true),
  post("jm-04", "jinming", "东门黄焖鸡：稳定的及格线", "不知道吃什么时的保底选项。", "随缘干饭人", 88, 30, ["黄焖鸡", "东门", "NPC"],
    { name: "杨铭宇黄焖鸡（东门）", lat: 34.8233, lng: 114.3130 }, "07-15 18:44",
    [
      { type: "p", text: "连锁黄焖鸡，味道稳定没有惊喜也没有雷，小份 ¥18 加米饭管饱。" },
      { type: "quote", text: "锐评：不难吃，但也不值得专门跑一趟。打分：中。" },
    ]),
  post("jm-05", "jinming", "图书馆咖啡车：贵但离不开", "¥15 的美式，买的是不回宿舍的决心。", "DDL 战士", 121, 47, ["咖啡", "图书馆", "NPC"],
    { name: "咖啡车（图书馆西）", lat: 34.8220, lng: 114.3070 }, "07-14 14:30",
    [
      { type: "p", text: "图书馆西侧的咖啡车，美式 ¥15 比商业街贵三块，但胜在近，下楼三十秒。" },
      { type: "quote", text: "锐评：味道及格，主要功能是心理建设。打分：中。" },
    ]),

  // ---- 龙子湖校区 ----
  post("lz-01", "longzihu", "南门灌汤包：先开窗后喝汤", "皮薄汤足，小心烫嘴。", "汤包猎人", 178, 71, ["灌汤包", "南门", "夯"],
    { name: "第一楼灌汤包（南门）", lat: 34.8156, lng: 113.8301 }, "07-18 11:30",
    [
      { type: "p", text: "南门的灌汤包是开封做法，一笼八只 ¥16，先咬小口喝汤再吃肉。" },
      { type: "quote", text: "锐评：刚出笼的能烫掉一层嘴皮，等三分钟。打分：夯。" },
    ]),
  post("lz-02", "longzihu", "西区食堂的隐藏窗口：烩面", "本地人认证，汤是羊骨熬的。", "你", 145, 60, ["烩面", "食堂", "夯"],
    { name: "龙子湖西区食堂二楼", lat: 34.8180, lng: 113.8275 }, "07-17 12:20",
    [
      { type: "p", text: "西区食堂二楼最里面的烩面窗口，¥10 一碗，汤头奶白，海带丝和千张丝给得足。" },
      { type: "quote", text: "锐评：比校外 ¥18 的强。打分：夯。" },
    ], true),
  post("lz-03", "longzihu", "东门炸鸡排：油是昨天剩下的吗", "吃一次拉一次，怕了。", "肠胃科常客", 52, 9, ["炸鸡", "东门", "拉完了"],
    { name: "豪大鸡排（东门）", lat: 34.8168, lng: 113.8322 }, "07-15 19:40",
    [
      { type: "p", text: "东门炸鸡排闻着香，但油品明显不行，宿舍三个人吃完两个人拉肚子。" },
      { type: "quote", text: "锐评：从夯到拉，只说人话——这家是拉中拉。打分：拉。" },
    ]),
  post("lz-04", "longzihu", "地铁口煎饼果子：加两个蛋的奢侈", "通勤路上的固定节目。", "早八在逃人员", 99, 34, ["早餐", "地铁口", "顶级"],
    { name: "煎饼果子（地铁口 B 口）", lat: 34.8190, lng: 113.8310 }, "07-14 08:15",
    [
      { type: "p", text: "地铁口的煎饼摊，标准 ¥7 加蛋 ¥1.5，薄脆是现炸的，高峰期排十个人。" },
      { type: "quote", text: "锐评：稳定发挥的早餐，打分：中偏夯。" },
    ]),
  post("lz-05", "longzihu", "北苑螺蛳粉：整层楼都知道你吃了", "味道两极，爱的人天天来。", "你", 113, 41, ["螺蛳粉", "北苑", "NPC"],
    { name: "柳州螺蛳粉（北苑）", lat: 34.8202, lng: 113.8288 }, "07-13 18:05",
    [
      { type: "p", text: "北苑螺蛳粉，酸笋味道穿透力极强，堂食 ¥13，加鸭脚 ¥4。" },
      { type: "quote", text: "锐评：好吃是真好吃，回宿舍记得先洗头。打分：中。" },
    ], true),
];

const INITIAL_COMMENTS: Comment[] = [  { id: "c1", postId: "ml-01", author: "干饭组组长", time: "07-18 13:02", text: "油泼扯面 +1，辣子是真的香。" },
  { id: "c2", postId: "ml-01", author: "早八不迟到", time: "07-18 14:20", text: "昨天下午两点去果然卖完了，血泪教训。" },
  { id: "c3", postId: "ml-02", author: "柠檬精本精", time: "07-17 20:11", text: "微微辣选手报到，上次点微辣喝了三瓶酸梅汤。" },
  { id: "c4", postId: "ml-03", author: "虾仁猪心", time: "07-16 13:00", text: "今天阿姨手稳，肉量感人，建议去买彩票。" },
  { id: "c5", postId: "jm-02", author: "你", time: "07-17 08:30", text: "油馍头泡进去十秒再吃，懂的都懂。" },
  { id: "c6", postId: "jm-01", author: "DDL 战士", time: "07-18 16:40", text: "少冰+半糖，夏日图书馆标配。" },
  { id: "c7", postId: "lz-01", author: "汤包猎人", time: "07-18 12:05", text: "配一碗蛋花汤，人均 ¥20 封顶。" },
  { id: "c8", postId: "lz-03", author: "肠胃科常客", time: "07-15 21:12", text: "别抱侥幸心理，我已经替大家试过了。" },
];

// 与“河大新生手册”参考站点共用的真实餐食氛围图。它们是菜品参考图，
// 不冒充具体商家实拍；离线/失败时 Img 组件回退图纸占位块。
const FOOD_REFERENCE_IMAGES = {
  dumplings:
    "https://images.unsplash.com/photo-1563245372-f21724e3856d?auto=format&fit=crop&w=1200&q=82",
  colorfulDish:
    "https://images.unsplash.com/photo-1547592180-85f173990554?auto=format&fit=crop&w=1200&q=82",
  noodles:
    "https://images.unsplash.com/photo-1559314809-0d155014e29e?auto=format&fit=crop&w=1200&q=82",
  hotBowl:
    "https://images.unsplash.com/photo-1569718212165-3a8278d5f624?auto=format&fit=crop&w=1200&q=82",
  sharedMeal:
    "https://images.unsplash.com/photo-1515003197210-e0cd71810b5f?auto=format&fit=crop&w=1200&q=82",
} as const;

INITIAL_POSTS.find((p) => p.id === "ml-01")!.images = [FOOD_REFERENCE_IMAGES.hotBowl];
INITIAL_POSTS.find((p) => p.id === "ml-01")!.blocks.splice(2, 0, {
  type: "img",
  src: FOOD_REFERENCE_IMAGES.noodles,
});
INITIAL_POSTS.find((p) => p.id === "ml-02")!.images = [FOOD_REFERENCE_IMAGES.sharedMeal];
INITIAL_POSTS.find((p) => p.id === "jm-01")!.images = [FOOD_REFERENCE_IMAGES.colorfulDish];
INITIAL_POSTS.find((p) => p.id === "jm-02")!.images = [FOOD_REFERENCE_IMAGES.hotBowl];
INITIAL_POSTS.find((p) => p.id === "jm-02")!.blocks.splice(1, 0, {
  type: "img",
  src: FOOD_REFERENCE_IMAGES.colorfulDish,
});
INITIAL_POSTS.find((p) => p.id === "lz-01")!.images = [FOOD_REFERENCE_IMAGES.dumplings];
INITIAL_POSTS.find((p) => p.id === "lz-02")!.images = [FOOD_REFERENCE_IMAGES.noodles];

// ---------------------------------------------------------------- foodStore

export interface FoodData {
  posts: Post[];
  comments: Comment[];
}

const INITIAL: FoodData = { posts: INITIAL_POSTS, comments: INITIAL_COMMENTS };

/** 静态预生成用（generateStaticParams） */
export const STATIC_POSTS = INITIAL_POSTS;

let state: FoodData = INITIAL;
const listeners = new Set<() => void>();

function emit() {
  listeners.forEach((l) => l());
}

function set(patch: Partial<FoodData>) {
  state = { ...state, ...patch };
  emit();
}

let seq = 100; // 会话内新 id 递增

export const foodStore = {
  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },
  get: (): FoodData => state,
  getServer: (): FoodData => INITIAL,

  toggleLike(id: string) {
    set({
      posts: state.posts.map((p) =>
        p.id === id
          ? { ...p, liked: !p.liked, likes: p.likes + (p.liked ? -1 : 1) }
          : p
      ),
    });
  },

  toggleStar(id: string) {
    set({
      posts: state.posts.map((p) =>
        p.id === id
          ? { ...p, starred: !p.starred, stars: p.stars + (p.starred ? -1 : 1) }
          : p
      ),
    });
  },

  addComment(postId: string, author: string, text: string) {
    set({
      comments: [
        ...state.comments,
        { id: `c-new-${seq++}`, postId, author, time: "刚刚", text },
      ],
    });
  },

  addPost(input: Omit<Post, "id" | "likes" | "stars" | "time" | "hidden">) {
    const p: Post = {
      ...input,
      id: `p-new-${seq++}`,
      likes: 0,
      stars: 0,
      time: "刚刚",
      hidden: false,
    };
    set({ posts: [p, ...state.posts] });
    return p.id;
  },

  updatePost(id: string, patch: Partial<Post>) {
    set({ posts: state.posts.map((p) => (p.id === id ? { ...p, ...patch } : p)) });
  },

  toggleHidden(id: string) {
    set({
      posts: state.posts.map((p) => (p.id === id ? { ...p, hidden: !p.hidden } : p)),
    });
  },

  removePost(id: string) {
    set({
      posts: state.posts.filter((p) => p.id !== id),
      comments: state.comments.filter((c) => c.postId !== id),
    });
  },
};

// ---------------------------------------------------------------- mini-markdown 解析

/** `# ` → h2，`- ` → list，`> ` → quote，`![N]` → 第 N 张附件图，其余非空行 → p。不渲染原始 HTML。 */
export function parseMiniMd(src: string): PostBlock[] {
  const blocks: PostBlock[] = [];
  let list: string[] | null = null;
  const flush = () => {
    if (list && list.length) blocks.push({ type: "list", items: list });
    list = null;
  };
  for (const raw of src.split("\n")) {
    const line = raw.trimEnd();
    if (line.startsWith("- ")) {
      (list ??= []).push(line.slice(2).trim());
      continue;
    }
    flush();
    if (!line.trim()) continue;
    const imgMatch = line.match(/^!\[(\d+)\]$/);
    if (imgMatch) blocks.push({ type: "img", ref: Number(imgMatch[1]) });
    else if (line.startsWith("# ")) blocks.push({ type: "h2", text: line.slice(2).trim() });
    else if (line.startsWith("> ")) blocks.push({ type: "quote", text: line.slice(2).trim() });
    else blocks.push({ type: "p", text: line.trim() });
  }
  flush();
  return blocks;
}
