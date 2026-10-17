# HyperFrames 源码评估（2026-10-16）

结论：**可以用于教学动画，优先作为可选 MP4 导出能力的候选；暂不替换本项目受控作者数据、教学播放器和审核账务流程。** 最有价值的是逐帧定位与视频编码。它不是现成的中文数学教学设计器。

## 核查范围与版本

- 官方仓库：https://github.com/heygen-com/hyperframes；本地目录：`D:/tools/harness/edu-agent/work/third-party/hyperframes`。
- 固定提交：`8be76fc86cda5b2a77d0ee23b65f582e85376042`，提交时间 `2026-10-16T02:37:14Z`；本地 rev-parse/log 与 GitHub commits/main API 一致。下文所有上游文件/行号针对该提交。
- CLI、engine、producer 的 package.json 都是 **0.8.163**，不把源码版本等同于 npm 当时的 latest。[engine manifest](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/engine/package.json#L1-L5)
- 源码评估后，主任务在独立忽略目录安装固定版本并完成真实 render 和视频解码探针；结果见下节。未将上游依赖加入生产项目，未公开其服务。

## 本机真实验证

2026-10-16在Windows/Node 24.21.0/Microsoft Edge 155上，用发布包`@hyperframes/engine@0.8.163`成功导出已有的真实DeepSeek+Seed Audio几何作品。原HTML不修改；测试副本只补`__hf`秒协议、导出画幅与AI标识，重算CSP脚本/样式hash。逐句音轨从包内提取后按原教学时间轴混合，没有重新调用模型或TTS。

| 实测项 | 结果 |
| --- | --- |
| 内容 | 4幕、12句原配音、原字幕、固定停顿 |
| 输出 | 1280×720、24fps、1971帧、H.264+AAC、2,714,281字节 |
| 时长 | 原时间轴82.108秒；容器时长允许帧取整偏差，小于0.1秒 |
| 耗时 | 从捕获会话启动至完成混音/封装201.804秒（约3分22秒），不含原模型/TTS制作；单次探针不能当吞吐基准 |
| 定位 | 首/中/末与随机倒退，重复时间点的SVG/字幕状态一致 |
| 浏览器播放 | 真实MP4解码、播放推进，0.25/25/60/81.5秒定位成功 |
| 画面 | 4幕画幅、播放控件不入画；查看源捕获与MP4解码截图，中文字和公式可见 |

证据：`work/hyperframes-probe-20261016/probe.mjs`、`report.json`、`playback.mjs`、`playback-report.json`、`geometry-hyperframes.mp4`。源包SHA256(base64)：`cY0eq1z02v9GXFMsrGXk2rPXlf6p/djGERC7NubdSfg=`。

实测限制：Windows走screenshot路径，并非Linux BeginFrame；系统安全软件注入了被原CSP阻止的连接并改写CSP，日志也有上游内联样式尝试被阻止，不能称浏览器控制台全无错误或整段会话全断网。`errors=[]`仅指pageerror异常；`external=[]`仅指初始化之后的请求拦截统计。探针不放宽原产物CSP，正式worker宜用固定、干净浏览器环境并从导航前限制网络。没有人工听觉验收、逐句字幕偏差实测、并发/取消/恢复/计费端到端验收，故不宣称MP4产品入口已完成。

探针使用独立`ffmpeg-static@5.3.0`/`ffprobe-static@3.1.0`工具，不改变系统PATH；正式分发须按所选FFmpeg构建核对许可证，不随根Apache许可一并推断。

## 能力及适用位置

| 能力 | 已核实事实与一手证据 | 对本项目的意义 |
| --- | --- | --- |
| 逐帧渲染 | Engine 要求页面暴露 `window.__hf.duration`（秒）及确定性 `seek(seconds)`；捕获逐帧调用 seek。[types.ts:125–133](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/engine/src/types.ts#L125-L133)、[frameCapture.ts:2833–2858](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/engine/src/services/frameCapture.ts#L2833-L2858) | 可桥接现有 `__anim.seek(ms)` / `render(t)`。 |
| 视频流水线 | Producer 提供 `createRenderJob` / `executeRenderJob`，包含编译、抓帧、编码、混音，可传 AbortSignal。[producer 文档:6–126](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/docs/packages/producer.mdx#L6-L126) | 可选 MP4 后台导出，不需恢复旧 Manim。 |
| 多段音频混合 | 音轨具有 src/start/end/mediaStart；使用 adelay 定位并补齐总时长。[audioMixer.types.ts:8–25](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/engine/src/services/audioMixer.types.ts#L8-L25)、[audioMixer.ts:331–341](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/engine/src/services/audioMixer.ts#L331-L341) | 复用已合成并验收的逐句语音，不重复调 TTS。 |
| 动画参考 | 支持 GSAP/CSS/WAAPI 等 seekable 动画，FrameAdapter 要求随机定位与同帧同状态。[frame-adapters.mdx:11–89](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/docs/concepts/frame-adapters.mdx#L11-L89) | 可参考开场、转场、强调效果，先不引入整个 Studio/Catalog。 |

**FrameAdapter 不等于现成插件插槽。** 上游明确其为 experimental v0，签名可能变化；host 仍需负责抓帧与编码。实际最小验证应使用 Engine 的 `__hf` 页面协议，避免先设计通用适配器层。[frame-adapters.mdx:6–9、89](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/docs/concepts/frame-adapters.mdx#L6-L9)

## 不能把现有 HTML 直接当成已兼容

1. **时间单位与时钟。** 本项目 `backend-go/animplayer/player.js:197、219` 暴露 `__anim.seek`，单位毫秒；需显式秒→毫秒适配，导出固定为自动播放，禁用真实播放/音频时钟/等待点击。固定教学停顿已由 `backend-go/animtimeline.go:41–63` 算入总时间。本机探针已验证这层桥接，正式导出接口尚未实现。
2. **音轨发现。** 本项目 `player.js:162` 以 `new Audio(D.audio[id])` 播放 JSON 中 data URL；上游解析的是 HTML 的 `audio[id]` 与有声音频 video，不能自动找到我们的 JSON 音轨。[audioMixer.ts:538–639](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/engine/src/services/audioMixer.ts#L538-L639)。应从既有构建结果提取本地音频，沿用原全局起止混合。
3. **CSP 与编译。** 本项目 `backend-go/animpackage.go:73–90` 内嵌音频且以哈希只允许固定 JS/CSS。Producer 合并/插入脚本，render 文件服务器注入 runtime，可能受原 CSP 阻止；不能放宽用户下载包来迁就导出。[htmlCompiler.ts:901–958](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/producer/src/services/htmlCompiler.ts#L901-L958)、[fileServer.ts:819–835](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/producer/src/services/fileServer.ts#L819-L835)。可使用仅供后台导出的受控页面，或 Engine 直接捕获。
4. **字幕/画幅。** 本项目 `player.js:7、120–148、188–195` 中 stage 是1280×720 SVG，字幕和控件在外层：抓整个页面可能包含控件，只抓 SVG 又可能漏字幕。导出区域和布局需实际验证。
5. **离线交付。** 上游 player 缺 runtime 时可能从 CDN 注入；默认示例不等于本项目 §7.1 的断网单 HTML。[player README:288–290](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/player/README.md#L288-L290)。既有离线播放器应保留。

## 继续由本项目负责

- **教学正确性和受控输入：** SPEC 的2026-10-15决定、设计文档 §7 规定模型只产作者数据，不能执行模型代码。HyperFrames 接受可执行 HTML/JS，但不证明斜率、面积、中文旁白正确；可让程序把受控 JSON 渲染为兼容页，无需恢复自由 HTML 生成。
- **审核和账务：** 代表幕局部内容 hash、成品验收、每幕一次免费修改、失败/取消释放及整个请求最多一次结算仍归 `animworkflow.go`。渲染 job 不是新的业务生成请求；MP4 应作为同一修订的附属导出。
- **语音：** 上游 CLI voice router 枚举 HeyGen/ElevenLabs/Kokoro，未含当前 Seed Audio 方案；混音可直接用已有音频，无需切供应商。开场与情感语气应在本项目作者提示/TTS 版本配置完成。[audio/providers.ts:7–64](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/cli/src/audio/providers.ts#L7-L64)
- **我的作品/权限：** 上游不具备本项目企业、成员、来源快照与积分账务领域语义，下载/SSE/恢复必须经现有鉴权。

## 安全接法

以下为影响集成的源码事实，不是完整安全审计。

- Producer 的 app factory 直接注册 render/SSE/queue/lint/download 路由，未加身份/成员中间件；下载凭内存随机 token，不检查当前企业/成员。不能原样暴露为本项目成员接口。[server.ts:933–944](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/producer/src/server.ts#L933-L944)、[server.ts:896–914](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/producer/src/server.ts#L896-L914)
- 请求可接收主机 projectDir/outputPath，previewUrl 用普通 fetch；只能由程序指定单任务临时目录和产物路径，不透传成员的任意 URL/文件路径。[server.ts:401–474](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/producer/src/server.ts#L401-L474)、[paths.ts:134–145](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/producer/src/utils/paths.ts#L134-L145)
- Engine Chrome args 包含 `--no-sandbox` / `--disable-setuid-sandbox`，不能视作任意模型脚本的安全执行箱；后台 worker 应无应用密钥、无DB访问、限制网络/资源。[browserManager.ts:1041–1056](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/engine/src/services/browserManager.ts#L1041-L1056)
- 上游有局部防护：render 文件服务器做路径/符号链接检查并绑定127.0.0.1；媒体下载器做HTTPS、私网地址与逐跳重定向检查；这些不等于整个任意HTML执行环境都隔离。[fileServer.ts:57–91、916](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/producer/src/services/fileServer.ts#L57-L91)、[urlDownloader.ts:258–279、511–530](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/engine/src/utils/urlDownloader.ts#L511-L530)
- Player 的 `sandbox-origin` 可选，默认加 `allow-same-origin`；不能仅凭 README 的 sandboxed iframe 字样，放任不可信脚本与应用身份同源。[hyperframes-player.ts:369–375](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/player/src/hyperframes-player.ts#L369-L375)

## Windows、授权与维护成本

- **不是只支持 Linux。** Windows 工作流会 scaffold、render，再用 ffprobe 验证1920×1080/30fps/约8秒；Engine 有 win64 headless shell 与 D3D11分支。这是上游测试定义，不代表本轮亲见CI成功或本机已通过。[windows-render.yml:242–297](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/.github/workflows/windows-render.yml#L242-L297)、[browserManager.ts:189–190、1233](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/engine/src/services/browserManager.ts#L189-L190)
- 运行需Node≥22、FFmpeg；monorepo源码开发构建用Bun，engine/producer带Puppeteer等依赖。采用发布包可避免把整个项目装进产品；固定版本并验证seek/音轨/CSP，不在运行时跟随latest。[README:138](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/README.md#L138)、[producer/package.json:45、77–101](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/producer/package.json#L77-L101)
- 逐像素结果会受Chrome/字体/codec/GPU环境影响，官方Engine文档明确这一限制。应固定中文字体和渲染版本，纳入渲染依赖hash。[engine.mdx:20–26](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/docs/packages/engine.mdx#L20-L26)
- 仓库Apache-2.0，README声明无按次渲染费用；分发/修改时遵循LICENSE §4的许可/归属/修改说明义务，Catalog素材、字体、GSAP等另核其许可，不将根许可推及外部资产。[LICENSE:66–121](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/LICENSE#L66-L121)、[README:286](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/README.md#L286)
- LFS视频基线约240MB，源码评估可跳过，不必安装全部skills/Studio/云设施。[README:338–363](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/README.md#L338-L363)

## 最小接入建议与待验证项

本轮已完成独立可删除探针：已有真实HTML/构建快照 → 受信任`__hf.seek(seconds)`桥接 → 固定1280×720/24fps教学画面与字幕 → 原逐句音轨按原时间轴混合 → MP4。验证采用Engine；真实作品的教师审核不由测试点击代替。下一步若正式采用，先接同修订附属导出，不引入整个Studio；既有浏览器/FFmpeg若已足够，也应比较依赖成本。

验收至少包含：首/中/末及随机倒退定位状态；控件不入画；真实音轨、每句起止与停顿；Windows可打开、中文/公式无缺字；短样本及真实60–90秒作品的耗时/资源。正式接入补HTTP认证/企业成员隔离/取消恢复/修订下载/不重复结算；沿用设计§9的MP4显式与隐式AI标识，不把能编码当作全部验收通过。

音频发现失败不可静默交付静音，参考上游音频失败门禁：[renderOrchestrator.ts:816–825](https://github.com/heygen-com/hyperframes/blob/8be76fc86cda5b2a77d0ee23b65f582e85376042/packages/producer/src/services/renderOrchestrator.ts#L816-L825)。

**建议拍板：保留拉取的上游用于MP4探针；借用逐帧捕获/编码的价值高，整体替换现有动画模块的价值低。**
