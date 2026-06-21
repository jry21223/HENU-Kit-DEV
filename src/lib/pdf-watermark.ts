import { existsSync } from "node:fs";
import { readFile } from "node:fs/promises";
import fontkit from "@pdf-lib/fontkit";
import { degrees, PDFDocument, rgb, StandardFonts } from "pdf-lib";

const DEFAULT_WATERMARK_NOTICE = "Jerry 制作｜仅供个人复习使用｜禁止盗印与商用传播";
const FONT_CANDIDATES = [
  process.env.PDF_WATERMARK_FONT_PATH,
  "C:\\Windows\\Fonts\\simhei.ttf",
  "C:\\Windows\\Fonts\\msyh.ttc",
  "C:\\Windows\\Fonts\\simsun.ttc",
].filter(Boolean) as string[];

export type PdfWatermarkOptions = {
  userEmail?: string | null;
  downloadedAt?: Date;
  notice?: string;
};

export function isPdfFileName(fileName?: string | null) {
  return Boolean(fileName?.toLowerCase().endsWith(".pdf"));
}

export function buildWatermarkLines(options: PdfWatermarkOptions = {}) {
  const downloadedAt = options.downloadedAt ?? new Date();
  return [
    options.notice ?? DEFAULT_WATERMARK_NOTICE,
    `用户：${options.userEmail || "未登录用户"}｜下载时间：${downloadedAt.toISOString()}`,
  ];
}

async function findWatermarkFontPath() {
  return FONT_CANDIDATES.find((fontPath) => existsSync(fontPath));
}

export async function addPdfWatermark(
  pdfBytes: Uint8Array,
  options: PdfWatermarkOptions = {},
) {
  const pdfDoc = await PDFDocument.load(pdfBytes);
  const fontPath = await findWatermarkFontPath();
  const font = fontPath
    ? await (async () => {
        pdfDoc.registerFontkit(fontkit);
        return pdfDoc.embedFont(await readFile(fontPath), { subset: true });
      })()
    : await pdfDoc.embedFont(StandardFonts.Helvetica);

  const lines = buildWatermarkLines(options);
  const pages = pdfDoc.getPages();

  for (const page of pages) {
    const { width, height } = page.getSize();
    const fontSize = Math.max(10, Math.min(18, width / 34));
    const x = width * 0.11;
    const y = height * 0.58;

    page.drawText(lines[0], {
      x,
      y,
      size: fontSize,
      font,
      color: rgb(0.62, 0.62, 0.62),
      opacity: 0.24,
      rotate: degrees(-32),
    });
    page.drawText(lines[1], {
      x,
      y: y - fontSize * 1.6,
      size: fontSize * 0.78,
      font,
      color: rgb(0.62, 0.62, 0.62),
      opacity: 0.24,
      rotate: degrees(-32),
    });
  }

  return Buffer.from(await pdfDoc.save());
}
