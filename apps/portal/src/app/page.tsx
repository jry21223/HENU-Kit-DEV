import type { Metadata } from "next";
import Navbar from "@/components/navbar";
import Hero from "@/components/hero";
import SectionLibrary from "@/components/section-library";
import SectionPractice from "@/components/section-practice";
import SectionFood from "@/components/section-food";
import SectionCampus from "@/components/section-campus";
import SectionCareer from "@/components/section-career";
import Footer from "@/components/footer";
import SnapScroll from "@/components/snap-scroll";
import { homeMetadata } from "@/lib/seo";

export const metadata: Metadata = homeMetadata;

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
        <SectionCareer />
      </main>
      <Footer />
    </>
  );
}
