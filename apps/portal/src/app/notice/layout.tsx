import Navbar from "@/components/navbar";
import Footer from "@/components/footer";

export default function NoticeLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-svh bg-paper text-ink">
      <Navbar />
      {children}
      <Footer />
    </div>
  );
}
