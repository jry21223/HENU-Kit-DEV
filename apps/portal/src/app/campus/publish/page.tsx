import CampusComingSoon from "@/components/campus/coming-soon";

export default function PublishPage() {
  return (
    <CampusComingSoon
      index="M-03"
      en="PUBLISH"
      title="发布单子"
      badge="发单入口"
      description="可以在互助平台广场直接点击「发布新单子」，发布同校互助求助或闲置信息，并附上最多 6 张图片；发布前也可以先设置「联系方式簿」，待双方同意后再交换联系渠道。"
      note="请勿在单子标题、说明、地点或图片中填写私人联系方式；互助平台不提供在线资金托管或支付结算。"
      ctaLabel="前往互助平台发布"
    />
  );
}
