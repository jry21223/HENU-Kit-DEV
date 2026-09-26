import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import Img from "./img";

/**
 * 用户上传图片默认懒加载、异步解码（#548）：列表一次渲染十几张卡片时，手机只下载
 * 滚动到附近的图。首屏关键图由调用方显式改为立即加载、高优先级。
 */
describe("Img", () => {
  it("loads lazily and decodes off the main thread by default", () => {
    const html = renderToStaticMarkup(<Img src="/photo.jpg" alt="店铺照片" className="h-16 w-full" />);

    expect(html).toMatch(/<img[^>]*loading="lazy"/);
    expect(html).toMatch(/<img[^>]*decoding="async"/);
    expect(html).not.toMatch(/fetchpriority/i);
  });

  it("lets a first-screen image load eagerly with high priority", () => {
    const html = renderToStaticMarkup(
      <Img src="/photo.jpg" alt="店铺照片" className="h-16 w-full" loading="eager" fetchPriority="high" />
    );

    expect(html).toMatch(/<img[^>]*loading="eager"/);
    expect(html).toMatch(/<img[^>]*fetchpriority="high"/i);
    expect(html).toMatch(/<img[^>]*decoding="async"/);
  });
});
