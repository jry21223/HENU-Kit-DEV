import BackLink from "@/components/back-link";
import SiteShell from "@/components/site-shell";

export default function LegalLayout({ children }: { children: React.ReactNode }) {
  return (
    <SiteShell
      header={
        <header className="sticky top-0 z-40 border-b border-line bg-paper">
          <div className="mx-auto flex h-14 max-w-[1440px] items-center gap-4 px-5 md:px-8">
            <BackLink className="inline-flex min-h-11 items-center" />
            <span className="font-display text-base font-bold tracking-tight">LEGAL</span>
          </div>
        </header>
      }
    >
      {children}
    </SiteShell>
  );
}
