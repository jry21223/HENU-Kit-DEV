import { IBM_Plex_Mono, Space_Grotesk } from "next/font/google";

// 根布局与 global-error.tsx（它替换根布局、自带 <html>）共用这一组字体实例。
const spaceGrotesk = Space_Grotesk({
  variable: "--font-space-grotesk",
  subsets: ["latin"],
});

const plexMono = IBM_Plex_Mono({
  variable: "--font-plex-mono",
  subsets: ["latin"],
  weight: ["400", "500"],
});

/** 挂在 <html> 上的字体变量，globals.css 的字体栈都引用它们。 */
export const fontVariables = `${spaceGrotesk.variable} ${plexMono.variable}`;
