---
name: subject-final-review
description: Generate a complete, subject-agnostic final-exam review package from sample exams, recalled papers, notes, exercises, outlines, or instructor review materials. Use for any course, including math, physics, computer science, languages, humanities, social sciences, economics, management, law, medicine, and interdisciplinary subjects.
---

# Subject Final Review Package Skill

## Purpose

Use this skill to turn course materials into a complete final-exam review package. The output is not a single mock exam. It is a structured exam-prep product containing:

1. A review-oriented knowledge-point handout.
2. A prioritized exam topic list.
3. High-frequency question-type templates.
4. Mistake traps and scoring reminders.
5. Two newly authored mock exams.
6. Complete answer keys and explanations for both exams.
7. Render-ready LaTeX/PDF deliverables and a ZIP package when requested.

This skill is deliberately subject-agnostic. Do not assume the course is mathematics, programming, physics, English, or any specific discipline unless the supplied materials say so.

## Inputs to Accept

The user may provide any combination of:

- Past exam papers, recalled exams, sample papers, screenshots, scans, PDFs, Word files, Markdown, HTML, or ZIP files.
- Course outlines, knowledge summaries, instructor-highlighted topics, lecture notes, slides, lab sheets, exercises, answer keys, or textbook problem lists.
- Extra constraints such as course name, school, college, major, grade, semester, exam type, total score, duration, difficulty, open-book/closed-book mode, or output style.

Use all available materials, not only file names or the first file. When materials conflict, follow the newest user message and newest uploaded material.

## Core Rules

- Preserve unknowns as `未明确`; do not invent school, teacher, semester, duration, score ratio, or exam rules.
- Treat the sample exam's question types, question counts, and score structure as hard constraints for mock exams.
- Do not copy old questions directly.
- Do not merely change numbers, variable names, option order, proper nouns, or surface wording.
- Preserve the style, difficulty, scoring rhythm, and common methods of the sample exam while authoring genuinely new questions.
- Make the two mock exams complementary: different topic allocation, similar overall difficulty.
- Every question must have an answer. Every non-trivial answer must have an explanation.
- For calculation-heavy subjects, show derivation. For programming subjects, include key code logic, expected output, and explanation. For concept-heavy subjects, give judgment criteria and wording templates. For case/essay subjects, provide point structure and scoring logic.
- Include a quality check before final delivery.
- Remove personal or sensitive information from reusable templates: names, student IDs, phone numbers, emails, school-internal data, tokens, private payment data, and unauthorized copyrighted materials.

## Workflow

### 1. Material Analysis

Before generating final content, analyze the supplied materials.

Extract course metadata:

- Course name.
- School / college / major / grade.
- Academic year and semester.
- Exam type: closed-book, open-book, recalled version, mock, etc.
- Total score.
- Exam duration.
- Usual score split between coursework and final exam.
- Difficulty level: public course, major core course, A/B level, advanced/easy, etc.

Then identify the sample exam structure:

| Question type | Count | Points each | Total points | Notes |
| --- | ---: | ---: | ---: | --- |
| Fill from source | Fill | Fill | Fill | Concepts, computation, cases, code, essays, etc. |

This table is a hard constraint for both mock exams.

Build a topic matrix:

| Module | How it appears in samples | Question location | Difficulty | Priority |
| --- | --- | --- | --- | --- |
| Module name | Method or concept tested | Choice/fill/calculation/essay/etc. | Basic/medium/hard | Must-test/high-frequency/possible/overview |

### 2. Review Handout

Generate a handout titled:

`《课程名称》考前复习知识点讲义`

The handout must be exam-oriented, not a textbook summary. Include:

1. Exam scope overview.
2. Question-type and score structure.
3. Topic priority ranking.
4. Four-level classification: must-test core, high-frequency focus, possible topics, overview only.
5. Core formulas, rules, definitions, concepts, procedures, or argument structures for every major topic.
6. Typical testing methods for each major topic.
7. Step-by-step solving templates for each high-frequency question type.
8. Common mistakes and scoring deductions.
9. Final 3--7 day review plan.
10. Last-minute checklist.

Use discipline-appropriate language. For example:

- Mathematics: formulas, conditions, transformations, proof and computation steps.
- Physics/engineering: laws, units, diagrams, modeling assumptions, sign conventions.
- Programming/computer science: algorithms, data structures, API behavior, code reading, outputs, complexity, edge cases.
- Humanities/social sciences: definitions, comparison tables, argument templates, case-analysis frames, quote/keyword reminders.
- Economics/management/law: concepts, frameworks, case facts, rule application, calculation models, scoring keywords.
- Language courses: grammar points, reading skills, writing templates, translation traps, listening or cloze patterns.

### 3. Mock Exam Planning

Before writing questions, create a two-exam topic allocation table:

| Topic | Mock Exam 1 | Mock Exam 2 |
| --- | --- | --- |
| Topic A | Tested by one method | Tested by another method |
| Topic B | Appears in small question | Appears in large question |

The two exams should not be parallel clones. They should share the same sample-exam structure but differ in topic placement, question context, data, examples, cases, functions, code snippets, texts, or scenarios.

### 4. Mock Exam Generation

Generate two files conceptually:

- `《课程名称》_模拟卷一_含答案解析.pdf`
- `《课程名称》_模拟卷二_含答案解析.pdf`

For each exam:

- Follow the sample exam's question types, counts, and score structure exactly.
- Keep the total score consistent with the sample or user-stated total.
- Include clear instructions, section points, and per-question point values.
- Author new questions from the same knowledge map, not copied questions.
- Balance basic, medium, and difficult questions according to the sample.
- Include answer explanations directly after the exam or in a clearly separated answer section.

### 5. Answer Explanation Standard

For choice questions:

- Correct option.
- Judgment basis.
- Exclusion reason when useful.

For fill-in-the-blank questions:

- Final answer.
- Key calculation, rule, definition, or reasoning.

For calculation / proof / programming / case / essay questions:

1. Problem-solving idea.
2. Key formula, rule, concept, algorithm, legal/management framework, or argument structure.
3. Main steps.
4. Final answer.
5. Easy-mistake reminder.

### 6. LaTeX/PDF Output

When asked to render files, use the provided LaTeX templates or an equivalent clean template.

Minimum output files:

```txt
《课程名称》_考前复习知识点讲义.pdf
《课程名称》_模拟卷一_含答案解析.pdf
《课程名称》_模拟卷二_含答案解析.pdf
《课程名称》_期末复习包.zip
```

The ZIP should include the three PDFs. If source files are requested, include `.tex` files in a separate source folder.

Watermark should be configurable. Use a neutral default in public templates:

```txt
仅供个人复习使用｜请勿商用传播
```

Use a personal or organization watermark only when the user explicitly requests it for a private deliverable.

### 7. Quality Gate

Before final reply, verify:

1. Latest sample exam and latest user instruction were used.
2. Course metadata is extracted or marked `未明确`.
3. Question types and score structure match the sample.
4. Each mock exam total score is correct.
5. Every question has an answer.
6. Non-trivial questions have full explanations.
7. The review handout is priority-based, not a flat textbook summary.
8. High-frequency question-type templates are included.
9. Mistake traps are included.
10. A 3-day and/or 7-day review plan is included.
11. No old question is copied directly.
12. Questions are not just number-swapped or variable-renamed.
13. The two exams have different topic allocation but similar difficulty.
14. PDFs open successfully after rendering.
15. File names follow the required naming convention.
16. ZIP contains all required PDFs.
17. Public template files contain no personal, school-internal, or credential-like sensitive information.

## Final Response Format

When the package is complete, reply in Chinese by default:

```md
已完成本课程期末复习包，包含：
1. 考前复习知识点讲义
2. 模拟卷一及答案解析
3. 模拟卷二及答案解析
4. 汇总 ZIP

下载链接：
- 复习讲义 PDF：...
- 模拟卷一 PDF：...
- 模拟卷二 PDF：...
- 汇总 ZIP：...
```

If any scan page, image, formula, table, missing page, or question structure could not be recognized, state that explicitly and explain the fallback used.
