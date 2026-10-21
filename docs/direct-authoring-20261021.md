# 动画与课件直接生成 · 2026-10-21

状态：本地候选版已完成四科真实模型重测，隔离 PostgreSQL 回归已完成；未部署至8080/8000，未提交或推送。依据用户最新要求“都改成直接生成的方式”，本文覆盖历史受控组件及固定页面模板的生成约束。历史作品保留兼容，不自动迁移。

## 实际生成链路

| 环节 | 课件 | 动画 |
| --- | --- | --- |
| 作者输出 | 每页Slide.html完整HTML/CSS/SVG/少量同步JS | html.direct@1，每幕AnimScene.html与同步renderFrame |
| 模型职责 | 构图、图形、公式、可见文字和教学表达 | 构图、图形、运动路径与逐句演示 |
| 宿主职责 | 资料、稳定页ID、审核、额度、作品、导出 | 资料、稳定幕ID、真实TTS、字幕、停顿、播放、审核、额度、作品、导出 |
| 实际预览 | 浏览器测量并导出PPTX，以真实PPT截图审核 | 隔离iframe运行同一源码；方案无语音预览，代表幕/成品使用实际音轨 |
| 编辑 | data-editable文字本地编辑；对话/单页AI改写修改html | 指定幕改写html/旁白/停顿，其他幕由宿主保留 |
| 交付 | 真实PPTX，原生可编辑文字＋图形背景 | 离线单文件HTML；现有可选MP4导出 |

JSON继续用于任务结构与元数据，不再让后端按bullets/objects补画教学画面。新模型响应缺少html会明确失败并进入已有有界修复；新课件混入旧图示/公式组件也会拒绝，避免实际画面静默丢失。layout仅记录教学用途，不等于页面版式。显式sample模式仍使用固定测试内容并标明非真实模型结果。旧动画原型仅供已有作品读取及兼容修改。

## 代码落点与规则

- `backend-go/direct_author.go`：两类直接创作技术契约、源码边界与新课件响应校验。
- `backend-go/skills/direct-authoring.md`：Manim的机制表达、frontend-slides的视口容量、HyperFrames的确定性时间轴，实际嵌入生成请求。课件/动画技能版本分别为edu-courseware-direct-v1、edu-animation-direct-v1；全课规划仍使用教学构图共用原则。
- `courseware_batches.go`、`courseware_model.go`、`providers.go`、`courseware_conversation.go`、`revisions.go`：逐页/整套生成、修复、对话及交付后改写均要求源码。单页改写预算升至12000 tokens。
- `animation-video/authored-page.mjs`、`render-service-py/authored_page.py`：真实Chromium隔离运行、文字测量与PPTX导出。检测可见文字标记、重复/嵌套ID、越界、溢出、文字互相重叠；重叠返回安全错误类别authored_text_overlap供模型修复，不转发原始浏览器栈或私有正文。
- `animplayer/direct.js`、`direct-boot.js`、`direct_runtime.go`、`verify-authored.mjs`：按实际cue和当前时间同步源码；在交付前实际检查运行、代表幕可见变化、倒退DOM及像素一致性。像素允许每通道不超过2级的Canvas抗锯齿差异。
- `science_visuals.go`、`AuthoredScenePreview.vue`：已保存方案的鉴权分镜源码预览；无TTS且时长估算，不能当代表幕确认。
- `AuthoredPageEditor.vue`、`CoursewareStudio.vue`、`CoursewareEvidence.vue`：直接页编辑文字并保存实际PPT；不再显示无效的旧组件编辑入口。改图形与布局通过当前页对话。混合文字样式经本地纯文字修改后会合并为普通文字，界面明确告知。
- `ClassroomPresentation.vue`：直接页课堂演示显示实际PPT截图；一次翻页对应一页，不再按摘要重排旧模板。截图未就绪时明确提示，不能悄悄换成纯文字画面。教师答案留在提示窗口，当前直接页不宣称支持按答案逐步揭示。
- `tasks.go`：直接页检查排版服务authored-html-v2能力，旧服务明确报错，不静默丢弃源码。草稿保存、成品编辑和确认的完整课件JSON限32MiB，其余请求仍2MiB；每页源码仍有独立200000字节限制。
- `compose.yaml`、渲染Dockerfile：打包Chromium/Node/Puppeteer与桥接脚本，`.dockerignore`排除.env、存储、工作产物等。Docker镜像尚未构建验证。

新源码仅在受限浏览器iframe执行；CSP和网络拦截限制外部访问，脚本和浏览器均有等待上限。后端/Python不执行模型产生的系统代码。源码hash、音轨和依赖继续参与确认绑定；现有身份、成员隔离、额度与结算代码保持原流程。本地样片不创建正式收费任务。

## 实际产物与自评

### 地理动画：海陆温差与白天海风环流

路径：`work/animation-direct-20261021/sea-breeze.html`、`sea-breeze.mp4`及旁侧generation/script/export记录。真实deepseek-flash直接创作全部画面，5句真实TTS音轨，4幕，自动播放66.794秒。最终MP4为1280×720、24fps、1604帧、H264/AAC。最终导出sourceHash与当前HTML一致。

已实际查看各幕截图、关键句中/末帧及倒退状态。期间退回模型修正近地面海→陆、上空陆→海、海上下沉、q4前不得提前显示完整环流和四段端点断开；最终画面环路闭合，粒子真实运动，问题幕未提前展示夜间答案方向。画面由模型生成，没有人工硬写图形冒充真实结果。累计调用记录11次，包含前期配置错误及多轮修复，不能把最终一次成功当总调用成本。

自评：从纯文字/卡片填槽改为实际机制演示，方向和回跳检查通过；仍是简化示意，不是数值气象仿真或教师课堂验收。个别标注仍靠近路径，视觉精致程度还有提升空间。该CLI样片使用整句音频文件时长作cue，不宣称满足200ms语音边界偏差；正式HTTP链路使用已测语音边界。

### 生物课件：植物细胞结构

最终路径：`work/animation-direct-20261021/courseware-final-delivery/plants.pptx`；`plants/slide-1.png`至`slide-3.png`为实际PowerPoint导出截图。3页完整细胞结构、叶绿体过程/液泡吸失水对比、具体读图任务；答案留在教师备注。真实deepseek-flash编写HTML，图形未由人工重画。

实际查看三页后修复了光照被写为反应物、光照标签撞标题、氧气标签压图、四个三角箭头方向不清等问题。最后单页模型修复使用SVG marker-end；第1、3页源码与修复前逐字一致。最后阶段模型调用1次，之前多轮生成及修复记录分目录保留，不表示整套仅调用1次。真实PowerPoint COM检查3页文字均无溢出。

自评：主体图形和教学任务已存在，页面仍有传统讲授式图文对照；直接生成允许变化，并不自动保证优秀美术设计。细胞/叶绿体均为非比例示意，尚未取得教师学科验收。PPT文字可编辑，复杂图形为图片背景；本轮未实现图形对象级全可编辑或任意HTML交互转换为PPT原生动画。

## 四科真实重测与产物（本地，2026-10-21）

以下课件画面及动画源码均来自实际 deepseek-flash 调用，动画带真实 TTS；不是 sample 模式或人工固定图形冒充模型结果。课件与动画分别生成，不能由课件成功推断动画成功。

| 科目 | 教学主题 | 真实 PPTX 与实际 PowerPoint 截图 | 离线动画 HTML | 自动播放时长 |
| --- | --- | --- | --- | --- |
| 数学 | 勾股定理：面积为什么相等 | `work/direct-closure-20261021/reviewed/math/courseware/lesson.pptx`，同目录 `lesson/slide-1.png` 至 `slide-3.png` | `work/direct-closure-20261021/reviewed/math/animation/lesson.html` | 56.500 秒，3 幕 |
| 物理 | 凸透镜成像：物距改变时像怎样变化 | `work/direct-closure-20261021/reviewed/physics/courseware/lesson.pptx`，同目录三张实际截图 | `work/direct-closure-20261021/reviewed/physics/animation/lesson.html` | 51.876 秒，3 幕 |
| 历史 | 丝绸之路：交流如何发生 | `work/direct-closure-20261021/reviewed/history/courseware/lesson.pptx`，同目录三张实际截图 | `work/direct-regression-retry-20261021/history/animation-retry/lesson.html` | 63.816 秒，4 幕 |
| 英语 | 位置表达：in、on、under | `work/direct-closure-20261021/reviewed/english/courseware/lesson.pptx`，同目录三张实际截图 | `work/direct-regression-retry-20261021/english/animation/lesson.html` | 81.272 秒，4 幕 |

四科课件合计 12 页，已实际由 PowerPoint 导出截图，各科 `layout-qa.json` 均为 `overflows: []`。这只证明文字边界检查通过，不能替代教学、构图和美术验收。四份动画实际浏览器检查均通过运动、倒退定位、隔离沙箱和响应式尺寸检查，记录在 `work/direct-closure-20261021/all-subjects-browser-final.log`。

本轮修复没有止于“能导出”：数学退回模型修正正方形面积比例、画布越界、重复标题和直角标记；物理退回模型修正像距/像高、光线交点、所有中间状态以及读数精度。最终通过浏览器读取实际 SVG 坐标，检查数学三个正方形面积按同一比例为 9、16、25；物理抽查 21 个中间状态，验证薄透镜公式、像高、过光心光线及折射后经过焦点的光线。实际几何证据见 `work/direct-closure-20261021/geometry-final.json`。动态读数统一使用约号及一位小数，计算使用未舍入参数；此规则已同步至实际嵌入模型请求的 `backend-go/skills/direct-authoring.md`。

物理 MP4 为 `work/direct-closure-20261021/reviewed/physics/animation/lesson.mp4`：1280×720、24fps、1246 帧、H264/AAC、6 条原始语音合成入轨。旁侧 `lesson.mp4.json` 的 sourceHash 已与最终 HTML 字节哈希核对一致。数学、历史、英语 MP4 也已按最终 HTML 导出，均位于各科 `work/direct-closure-20261021/reviewed/<subject>/animation/lesson.mp4`：分别为 1356、1532、1951 帧；四科全部原始语音、H264/AAC、1280×720/24fps、首中末和回跳检查通过，四份导出 sourceHash 均与对应 HTML 一致。汇总见 `work/direct-closure-20261021/delivery-manifest.json`。

模型辅助截图审查的原始结果保存在 `work/direct-closure-20261021/*-final-visual-review.json`。其中存在误报，未盲目照单修改：例如把练习的空间图示说成“泄露答案”、对已经声明方向的路线示意套用地图朝向、对正确面积比例做错误判断。涉及几何和物理机制的结论以当前源码、实际浏览器坐标、公式及更新后的截图交叉核对；旧截图的意见不作为新修订验收证据。本轮没有人工教师验收，不宣称优秀设计或所有学科质量均已保证。数学当前是具体数值的面积验证，不是一般性证明；历史为交流路线示意，不是准确地理地图。

动画模型调用累计成本没有按最后一次成功抹平：数学当前记录 8 次、物理 5 次；对应 `lesson.html.generation.json` 保留累计 tokens 和实际模型。课件后续局部修复的 generation 文件只代表该次调用，早期输入、provider、prior-deck 与日志分目录保留，不把它当整套累计成本。

## 验证与未覆盖项

- 最新源码无数据库环境 Go 测试：134 个顶层测试通过、170 个顶层测试按环境跳过、无失败；见 `work/direct-closure-20261021/go-final-source.log`。数据库测试的跳过不当通过，另做下面的真实数据库回归。旧 `work/direct-go-complete.jsonl` 仅为上一阶段基线。
- Python 全量：267 passed、3 skipped、2 warnings，88.33 秒；见 `work/direct-closure-20261021/python-final.log`。覆盖真实浏览器页面/文字/配图/PPTX、动画运行、网络拒绝、死循环等待上限、倒退一致性与安全诊断。
- Vue 生产构建通过，见 `work/direct-closure-20261021/frontend-final.log`。
- Node 原生测试：`node --test animation-video/verify-authored.test.mjs` 通过，覆盖安全诊断和 CSSOM 倒退一致性；见 `work/direct-closure-20261021/node-final.log`。
- `node scripts/check-direct-classroom.cjs` 的既有真实浏览器检查通过实际 PPT 画面、一键切页、缺图显式提示、不退回摘要模板；本轮四科另按实际 PowerPoint 输出验证，没有以 HTML 截图冒充 PPT 截图。
- 已配置临时隔离 PostgreSQL，每轮创建独立测试数据库并在 finally 中删除；完整 HTTP 回归首轮达到 35 分钟上限（182 通过、18 跳过），随后仅续跑未完成项（90 通过、14 跳过）；合计 304 项中 272 通过、32 按条件跳过、无测试失败。两轮清理日志均确认独立测试库删除。证据为 `work/direct-closure-20261021/database-final-current.log`、`database-final-continuation.log` 及旁侧 drop 日志。跳过项需要独立云服务、指定样包/科学渲染器，或额外显式启用的付费/在线探针，不当作通过。外部模型在 HTTP 回归中为显式合成夹具，与上面的真实模型样片区分。
- 历史修订恢复已迁移并单独通过：`database-history-archive-final.log`。覆盖恢复不重复扣费、旧文件保持字节不变、跨成员/企业隔离、陈旧版本冲突和删除后不可访问。全量回归启动后此旧夹具又完成一次修正，应以最新定向结果补充全量快照中的该项，不能隐去两个编译时点。
- 旧测试迁移保留生成、确认、额度、隔离、取消和真实产物断言；直接生成用 HTML 响应，已有组件作品继续按存储草稿兼容路径验收。`courseware_figures.go` 同时修复旧表格与时间轴组合的容量错误：有表格时每页最多 4 节点，无表格时间轴仍可 6 节点。这是兼容旧作品的修复，没有恢复新生成的固定模板流程。
- 本轮是测试夹具迁移；没有迁移正式数据库中的历史作品，也未改变“历史作品不自动迁移”的约定。
- 直接页排版修复测试补强：先以精确表头/单元格断言复现了旧夹具转换丢失表格数据，再改为真实 HTML 表格，最终逐格检查导出的 `slide3.xml` 中存在可编辑文本与公式。三类溢出恢复均通过，见 `database-table-evidence-exact-red.log`（失败证据）和 `database-table-evidence-final.log`（修复后）；不能以清空后的旧 Table 字段相等代替教学数据保留。
- 当前本地运行和隔离 PostgreSQL 回归不依赖 Docker，继续使用现有 Go、Python、浏览器和 WSL 测试数据库。Docker 只是额外的镜像构建探测，已结束，未启动镜像构建；不是当前回归或沿用现有方式发布的阻塞项。正式服务尚未部署，发布仍须按仓库既有流程从完整 commit/tag 打包并校验，同时更新 Go、Vue 和渲染服务，避免新后端连接旧渲染能力。
- MP4 日志含本机杀毒软件注入的外网请求及样式 CSP 提示，外网请求被阻止，实际导出 pageerror 为空；不描述为零控制台告警。

隔离数据库回归已收尾，教师审片仍待进行；本地文件已可查看，不代表线上服务已切换。
