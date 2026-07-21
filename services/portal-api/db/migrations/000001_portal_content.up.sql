-- Portal API content tables
-- These tables store user-facing content that doesn't exist in other services.

-- Food posts (consumer-facing food reviews)
CREATE TABLE IF NOT EXISTS portal_food_posts (
    id TEXT PRIMARY KEY,
    campus TEXT NOT NULL CHECK (campus IN ('minglun', 'jinming', 'longzihu')),
    title TEXT NOT NULL,
    excerpt TEXT NOT NULL DEFAULT '',
    blocks JSONB NOT NULL DEFAULT '[]',
    author TEXT NOT NULL,
    likes INTEGER NOT NULL DEFAULT 0,
    stars INTEGER NOT NULL DEFAULT 0,
    tags TEXT[] NOT NULL DEFAULT '{}',
    shop_name TEXT NOT NULL DEFAULT '',
    shop_lat DOUBLE PRECISION NOT NULL DEFAULT 0,
    shop_lng DOUBLE PRECISION NOT NULL DEFAULT 0,
    time TEXT NOT NULL,
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    images TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_food_posts_campus ON portal_food_posts(campus);
CREATE INDEX IF NOT EXISTS idx_food_posts_hidden ON portal_food_posts(hidden);

-- Food comments
CREATE TABLE IF NOT EXISTS portal_food_comments (
    id TEXT PRIMARY KEY,
    post_id TEXT NOT NULL REFERENCES portal_food_posts(id) ON DELETE CASCADE,
    author TEXT NOT NULL,
    time TEXT NOT NULL,
    text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_food_comments_post ON portal_food_comments(post_id);

-- Campus marketplace items
CREATE TABLE IF NOT EXISTS portal_campus_items (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN ('help', 'sell')),
    category TEXT NOT NULL,
    title TEXT NOT NULL,
    desc TEXT NOT NULL DEFAULT '',
    price DOUBLE PRECISION NOT NULL DEFAULT 0,
    seller TEXT NOT NULL,
    credit INTEGER NOT NULL DEFAULT 80,
    deals_done INTEGER NOT NULL DEFAULT 0,
    wants INTEGER NOT NULL DEFAULT 0,
    place TEXT NOT NULL DEFAULT '',
    deadline TEXT,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'ongoing', 'done', 'hidden')),
    time TEXT NOT NULL,
    images TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_campus_items_type ON portal_campus_items(type);
CREATE INDEX IF NOT EXISTS idx_campus_items_status ON portal_campus_items(status);
CREATE INDEX IF NOT EXISTS idx_campus_items_category ON portal_campus_items(category);

-- Campus item messages
CREATE TABLE IF NOT EXISTS portal_campus_messages (
    id TEXT PRIMARY KEY,
    item_id TEXT NOT NULL REFERENCES portal_campus_items(id) ON DELETE CASCADE,
    author TEXT NOT NULL,
    time TEXT NOT NULL,
    text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_campus_messages_item ON portal_campus_messages(item_id);

-- Seed data: Food posts (matching frontend mock)
INSERT INTO portal_food_posts (id, campus, title, excerpt, blocks, author, likes, stars, tags, shop_name, shop_lat, shop_lng, time) VALUES
('ml-01', 'minglun', '老碗面：十年不换配方的汤头', '西门外的续命面馆，期末周排队的全是熟面孔。',
 '[{"type":"p","text":"明伦西门外走到头那家老碗面，开了快十年，汤头是每天现熬的牛骨汤。"},{"type":"h2","text":"点什么"},{"type":"list","items":["招牌牛肉面 ¥14","油泼扯面 ¥10","卤蛋 ¥2"]},{"type":"quote","text":"锐评：饭点排队 15 分钟起步，但出餐快。打分：夯。"}]',
 '楼下的猫', 214, 96, ARRAY['面食','西门','夯'], '老碗面（西门店）', 34.8201, 114.3512, '07-18 12:40'),
('ml-02', 'minglun', '鸡公煲的微辣是谎言', '南门鸡公煲，点单请自觉降一档辣度。',
 '[{"type":"p","text":"南门这家鸡公煲是宿舍聚餐默认选项，分量够两个人，人均 ¥25 左右。"},{"type":"quote","text":"锐评：微辣约等于外面的中辣。打分：夯。"}]',
 '干饭组组长', 187, 74, ARRAY['鸡公煲','南门','夯'], '重庆鸡公煲（南门店）', 34.8158, 114.3556, '07-17 19:05'),
('jm-01', 'jinming', '商业街手打柠檬茶：冰块比茶多', '夏天还是得靠它续命。',
 '[{"type":"p","text":"金明商业街新开的柠檬茶，现切现打，¥12 一杯。"},{"type":"quote","text":"锐评：冰给得比茶多是事实，点少冰刚好。打分：夯。"}]',
 '柠檬精本精', 156, 63, ARRAY['饮品','商业街','夯'], '手打柠檬茶（金明商业街）', 34.8240, 114.3105, '07-18 15:20'),
('jm-02', 'jinming', '北门胡辣汤：河南人的早八仪式感', '配两块钱的油馍头，满血进教室。',
 '[{"type":"p","text":"北门的胡辣汤早上六点半就开，牛肉片给得不抠门。"},{"type":"list","items":["优质胡辣汤 ¥8","油馍头 ¥2/份","茶叶蛋 ¥1.5"]},{"type":"quote","text":"锐评：早八前来一碗，一上午不饿。打分：夯。"}]',
 '胡辣汤守卫者', 203, 88, ARRAY['早餐','北门','夯'], '方中山胡辣汤（北门）', 34.8251, 114.3062, '07-17 07:50'),
('lz-01', 'longzihu', '南门灌汤包：先开窗后喝汤', '皮薄汤足，小心烫嘴。',
 '[{"type":"p","text":"南门的灌汤包是开封做法，一笼八只 ¥16。"},{"type":"quote","text":"锐评：刚出笼的能烫掉一层嘴皮，等三分钟。打分：夯。"}]',
 '汤包猎人', 178, 71, ARRAY['灌汤包','南门','夯'], '第一楼灌汤包（南门）', 34.8156, 113.8301, '07-18 11:30'),
('lz-02', 'longzihu', '西区食堂的隐藏窗口：烩面', '本地人认证，汤是羊骨熬的。',
 '[{"type":"p","text":"西区食堂二楼最里面的烩面窗口，¥10 一碗，汤头奶白。"},{"type":"quote","text":"锐评：比校外 ¥18 的强。打分：夯。"}]',
 '你', 145, 60, ARRAY['烩面','食堂','夯'], '龙子湖西区食堂二楼', 34.8180, 113.8275, '07-17 12:20')
ON CONFLICT (id) DO NOTHING;

-- Seed data: Food comments
INSERT INTO portal_food_comments (id, post_id, author, time, text) VALUES
('c1', 'ml-01', '干饭组组长', '07-18 13:02', '油泼扯面 +1，辣子是真的香。'),
('c2', 'ml-01', '早八不迟到', '07-18 14:20', '昨天下午两点去果然卖完了，血泪教训。'),
('c3', 'ml-02', '柠檬精本精', '07-17 20:11', '微微辣选手报到，上次点微辣喝了三瓶酸梅汤。'),
('c5', 'jm-02', '你', '07-17 08:30', '油馍头泡进去十秒再吃，懂的都懂。'),
('c7', 'lz-01', '汤包猎人', '07-18 12:05', '配一碗蛋花汤，人均 ¥20 封顶。')
ON CONFLICT (id) DO NOTHING;

-- Seed data: Campus items
INSERT INTO portal_campus_items (id, type, category, title, desc, price, seller, credit, deals_done, wants, place, deadline, status, time) VALUES
('h-01', 'help', 'express', '代取中通快递 3 件到 6 号楼', '快递在明伦西门菜鸟驿站，三个小件。', 3, '取快递困难户', 86, 12, 45, '明伦校区 · 西门驿站', '今天 18:00 前', 'open', '07-19 10:20'),
('h-02', 'help', 'luggage', '开学搬行李上六楼（无电梯）', '两个 28 寸行李箱 + 一个编织袋。', 15, '你', 91, 3, 28, '金明校区 · 桃李园', '本周五下午', 'open', '07-19 09:05'),
('h-03', 'help', 'skill', '代做数据结构课程小项目（哈夫曼编码）', '课程小项目：实现哈夫曼编码/解码。', 80, 'DDL 战士', 79, 31, 67, '线上交付', '下周五 23:59 前', 'ongoing', '07-18 22:40'),
('s-01', 'sell', 'flea', '九成新机械键盘 青轴 87 键', '用了三个月，无打油无暗病，箱说全。', 120, '你', 91, 3, 58, '金明校区 · 可送到楼下', NULL, 'open', '07-18 15:30'),
('s-02', 'sell', 'flea', '考研英语红宝书 全新未拆封', '25 版红宝书，买重复了。', 25, '考研上岸ing', 95, 27, 41, '明伦校区', NULL, 'open', '07-18 11:10'),
('s-03', 'sell', 'flea', '宿舍小冰箱 46L 用了一年', '制冷正常，无异味，毕业出。', 180, '毕业清仓中', 88, 19, 73, '明伦校区 · 自提', NULL, 'done', '07-17 18:25')
ON CONFLICT (id) DO NOTHING;

-- Seed data: Campus messages
INSERT INTO portal_campus_messages (id, item_id, author, time, text) VALUES
('m1', 'h-01', '顺路侠', '07-19 10:35', '我中午正好要去驿站，可以接。'),
('m2', 'h-01', '取快递困难户', '07-19 10:40', '好！我私你取件码。'),
('m3', 's-01', '键盘侠', '07-18 16:02', '什么牌子的？轴体是樱桃青吗？'),
('m4', 's-01', '你', '07-18 16:20', '国产轴，手感接近青轴，介意勿拍。')
ON CONFLICT (id) DO NOTHING;
