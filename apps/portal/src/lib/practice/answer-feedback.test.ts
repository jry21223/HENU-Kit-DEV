import { describe, expect, it } from "vitest";
import { choiceOptionState, judgeOptionState } from "./answer-feedback";

const options = ["甲", "乙", "丙", "丁"];

describe("choiceOptionState", () => {
  it("single: marks the chosen right option, the chosen wrong option and the missed right option apart", () => {
    // expected = index 1, user picked index 0
    expect(choiceOptionState({ selected: true, expectedAnswer: 1, option: options[0], index: 0 })).toBe("wrong");
    expect(choiceOptionState({ selected: false, expectedAnswer: 1, option: options[1], index: 1 })).toBe("missed");
    expect(choiceOptionState({ selected: false, expectedAnswer: 1, option: options[2], index: 2 })).toBe("neutral");
    expect(choiceOptionState({ selected: true, expectedAnswer: 1, option: options[1], index: 1 })).toBe("correct");
  });

  it("multi: a partial pick leaves the unpicked right option as missed instead of looking chosen", () => {
    // expected BD = [1, 3], user picked only B
    const expected = [1, 3];
    expect(choiceOptionState({ selected: true, expectedAnswer: expected, option: options[1], index: 1 })).toBe("correct");
    expect(choiceOptionState({ selected: false, expectedAnswer: expected, option: options[3], index: 3 })).toBe("missed");
    expect(choiceOptionState({ selected: true, expectedAnswer: expected, option: options[0], index: 0 })).toBe("wrong");
    expect(choiceOptionState({ selected: false, expectedAnswer: expected, option: options[2], index: 2 })).toBe("neutral");
  });

  it("accepts an expected answer given as option text", () => {
    expect(choiceOptionState({ selected: false, expectedAnswer: ["乙"], option: options[1], index: 1 })).toBe("missed");
  });
});

describe("judgeOptionState", () => {
  it("shows a wrong judge pick as wrong and the other side as the correct answer", () => {
    expect(judgeOptionState({ selected: true, expectedAnswer: false, value: true })).toBe("wrong");
    expect(judgeOptionState({ selected: false, expectedAnswer: false, value: false })).toBe("missed");
  });

  it("shows a right judge pick as correct and leaves the other side neutral", () => {
    expect(judgeOptionState({ selected: true, expectedAnswer: true, value: true })).toBe("correct");
    expect(judgeOptionState({ selected: false, expectedAnswer: true, value: false })).toBe("neutral");
  });
});
