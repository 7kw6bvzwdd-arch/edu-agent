# GitHub 产品 UI Skills 调研

调研日期：2026-10-15。依据 GitHub 仓库当前的 README、SKILL.md 与其引用的规则文件，未实际安装或运行这些第三方技能。下面的推荐属于用途匹配判断，不是生成效果的对比测试。

## 推荐结论

对于教育产品的工作台、课件编辑器、任务列表、成员与额度页面，优先考虑 **Impeccable + Web Design Guidelines**：前者负责信息架构、界面改版和状态细节，后者负责实现后的交互与无障碍检查。需要从头整理色彩、字体、布局和组件规范时，再用 **UI UX Pro Max**。希望形成更鲜明的视觉方向时，可选择 **Anthropic frontend-design**。

不要把几个重叠的设计技能同时当作最高指令。每次选一个主要设计技能，再配一个审查技能，保留项目现有组件与技术栈。

| Skill | GitHub 与实际文件 | 主要用途 | 适合本项目的场景 |
| --- | --- | --- | --- |
| **Impeccable** | [仓库](https://github.com/pbakaus/impeccable) · [SKILL.md](https://github.com/pbakaus/impeccable/blob/main/.agents/skills/impeccable/SKILL.md) | 从交互规划到已有界面的审查、改版、打磨、文案和异常状态完善 | 重整课件工作台、编辑器工具栏、首次使用引导、空状态与失败恢复 |
| **UI UX Pro Max** | [仓库](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill) · [SKILL.md](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill/blob/main/.claude/skills/ui-ux-pro-max/SKILL.md) | 可检索的产品设计、配色、排版、图表、交互与技术栈指导 | 新建或统一产品设计规范；任务列表、表单、导航、统计图表设计 |
| **frontend-design** | [Anthropic 官方技能仓库](https://github.com/anthropics/skills) · [SKILL.md](https://github.com/anthropics/skills/blob/main/skills/frontend-design/SKILL.md) | 形成有意图的视觉方向，并将其落实为前端页面 | 新页面的布局、字体和视觉层级；改善过于模板化的界面 |
| **web-design-guidelines** | [Vercel 官方技能仓库](https://github.com/vercel-labs/agent-skills) · [SKILL.md](https://github.com/vercel-labs/agent-skills/blob/main/skills/web-design-guidelines/SKILL.md) | 按 Web Interface Guidelines 审查 UI 代码，输出具体文件与行号的问题 | 检查键盘操作、焦点、表单、加载反馈、长内容、响应式与状态导航 |

## 每项的依据与边界

### 1. Impeccable：最适合已有产品界面的持续改进

当前技能明确涵盖 dashboards、product UI、app shells、forms、settings、onboarding、empty states，并区分 `Operate` 模式：针对编辑器、后台、设置等操作界面，优先考虑可扫描性、一致性、原生预期和真实使用场景。[来源：技能范围与 Modes](https://github.com/pbakaus/impeccable/blob/main/.agents/skills/impeccable/SKILL.md)

它提供 `shape`（先规划 UX/UI）、`critique`（UX 审查）、`audit`（无障碍、性能和响应式检查）、`polish`（交付前打磨）、`harden`（错误、国际化和边界情况）、`clarify`（标签与错误文案）、`extract`（提取设计令牌与组件）等入口。这些能力与工作台类产品的改版匹配。[来源：Commands 表](https://github.com/pbakaus/impeccable/blob/main/.agents/skills/impeccable/SKILL.md)

### 2. UI UX Pro Max：适合建立一套一致的产品设计规范

技能包含设计风格、产品配色、字体组合、UX 规则、图表与技术栈指导；优先级涵盖无障碍、触控交互、响应式、表单反馈、导航和数据展示。新页面或产品方向使用 `--design-system`，局部问题使用 `--domain`，已知实现技术使用 `--stack`。[来源：规则优先级、Query Contract 与 Workflow](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill/blob/main/.claude/skills/ui-ux-pro-max/SKILL.md)

它要求先检查项目依赖来识别 React、Vue、Svelte、Nuxt 等实际技术栈，明确禁止默认猜测某个框架；这比直接套 React 专用技能更适合现有 Vue 产品。[来源：Analyze User Requirements](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill/blob/main/.claude/skills/ui-ux-pro-max/SKILL.md)

### 3. Anthropic frontend-design：适合视觉方向与页面实现

当前技能强调从产品主题、受众和主要任务出发，先建立颜色、字体、布局与原则，再回看是否过于模板化，然后实现并自查。它同时要求移动端适配、可见的键盘焦点、减少动态效果偏好，以及清楚的操作名称、错误与空状态文案。[来源：Process、Restraint 与 More on writing in design](https://github.com/anthropics/skills/blob/main/skills/frontend-design/SKILL.md)

适合解决“页面看起来没有主次、套模板、文字与视觉脱节”的问题。其重点是视觉判断与表达，不应将其视为完整的产品流程发现方法或组件库。

### 4. Vercel web-design-guidelines：适合作为完成实现后的检查关卡

技能本身负责拉取最新规则、读取目标代码、逐项检查，再以 `file:line` 输出结果；其用途明确包括 UI 审查、无障碍检查和 UX 审查。[来源：SKILL.md](https://github.com/vercel-labs/agent-skills/blob/main/skills/web-design-guidelines/SKILL.md)

实际规则覆盖语义化 HTML、图标按钮标签、焦点可见性、表单错误、异步反馈、长文本、空数据、图像尺寸、动态效果、URL 状态、触摸与拖拽替代操作、暗色主题及国际化。[来源：规则文件](https://github.com/vercel-labs/web-interface-guidelines/blob/main/command.md)

边界：这是审查技能，不能代替产品视觉方案。规则中含 React/Next 风格示例，用于 Vue 时应按等效的原生 HTML 与 Vue 实现检查，不需要更换框架。

## Taste Skill 的定位

[Leonxlnx/taste-skill](https://github.com/Leonxlnx/taste-skill) 值得关注，但默认 `design-taste-frontend` 主要针对 landing pages、marketing sites、portfolios 和公开产品介绍页。其 README 的 FAQ 明确说默认技能不用于 dashboards 和 app screens，并建议产品 UI 改用 `minimalist-skill` 或 `soft-skill`。[来源：技能列表与 FAQ](https://github.com/Leonxlnx/taste-skill/blob/main/README.md)

如果后续制作教育产品官网或宣传落地页，默认 Taste Skill 更对口；现有产品工作台优先选择上表中的技能。

## 范围说明

- 本次只新增调研笔记，没有安装第三方技能或修改产品代码。
- 推荐按照产品 UI 的具体任务匹配，不以 GitHub star 数排序。
- 所有链接指向仓库当前主分支，后续版本可能更改路径、运行依赖与行为；采用时应重新确认目标版本。
