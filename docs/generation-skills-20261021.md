# 动画与课件技能接入核对 · 2026-10-21

状态：本地源码候选；未发布、未提交、未推送。桌面安装与后端编译嵌入分别记录；不把安装成功等同真实模型画面验收。

更新说明：下文保留早先接入审计的版本及测试证据。当前动画为 `edu-animation-v2`、课件为 `edu-courseware-v5`；本轮 manim-video/frontend-slides 蒸馏、连续场景渲染与真实模型样片验收见 [后续报告](animation-rewrite-20261021.md)。

## 动画改动

新增 `backend-go/skills/edu-animation`，版本 `edu-animation-v1`。`animskill.go` 通过 go:embed 载入教学构图指导，`animAuthorInstructions` 同时供 `animmodel.go` 的 CLI 和 `animworkflow.go` 的工作台生成、讨论与局部修改使用；有限 JSON 修复沿用同一 system 请求。技术契约独立保留。CLI 生成记录新增 skill_version，健康接口新增 animation_skill_version。部署后方能在运行服务中生效。

- 构图以教学对象、观察任务和语义动作组织，允许主对象、真实对照、局部放大、路径展开和原文批注；不要求每片同一幕序或固定3–5幕，实际幕数仍受1–12技术上限及30–180秒时长合同约束。
- 去掉“多个文字组件一律拒绝”的计数硬门禁，明确组件数量不代表审美。保留逐条 item_sentences、同幕句子引用、真实图形/语义动作与原始需求检查；允许有用的对照和短提示，不承诺文字卡自动变图。
- 位置介词、透镜、细胞、比喻、双向路线提示只在相应需求出现时附加；去掉固定月亮/小船坐标，图形必须符合实际例句，不能将案例变成全科模板。支持能力的科学 JSON 契约仍独立提供。
- 沿用受控作者数据与现有播放器；没有改成任意模型 HTML、GSAP 脚本或外链资产。播放器槽位、专业图单对象和课件7种layout等实际渲染能力仍存在，单靠提示词不能消除这些边界。

技能源版本与采用/舍弃项见 [来源记录](../backend-go/skills/edu-animation/SOURCES.md)。上游营销装饰数量、固定双焦点、持续背景运动和强制炫技转场未移植到教学规则。

## 课件审计结果

旧课件技能并非完全未接入。项目已有 `edu-courseware-v3`，基于 WorkBuddy PPTX v20260918 适配；它与桌面通用 Presentations 技能并不是同一份文件。

| 环节 | 审计发现与本次结果 |
| --- | --- |
| 初次/分批/单页生成 | agents.go → coursewarePrompt → coursewareDraft 已带教学规范；保持接入 |
| 单页检查点修复 | coursewarePageInstructions 的 page-repair 与生成基础 teaching-design 已接入；既有实际请求测试继续保留 |
| 需求对话 | courseware_conversation.go 已附教学规范；本次自动取得更新的共用原则 |
| 教师单页改写 | revisions.go 已附教学规范；本次自动取得更新的共用原则 |
| 整课规划 | courseware_plan.go 原来独立拼接提示，漏了共用技能；本次补接 composition.md，不灌入“仅生成当前页”的修复指令 |
| Matplotlib | courseware_plot.go、媒体契约与Python渲染已有受控图表接口；不是后端执行桌面技能或模型代码 |

版本升至 `edu-courseware-v4`。共用 composition.md 同时进入整课规划及原 teaching skill 的所有调用点。移除蓝白风格必配左右分栏/底部结论条的描述、无规划路径的固定目标→导入→概念→实验页序；theme 继续约束颜色与字体，具体表达服从内容及真实渲染能力。旧无独立规划预览仍制作前三页，新规划路径仍选三页真正代表内容质量的预览，未改任务协议和教师确认。

## 桌面安装

已装到 `C:/Users/YIZE/.codex/skills/`，下一轮可由 Codex 发现：hyperframes-creative、hyperframes-animation、faceless-explainer、playwright-interactive。每项保留 SOURCE.json。creative 与 Playwright 的下载内容按固定 GitHub tree blob SHA 核验；另外两项在新版下载遇到限流后采用既有本地 checkout 的固定版本，未冒称最新版。Playwright 技能用于开发验收，不注入 DeepSeek 内容提示。

## 验证与限制

- 先观察实际模型请求缺少独立技能、携带无关位置/月亮案例的失败，再实现：TestAnimGenerateCLI 的初次与修复请求检查及实际 HTML 产物通过；生成记录检查技能版本。
- 先更新“有效对照+短提示应可接受”用例，观察旧多文字计数门禁拒绝，再移除该门禁；逐句绑定与历史兼容继续通过。HTTP 文本构图用例同步为允许合法组合，真实图形代表幕另行检查。
- 全 Go 回归107个顶层通过、181个按外部配置跳过，无失败；go vet通过。日志：work/animation-skill-20261021/go-tests.json。任务HTTP测试需隔离 PostgreSQL，本环境未配置，新增的动画/课件规划实际请求断言也未执行到供应商桩。不能以跳过当通过。
- Python真实PPTX渲染边界 test_teaching_media.py：44项通过，含图片/表格/专业媒体组合。
- 真实Edge离线五类组件检查：5项组件、越界/重叠问题0；Node逐句显示和倒退恢复通过。产物与检查：work/animation-skill-20261021/text.html、text-check.json。均为标识明确的固定样例/测试音，不是真实AI重新生成。
- 两个项目适配技能通过 skill-creator 的 quick_validate；不把文档格式校验当作生成质量验证。
- 本轮没有新的真实模型/TTS调用，没有发布重启；生成美术和教学效果仍需以新模型样片与教师审核验证。
