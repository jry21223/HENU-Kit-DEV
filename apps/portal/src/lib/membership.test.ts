import { describe, expect, it } from "vitest";
import { LIFETIME_BENEFITS } from "./membership";

describe("终身会员开通与权益说明", () => {
  it("保留现有权益并说明四项校园服务无需再次付费", () => {
    for (const benefit of ["期末押题卷", "求职雷达", "雨课堂", "U校园", "AI版", "学习通", "图书馆定时预约"]) {
      expect(LIFETIME_BENEFITS).toContain(benefit);
    }
    expect(LIFETIME_BENEFITS).toContain("服务费用已包含");
    expect(LIFETIME_BENEFITS).toContain("无需额外付费");
    expect(LIFETIME_BENEFITS).toContain("绑定 HENU KIT 账号");
  });
});
