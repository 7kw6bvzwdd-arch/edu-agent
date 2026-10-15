---
version: 1
slug: "frontend-vue-src-homeworkspace-vue"
primary_target: "frontend-vue/src/HomeWorkspace.vue"
related_targets: ["frontend-vue/src/home.css","frontend-vue/src/review.css","frontend-vue/src/CoursewareStudio.vue"]
---

# 首页需求对话改版

范围：HomeWorkspace.vue、首页外壳样式；Operate 模式。
用户于2026-10-15确认优先突出新课需求对话，保留现有生成流程。

## Direction contract

THESIS: 打开首页即可沟通教学需求；输入与确认优先，作品和工具降为辅助入口。

OWN-WORLD: 用户指定既有紫水晶主题。直接继承 amethyst.css 的深紫侧栏、浅紫底色、白色工作区、紫色强调与中文系统字体，不另建配色。圆角8–16px。

STORY: 直接输入教学想法，与助手流式沟通；可编辑最新需求表格或按需展开设置，自行选择继续对话或制作三页预览，再进入教师审核。

FIRST VIEWPORT: 首页保留深紫侧栏、菜单平滑动画和紧凑顶部。参考用户提供页面的居中大输入框：主对话最大960px，输入只用一个白色面，资料与摘要折叠在下方；真实备课工具与最近作品位于对话后。开始对话后，工具入口收起，流式记录与最新可编辑需求表格占据对话区域。手机保持单列，输入与操作可达。审核页保持缩略图、完整PPT和右侧审核/助手的布局，中央不独立滚动。

FORM: 用户明确指定紫水晶主题与常见PPT编辑布局。首页保留对话主区、设置与摘要按需展开，制作由明确的用户操作触发；审核页的缩略图切换同步大幅预览和当前页审核，单页修改不混淆全套交付。seed: none (unavailable)

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
