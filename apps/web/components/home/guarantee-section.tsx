import { guaranteeItems } from "./home-data";

export function GuaranteeSection() {
  return (
    <section id="guarantee" aria-labelledby="guarantee-title" className="mx-auto w-[min(1120px,calc(100%-32px))] py-20">
      <div className="rounded-[2rem] border border-[#2b2117]/12 bg-[#f8efe2] p-6 lg:p-10">
        <div className="max-w-2xl">
          <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Guarantee</p>
          <h2 id="guarantee-title" className="mt-3 text-4xl font-black tracking-tight text-[#2b2117] sm:text-5xl">
            资料保障要讲清楚
          </h2>
          <p className="mt-5 text-sm leading-7 text-[#6f604f]">
            资料稳定供应、轻水印、权限校验和审核边界会分开表达，下载权限和生成内容都进入明确的校验流程。
          </p>
        </div>

        <div className="mt-8 grid gap-4 md:grid-cols-4">
          {guaranteeItems.map((item) => {
            const Icon = item.icon;

            return (
              <article key={item.title} className="rounded-3xl border border-[#2b2117]/10 bg-white/70 p-5">
                <Icon className="size-6 text-[#2f6b58]" aria-hidden={true} />
                <h3 className="mt-4 text-lg font-black tracking-tight text-[#2b2117]">{item.title}</h3>
                <p className="mt-3 text-sm leading-7 text-[#6f604f]">{item.body}</p>
              </article>
            );
          })}
        </div>
      </div>
    </section>
  );
}
