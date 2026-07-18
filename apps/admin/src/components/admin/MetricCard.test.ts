import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import MetricCard from "./MetricCard.vue";
import type { DashboardCard } from "../../lib/admin-api";

const card: DashboardCard = {
  domain: "notice",
  title: "校园通知",
  status: "not_integrated",
  primary_metric: { code: "notice_imported_today", label: "今日导入", value: null, previous_value: null, change_rate: null, definition_version: "v1", as_of: "2026-07-18T08:00:00Z" },
  metrics: [],
  as_of: "2026-07-18T08:00:00Z",
  last_success_at: null,
  action_path: "/notices",
  message: "尚未接入",
};

describe("MetricCard", () => {
  it("shows an honest placeholder and disables the action for an unintegrated domain", () => {
    const wrapper = mount(MetricCard, { props: { card }, global: { stubs: { RouterLink: true } } });
    expect(wrapper.text()).toContain("未接入");
    expect(wrapper.text()).toContain("—");
    expect(wrapper.text()).toContain("尚无处理入口");
    expect(wrapper.find("router-link-stub").exists()).toBe(false);
  });
});
