import SectionHeading from "@/components/ui/section-heading";

export default function CareerPage() {
  return (
    <main className="mx-auto max-w-7xl px-5 py-16 md:px-10">
      <SectionHeading index="05" en="WORK RADAR" title="求职雷达" />
      <p className="mt-6 max-w-xl text-sm leading-7 text-ink/70">
        设定求职画像，后台异步扫描受控招聘来源，匹配结果与命中原因一目了然，
        完成后自动发送结果简报至当前账户邮箱。本模块正在接入，即将上线。
      </p>
    </main>
  );
}
