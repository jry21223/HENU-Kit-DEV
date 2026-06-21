import { redirect } from "next/navigation";

export default function MePage() {
  redirect("/me/wrong-questions");
}
