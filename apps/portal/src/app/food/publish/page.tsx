import Link from "next/link";

const FOOD_DESK_URL =
  "https://henu-campus-guide.cocoa-brush-7952.chatgpt.site/#food-submit";

const REQUIRED_FACTS = [
  {
    index: "01",
    label: "商家名称",
    detail: "写到具体窗口或门店，避免只填“食堂二楼”。",
  },
  {
    index: "02",
    label: "校区与具体位置",
    detail: "校区、地址、楼层、窗口号和可选地图链接。",
  },
  {
    index: "03",
    label: "最近到店时间",
    detail: "告诉维护者信息有多新，过期资料不会直接上榜。",
  },
  {
    index: "04",
    label: "价格与营业参考",
    detail: "人均、单品价格和大致营业时间；不确定就留空。",
  },
  {
    index: "05",
    label: "为什么推荐",
    detail: "味道、分量、性价比、排队情况和适合场景。",
  },
  {
    index: "06",
    label: "推荐菜品",
    detail: "菜名、参考价格和推荐理由，可附最多 6 张图片链接。",
  },
] as const;

export default function FoodPublishPage() {
  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8 md:py-14">
      <div className="grid gap-12 lg:grid-cols-[minmax(0,1fr)_22rem] lg:gap-16">
        <div>
          <p className="font-mono text-xs tracking-[0.3em] text-ink/55">
            <span className="text-accent">F-03</span>
            <span className="mx-2">/</span>
            STUDENT FOOD DESK
          </p>
          <h1 className="mt-5 max-w-[15ch] font-display text-5xl font-bold leading-[0.92] tracking-[-0.05em] md:text-7xl">
            你吃到的好店，投到这里。
          </h1>
          <p className="mt-6 max-w-[64ch] text-lg leading-8 text-ink/65">
            推荐不会直接改榜单。线索先进入学生美食台的待审核队列，维护者核验地址、图片与时效后，再决定是否加入地图和“从夯到拉”五档榜。
          </p>

          <div className="mt-10 grid gap-px border border-line bg-line md:grid-cols-3">
            {[
              ["01", "写清楚是哪家", "店名、校区和具体位置最重要。"],
              ["02", "讲明白为什么", "价格、分量、排队和推荐菜都可以。"],
              ["03", "等待学生核验", "投稿不会未经审核直接上榜。"],
            ].map(([index, title, copy]) => (
              <section key={index} className="bg-paper p-5">
                <p className="font-display text-4xl font-bold text-accent">
                  {index}
                </p>
                <h2 className="mt-4 font-display text-lg font-bold">{title}</h2>
                <p className="mt-2 text-sm leading-6 text-ink/60">{copy}</p>
              </section>
            ))}
          </div>

          <section className="mt-12">
            <p className="font-mono text-[10px] tracking-[0.28em] text-accent">
              BEFORE YOU SUBMIT
            </p>
            <h2 className="mt-2 font-display text-3xl font-bold">
              投稿前准备这些信息
            </h2>
            <div className="mt-6 grid border-t border-ink md:grid-cols-2">
              {REQUIRED_FACTS.map((fact) => (
                <article
                  key={fact.index}
                  className="grid grid-cols-[3rem_1fr] gap-3 border-b border-line py-5 md:odd:pr-6 md:even:border-l md:even:pl-6"
                >
                  <span className="font-display text-2xl font-bold text-accent">
                    {fact.index}
                  </span>
                  <div>
                    <h3 className="font-display text-lg font-bold">
                      {fact.label}
                    </h3>
                    <p className="mt-1 text-sm leading-6 text-ink/60">
                      {fact.detail}
                    </p>
                  </div>
                </article>
              ))}
            </div>
          </section>
        </div>

        <aside className="lg:sticky lg:top-24 lg:h-fit">
          <section className="border border-ink p-6">
            <p className="font-mono text-[10px] tracking-[0.25em] text-accent">
              REAL SUBMISSION QUEUE
            </p>
            <h2 className="mt-3 font-display text-2xl font-bold">
              前往学生美食台
            </h2>
            <p className="mt-3 text-sm leading-6 text-ink/65">
              当前 HENU Kit 使用已经上线的学生美食台接收和跟踪投稿。登录后可提交推荐并查看审核状态。
            </p>
            <a
              href={FOOD_DESK_URL}
              target="_blank"
              rel="noreferrer"
              className="mt-6 block bg-ink px-5 py-3 text-center font-mono text-xs tracking-[0.12em] text-paper transition-colors hover:bg-accent"
            >
              登录学生美食台投稿 ↗
            </a>
            <Link
              href="/food/leaderboard"
              className="mt-3 block border border-ink px-5 py-3 text-center font-mono text-xs tracking-[0.12em] transition-colors hover:bg-ink hover:text-paper"
            >
              先看看五档榜
            </Link>
          </section>

          <section className="mt-5 border border-accent/50 bg-accent/5 p-5">
            <p className="font-mono text-[10px] tracking-[0.22em] text-accent">
              PRIVACY / 隐私提醒
            </p>
            <p className="mt-3 text-sm leading-6 text-ink/65">
              不要填写店主私人手机号、微信或学生个人信息；照片请使用自己的实拍或有授权的公开链接。
            </p>
          </section>
        </aside>
      </div>
    </main>
  );
}
