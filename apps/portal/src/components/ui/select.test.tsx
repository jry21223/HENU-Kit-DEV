import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Select } from "./select";

describe("Select", () => {
  it("renders a rectilinear native select with appearance-none and custom chevron", () => {
    const html = renderToStaticMarkup(
      <Select aria-label="排序" defaultValue="default">
        <option value="default">默认</option>
        <option value="latest">最新发布</option>
      </Select>
    );

    expect(html).toContain("relative inline-flex");
    expect(html).toContain("appearance-none");
    expect(html).toContain("rounded-none");
    expect(html).toContain("pr-9");
    expect(html).toContain("min-h-11");
    expect(html).toContain("bg-paper-raised");
    expect(html).toMatch(/<svg[^>]*aria-hidden="true"[^>]*class="[^"]*pointer-events-none/);
  });
});
