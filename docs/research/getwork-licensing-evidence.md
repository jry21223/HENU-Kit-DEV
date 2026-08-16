# `RyaoVen/getWork` licensing evidence

Checked: 2026-08-13 (Asia/Shanghai)

## Practical conclusion

Do not copy, vendor, modify, or redistribute `getWork` code or other repository material in HENU-Kit yet. The repository shows an MIT **intent signal**, but it does not currently contain the linked license text or another complete, explicit reusable grant. Continue only with evaluation, source attribution, and independently written design work while asking the owner to add a root license.

This is a conservative project-risk interpretation: a bare README label does not supply the license text, scope, conditions, or copyright notice that HENU-Kit would need to preserve. A standard root license removes that ambiguity.

## Observed evidence

The observations below are pinned to the default branch HEAD at the time checked:

- Repository: [`RyaoVen/getWork`](https://github.com/RyaoVen/getWork)
- Default branch: `master`
- HEAD: [`2c7800d65fb22d5094d812107c63ce94734b1c2e`](https://github.com/RyaoVen/getWork/commit/2c7800d65fb22d5094d812107c63ce94734b1c2e)
- Immutable tree: [tree at that SHA](https://github.com/RyaoVen/getWork/tree/2c7800d65fb22d5094d812107c63ce94734b1c2e)

| Evidence | Fact observed | Practical interpretation |
| --- | --- | --- |
| Public visibility | The [repository API](https://api.github.com/repos/RyaoVen/getWork) reports `visibility: "public"`. | Public visibility allows inspection and GitHub forking; [GitHub's licensing guidance](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/licensing-a-repository) distinguishes those platform rights from permission to reproduce, distribute, or create derivatives. It is therefore not by itself a reusable grant for HENU-Kit. |
| README | Lines 82–84 say `许可` and `[MIT](./LICENSE)` ([immutable blob](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/README.md#L82-L84)). The target `LICENSE` is absent from the same tree. | This indicates the owner's likely MIT intent, but the linked grant and its notice/conditions are unavailable. |
| Root/tree license files | The recursive HEAD tree contains no `LICENSE`, `LICENCE`, or `COPYING` file. | There is no repository license text for HENU-Kit to preserve and follow. |
| Git history | `master` has 14 reachable commits, from root commit `55a4fd34e899dabc06a61fb84ef14c4ecc0939ee` to the observed HEAD. The recursive tree for every one contains no `LICENSE`, `LICENCE`, or `COPYING` file. The README's MIT link first appears in [`bb2bf135f78f8cac253529c2de11c6efd60649d6`](https://github.com/RyaoVen/getWork/commit/bb2bf135f78f8cac253529c2de11c6efd60649d6) and remains at HEAD. | The missing license is not merely a deletion at current HEAD; no complete license file was found in the reachable default-branch history. |
| Package metadata | [`pyproject.toml`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/pyproject.toml#L1-L18) is the only Python/Node package metadata file in the HEAD tree and has no `license`, `license-files`, or license classifier. No `package.json` exists. | Package metadata supplies no additional grant or clarification. |
| GitHub detection | The [repository API](https://api.github.com/repos/RyaoVen/getWork) returned `default_branch: "master"` and `license: null`. The [repository license API](https://api.github.com/repos/RyaoVen/getWork/license) returned HTTP 404. | GitHub has not detected a repository license. This corroborates the missing-file evidence; detection itself does not decide legal rights. |

## Resolution requested from upstream

Preferred resolution: the owner adds the standard MIT text as `LICENSE` at the repository root, with an appropriate copyright line, and confirms it covers the repository contents. Re-check the exact commit before reuse and retain the resulting license and notices in any HENU-Kit copy or derivative.

### Owner request

With approval, the following concise request was sent by email on 2026-08-13 (Asia/Shanghai) to the public address on the owner's GitHub profile. No GitHub issue, pull request, or public comment was created.

**Subject:** 想确认一下 getWork 的 MIT 许可

嗨，RyaoVen：

我在 HENU-Kit 的「求职雷达」里评估复用 getWork 的部分代码。README 目前写了 MIT，但仓库根目录还没有 LICENSE 文件，pyproject.toml 里也没有许可证声明，所以我们暂时不敢把它当成明确的可复用授权。

方便的话，能否在仓库根目录补一个标准 MIT LICENSE（写明版权年份和版权人），确认你拥有版权的仓库内容可以按 MIT 使用、修改和再分发？如果你更希望采用别的许可证，也请告诉我。

仓库：https://github.com/RyaoVen/getWork

谢谢！  
Jerry

## Reproduction appendix

These read-only commands were used. The relevant GitHub API fields were `default_branch`, `license`, branch `commit.sha`, commit `parents`, and recursive tree `tree[].path`.

```sh
# Repository license status and default branch
curl -fsSL https://api.github.com/repos/RyaoVen/getWork \
  | jq '{default_branch, license}'

# Exact default-branch HEAD
curl -fsSL https://api.github.com/repos/RyaoVen/getWork/branches/master \
  | jq '{name, sha: .commit.sha}'

# README and package metadata at the observed SHA
sha=2c7800d65fb22d5094d812107c63ce94734b1c2e
curl -fsSL "https://raw.githubusercontent.com/RyaoVen/getWork/$sha/README.md" \
  | nl -ba | sed -n '82,84p'
curl -fsSL "https://raw.githubusercontent.com/RyaoVen/getWork/$sha/pyproject.toml" \
  | nl -ba | sed -n '1,40p'

# License-like paths at HEAD
curl -fsSL "https://api.github.com/repos/RyaoVen/getWork/git/trees/$sha?recursive=1" \
  | jq -r '.tree[].path' \
  | grep -Ei '(^|/)(license|licence|copying)(\.|$)'

# Repeat the recursive-tree path check for all reachable default-branch commits
for commit_sha in $(curl -fsSL \
  'https://api.github.com/repos/RyaoVen/getWork/commits?sha=master&per_page=100' \
  | jq -r '.[].sha'); do
  matches=$(curl -fsSL \
    "https://api.github.com/repos/RyaoVen/getWork/git/trees/$commit_sha?recursive=1" \
    | jq -r '.tree[].path' \
    | grep -Ei '(^|/)(license|licence|copying)(\.|$)' || true)
  printf '%s %s\n' "$commit_sha" "${matches:-NONE}"
done

# Confirm the oldest returned commit is the root (empty parents array)
curl -fsSL https://api.github.com/repos/RyaoVen/getWork/commits/55a4fd34e899dabc06a61fb84ef14c4ecc0939ee \
  | jq '{sha, parents: [.parents[].sha]}'

# GitHub detected-license endpoint; observed HTTP status: 404
curl -sS -o /dev/null -w '%{http_code}\\n' \
  https://api.github.com/repos/RyaoVen/getWork/license
```
