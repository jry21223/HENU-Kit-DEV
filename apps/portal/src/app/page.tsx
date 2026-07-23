import Navbar from "@/components/navbar";
import Hero from "@/components/hero";
import SectionLibrary from "@/components/section-library";
import SectionPractice from "@/components/section-practice";
import SectionFood from "@/components/section-food";
import SectionCampus from "@/components/section-campus";
import Footer from "@/components/footer";
import SnapScroll from "@/components/snap-scroll";

export default function Home() {
  return (
    <>
      <SnapScroll />
      <Navbar />
      <main>
        <Hero />
        <SectionLibrary />
        <SectionPractice />
        <SectionFood />
        <SectionCampus />
      </main>
      <Footer />
    </>
  );
}
