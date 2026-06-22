import { ArchiveBookReveal } from "./archive-book-reveal";
import { CommunityStickyNotes } from "./community-sticky-notes";
import { FinalHomeCta } from "./final-home-cta";
import { GuaranteeSection } from "./guarantee-section";
import { HeroIntro } from "./hero-intro";
import { HomeNav } from "./home-nav";
import { MembershipTicketSection } from "./membership-ticket-section";
import { MobileArchiveIntro } from "./mobile-archive-intro";
import { PracticeVisionSection } from "./practice-vision-section";
import { SalesAssistantNote } from "./sales-assistant-note";

export function HomePage() {
  return (
    <main className="home-page min-h-[100dvh] overflow-x-clip text-[#2b2117]">
      <HomeNav />
      <HeroIntro />
      <ArchiveBookReveal />
      <MobileArchiveIntro />
      <CommunityStickyNotes />
      <PracticeVisionSection />
      <MembershipTicketSection />
      <SalesAssistantNote />
      <GuaranteeSection />
      <FinalHomeCta />
      <footer className="mx-auto w-[min(1120px,calc(100%-32px))] py-10 text-center text-xs text-[#85745f]">
        软件学院资料库 / 课程资料、刷题、共创和资料保障
      </footer>
    </main>
  );
}
