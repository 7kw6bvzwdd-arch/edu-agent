---
name: 教育智能体 · 紫水晶
description: 沿用现有紫水晶主题的教学课件工作区
colors:
  am-50: "#f7f5fc"
  am-100: "#efeafb"
  am-150: "#e6defa"
  am-200: "#d9ccf6"
  am-300: "#bfa8f0"
  am-400: "#a283f3"
  am-500: "#8b5cf6"
  am-600: "#7440e6"
  am-700: "#6d28d9"
  am-800: "#4c1d95"
  am-900: "#2a1458"
  am-950: "#1d0f3d"
  ink: "#272238"
  ink-2: "#4b4560"
  ink-3: "#625b76"
  line: "#e4dff0"
  line-strong: "#d2cbe3"
  surface: "#fff"
  surface-2: "#faf8fe"
  surface-3: "#f3effb"
  ground: "#f7f5fc"
  ok: "#1d7a52"
  ok-bg: "#e7f6ee"
  ok-line: "#b9e2cc"
  warn: "#8a5a00"
  warn-bg: "#fdf3dc"
  warn-line: "#f0d9a2"
  err: "#b42318"
  err-bg: "#fdecea"
  err-line: "#f5c6c0"
  info: "#1f5fa8"
  info-bg: "#e8f1fc"
typography:
  headline:
    fontFamily: '"PingFang SC","Microsoft YaHei UI","Microsoft YaHei","Noto Sans SC","Source Han Sans SC",system-ui,sans-serif'
    fontSize: "30px"
    fontWeight: 650
    lineHeight: 1.5
    letterSpacing: "-0.7px"
  title:
    fontFamily: '"PingFang SC","Microsoft YaHei UI","Microsoft YaHei","Noto Sans SC","Source Han Sans SC",system-ui,sans-serif'
    fontSize: "17px"
    fontWeight: 600
    lineHeight: 1.5
  body:
    fontFamily: '"PingFang SC","Microsoft YaHei UI","Microsoft YaHei","Noto Sans SC","Source Han Sans SC",system-ui,sans-serif'
    fontSize: "14px"
    lineHeight: 1.6
  label:
    fontFamily: '"PingFang SC","Microsoft YaHei UI","Microsoft YaHei","Noto Sans SC","Source Han Sans SC",system-ui,sans-serif'
    fontSize: "13px"
    fontWeight: 500
rounded:
  xs: "6px"
  sm: "8px"
  md: "10px"
  lg: "14px"
  xl: "18px"
  dialogue: "16px"
  studio: "12px"
spacing:
  control-x: "14px"
  control-y: "8px"
  field-x: "12px"
  field-y: "9px"
  page-x: "32px"
  home-x: "36px"
  home-gap: "28px"
components:
  button-primary:
    backgroundColor: "{colors.am-700}"
    textColor: "{colors.surface}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "8px 14px"
  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-2}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "8px 14px"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.sm}"
    padding: "9px 12px"
  nav-item:
    rounded: "{rounded.md}"
    padding: "10px 12px"
  status-awaiting:
    backgroundColor: "{colors.warn-bg}"
    textColor: "{colors.warn}"
    rounded: "999px"
    padding: "3px 9px"
  panel:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.lg}"
---

# Design System: 教育智能体 · 紫水晶

## Overview

**Creative North Star: "紫水晶"**

用户确认保留的既有身份：深紫导航、浅紫底色、白色工作面和紫色操作强调。中文系统字体与紧凑原生控件服务日常备课，正文、资料和真实课件占据主要视觉空间。

提取依据为 `frontend-vue/src/amethyst.css`；`home.css`、`review.css` 在其后加载，补充当前页面布局与密度。本文记录现有系统，不新增配色或明暗主题。

**Key Characteristics:**

- 深紫侧栏与浅色内容工作区。
- 白色面板、浅紫分区、细线和轻阴影。
- 紧凑控件、清晰焦点、文字标注状态。
- 工作区主题与课件模板配色独立。

## Colors

### Primary

紫水晶色阶沿用 `am-*`：`am-700` 用于主要操作、链接和当前项，`am-800` 用于深色强调文字；较浅色阶用于选区、提示底色和悬停。深紫色阶构成侧栏，侧栏保留原有渐变与半透明选中层。

### Neutral

`ink` 为主文字，`ink-2` 为次级文字，`ink-3` 为说明；`ground` 为页面底色，`surface` 为白色工作面，`surface-2`、`surface-3` 为辅助层。`line` 分区，`line-strong` 描绘控件边框。

状态语义沿用 `ok`、`warn`、`err`、`info` 及对应背景、边框。成功任务使用绿色，待预览/待确认/待验收使用琥珀色，失败使用红色。真实模型模式使用成功色，样例模式使用警示色，始终附文字。页内“已确认”可以使用紫色选择态，不等同于任务成功状态。

**The Theme Boundary Rule.** 紫水晶只控制产品外壳；真实PPT预览、课件模板和课堂演示保留各自配色。

## Typography

所有工作区文字使用 frontmatter 中的中文系统字体栈，不依赖远程字体。首页标题采用 `headline`，对话与审核文档标题采用 `title`；普通页面标题为（26px / 700）。首页标题在窄屏降至（26px），手机降至（23px）。

正文基准为 `body`；说明与控件主要使用（12–13px）。审核页辅助信息为（10–12px），让中央PPT获得更多空间。首页对话正文采用（13px / 1.9）。额度、表格和历史记录等数字使用等宽数字。装饰性英文眉题在当前主题中隐藏。

## Layout

外壳默认侧栏（256px），可收至（76px）；顶部位置栏（60px）。通用内容最大宽（1640px），横向内边距使用 `page-x`。宽屏侧栏（268px），不超过（1050px）时为（220px）；不超过（850px）时改为单列外壳与（52px）顶部栏。

当前首页内容最大宽（1260px），主对话居中且最大宽（960px），保持舒适的阅读宽度。欢迎标题、单一白色输入面与下方辅助内容形成层次；输入框与外层合为一个面，聚焦时以紫色边框强调。教学资料与需求摘要置于输入面下方，默认折叠，展开后可操作原有设置。开始对话后显示真实流式记录和可编辑需求表格，标题收紧，备课快捷入口收起。

首页背景采用低对比的浅紫网格、少量线框水晶与细点，图案向下淡出；白色输入面保持实色。纹样由本地SVG和CSS绘制，不使用外部图片。首页使用（44px）顶部导航，保留紧凑的审核状态提示。“提出你的需求”输入标签保持不变。下方两个入口分别聚焦需求输入、展开教材与设置，不直接创建任务；最近作品继续取自当前成员的真实任务。手机上快捷入口与作品列表改为单列。紫水晶配色、侧栏动画和审核流程保持一致。

审核页采用缩略图 / 单页PPT / 检查面板三列，默认（126px / minmax(0,1fr) / 266px）。中央真实PPT保持（16:9）且完整显示；不超过（1050px）时辅助区置于预览后，不超过（760px）时缩略图横排、预览先于辅助区。页面确认与全套交付分别放置。审核页顶部使用紧凑导航、提示与文档标题；中央取消独立滚动，真实PPT按可用视口高度完整缩放，编辑展开后随整页滚动，交付栏不覆盖画面。教案、资料来源与长校验说明收进右侧详情；左侧缩略图保持自然高度并独立滚动。

其他工作区延续首页的浅紫网格与水晶纹样，通过 `workspace-patterns.css` 为工具、作品、资料、账户、额度、统计和成员页面分别调整位置、比例与网格疏密。审核页仅在课件周围的工作区显示淡纹样，表单与真实PPT保留实色背景；窄屏缩小纹样比例。

## Elevation & Depth

层次来自浅色面、细边框与轻阴影。`shadow-1` 用于通用控件和面板，`shadow-2` 用于悬停作品卡，`shadow-3` 用于弹窗。当前首页对话采用更轻的专用阴影；审核工作区容器无阴影，真实PPT有轻投影。精确阴影、动效与断点记录在 sidecar。

保留（150ms / 220ms）控件过渡；桌面侧栏使用（280ms）可反向的网格宽度过渡，菜单文字随收起淡出，主内容同步调整位置；减少动态效果偏好下，动画与过渡压缩至（0.01ms），内容仍可见。

## Shapes

圆角沿用 `r-*`：小型元素 `xs`，按钮和输入框 `sm`，导航与内层容器 `md`，通用面板 `lg`，弹窗 `xl`。当前首页对话使用 `dialogue`，审核外框使用 `studio`；状态标签为胶囊形。真实幻灯片边角保持轻微圆角（3px），不套用面板圆角。

## Components

- **按钮：** 默认原生按钮最小高度（36px），细边框与轻阴影；主要按钮保留原有紫色渐变，次要按钮为白底。按下位移（1px），禁用移除阴影与位移。触屏控件最小高度（44px）。
- **输入：** 白底、`line-strong` 边框和 `sm` 圆角，焦点切换紫色边框与光环；原生复选框和单选框使用紫色强调。对话输入允许纵向调整。
- **导航：** 深紫底上浅色文字，当前项为半透明渐变、细边框和侧边标记；收起状态保留图标，窄屏移除侧边标记。
- **面板：** 白底、`line` 边框和 `lg` 圆角；首页辅助区以浅紫提示面或平面列表呈现，避免每条信息都成为独立卡片。
- **状态标签：** 文字、圆点与语义色同时出现；样例/真实模式标记保持可见。状态文字不省略为单个颜色点。
- **审核：** 当前缩略图用紫色描边；检查与助手采用下划线选项卡。缩略图切换同步中央预览与当前页审核；大图使用真实PPT输出，未生成时明确显示等待状态。

所有可交互元素保留（2px）紫色键盘焦点轮廓与（2px）偏移；输入还保留自身焦点光环。组件完整状态示例见 `.impeccable/design.json`。

## Do's and Don'ts

### Do:

- **Do** 复用现有紫水晶变量和中文系统字体。
- **Do** 让真实课件内容和当前操作具有明确视觉优先级。
- **Do** 用文字说明状态、模型模式和交付门槛。
- **Do** 保留键盘焦点、触屏尺寸与减少动态效果适配。

### Don't:

- **Don't** 用产品紫色覆盖课件模板或真实PPT预览。
- **Don't** 把固定样例标为真实AI结果。
- **Don't** 以颜色独自传达任务状态。
- **Don't** 为延续现有页面另建一套配色、字体或控件体系。
