import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import App from "./App.vue";

describe("Console Overview", () => {
  it("shows exactly the six accepted modules and every degradation state", () => {
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App, { attachTo: document.body });

    expect(wrapper.findAll("[data-module-card]")).toHaveLength(6);
    for (const name of ["Portal", "Platform Operations", "Notice", "Library", "QuizCraft", "Food"]) {
      expect(wrapper.text()).toContain(name);
    }
    for (const state of ["empty", "partial", "stale", "unavailable", "denied"]) {
      expect(wrapper.find(`[data-state="${state}"]`).exists()).toBe(true);
    }
    expect(wrapper.text()).not.toContain("积分");
    expect(wrapper.text()).not.toContain("会员");
    wrapper.unmount();
  });

  it("exposes a real loading state without inventing metrics", () => {
    window.history.replaceState({}, "", "/?scenario=loading");
    const wrapper = mount(App);

    expect(wrapper.find("section[aria-busy='true']").exists()).toBe(true);
    expect(wrapper.findAll("[data-state='loading']")).toHaveLength(6);
    expect(wrapper.findAll(".metric-tile")).toHaveLength(0);
  });
});
