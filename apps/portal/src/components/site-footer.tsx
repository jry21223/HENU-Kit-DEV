import LegalNotice from "@/components/legal-notice";

/**
 * 子站、账户中心、登录页与协议页共用的页脚。首页用自己的大页脚（footer.tsx），
 * 两者的声明与链接都来自 LegalNotice。
 */
export default function SiteFooter() {
  return (
    <footer className="border-t border-line bg-paper">
      <div className="mx-auto max-w-[1440px] px-5 py-3 md:px-8 md:py-4">
        <LegalNotice />
      </div>
    </footer>
  );
}
