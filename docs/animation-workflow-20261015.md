# HTML教学动画首个端到端切片 · 2026-10-15

状态：代码已实现，隔离验收；未覆盖当前正式进程，未修改正式用户账务。用户最新“开始实施”优先于历史“动画入口停用”记录。仅新 HTML 入口开放；旧 Manim/Ark 入口仍停用，历史 MP4 不迁移。

## 已实现

- 新建一次函数参数动画（数学）：需求与分镜方案不预留；修改方案支持自然语言与经过严格校验的作者 JSON。真实模型和 TTS 未配置时明确拒绝创建，不回退固定样例。
- 确认方案预留 40 积分，制作包含真实动作、语音与字幕的代表幕；教师确认代表幕后生成其余幕；完整验收只结算一次。
- 成品为单文件离线 HTML；可自动/分步播放、等待点击、暂停、定位、回放。在线预览为鉴权获取的 Blob，置于仅 allow-scripts 的独立 iframe，不开放同源权限。
- 逐句 TTS，字幕使用供应商说话区间，时间轴使用实际音频时长，静音段不显示字幕。音频阻止播放或长时间卡顿时暂停并提示重试，不静默改为无声结果。
- 每幕一次免费局部 AI 修改，只允许一个待接受修改。无变化的句子复用音频，其余幕保留；接受与计次在同一事务。失败、放弃不计次，也不退原消费；修改未接受时下载仍为上次验收版本。首期不支持拆幕、合并幕或改变已有幕 ID。
- 代表幕 hash 包含该幕作者内容、实际音频摘要、字幕边界、渲染脚本/样式与语音配置；不包含全局 revision 或其他幕的绝对时间。改其他幕保留代表幕确认，改代表幕使确认失效。所有方案修改使全片验收失效，不静默继承。确认事件保留 revision/hash，已验收版本保存在 artifact_revisions。
- 失败、取消、24 小时等待超时释放未结算预留。恢复同一请求保留调用计数及音频检查点，重新检查权限/余额并建立新预留记录，历史预留和释放不抹平；整个请求唯一结算。租约/attempt/revision 防止过期 worker 回写。
- 复用企业与成员鉴权、任务事件、文件存储、删除清理和用量日志。旧课件队列保持优先。带动画内容/音频检查点的中断作品不会被当作空草稿自动丢弃。

## 接口

新建仍使用 POST /api/tasks，明确 type=animation、animation_engine=html，提供 topic、requirements、idempotency_key。

|路径（/api/tasks/{id} 后缀）|方法|用途|
|---|---|---|
|/animation|GET|状态、版本、分镜、hash、免费次数、调用数；不返回服务端路径|
|/animation/revise-plan|POST|revision、request_key，加 instruction 或 script|
|/animation/confirm-plan|POST|revision；不足 40 积分返回 402，方案保留|
|/animation/confirm-representative|POST|revision + representative_hash（字段 hash）|
|/animation/accept|POST|revision + build_hash；先检查实际文件，再事务验收|
|/animation/edit|POST|revision、scene_id、instruction、request_key|
|/animation/discard-edit|POST|放弃待接受修改；保留已交付内容|
|/animation/cancel、/animation/resume|POST|取消/恢复同一任务；恢复携带 revision|
|/animation/file|GET|当前预览|
|/download/html|GET|最新已验收版本，修改中也可下载|
|/revisions/{version}/download/html|GET|已验收历史 HTML 文件|

状态前缀 anim_ 与旧动画/课件队列隔离。方案、代表幕、全片和局部修改分别有 pending/running/待审核状态，最终使用 succeeded/failed/cancelled。schema.sql 增加真正的 unreserved 账务状态、tasks.animation/html_path、artifact_revisions.html_path，不伪装预留或修改历史流水。

## 试运行调用上限

以下为本切片的保守工程默认值，可通过环境配置收紧，尚未视为产品定价或最终 SLO。每类独立持久化，总数属于原请求，恢复不重置。

|计数|默认值|配置|
|---|---:|---|
|方案生成/讨论|8|ANIMATION_LIMIT_DISCUSSION|
|JSON校验修复|4|ANIMATION_LIMIT_JSON_REPAIR|
|局部画面修改生成|10|ANIMATION_LIMIT_SCENE_REPAIR|
|逐句语音请求|120|ANIMATION_LIMIT_TTS|
|截图模型审核|0|本切片未启用|

单次方案/修改最多一次 JSON 修复。HTTP 工作流对 TTS 失败保存检查点后等待显式恢复，不使用 CLI 的重试环。单阶段执行上限 30 分钟，租约 31 分钟；没有承诺 2–5 分钟内必定完成。创建时每成员最多 3 个进行中的 HTML 动画，原请求最多 64 次编辑请求键，避免无上限方案试探。

## 已有验证证据

- Go 单元测试、go vet、前端生产构建通过。新增 HTTP 契约使用隔离 PostgreSQL，接口覆盖认证、同企业不同成员、跨企业隔离、幂等创建、低余额、并发重复验收、预留释放/恢复、代表幕复用、单幕修改、失效 hash、取消中 worker、等待超时、删除失效。最终通过记录：work/conversation-first/animation-workflow-release-check.log（完整流程、成品损坏阻止结算、已结算修改失败保护、原作品管理兼容）、animation-recovery-final.log（低余额、释放恢复、历史流水、调用上限）。测试数据库均已清理。
- 界面使用真实 Vue 构建与显式 fixture API 验证：15 项通过，报告 work/anim-workflow/browser/report.json；不能把 fixture 验证称为真实 AI 生成。
- 真实 DeepSeek + Seed Audio create：1 次方案调用、10 次 TTS、4 幕，自动播放 74.392 秒；生成至验收约 279 秒。真实产物 work/anim-workflow/live/live-linear-k.html；测试账户的确认由测试程序发送，仅证明接口集成，不代替教师审核。
- 播放器增加明确语音失败暂停后，复用相同真实作者数据与音频生成开发验证包 live-linear-k-current.html，没有追加供应商调用或覆盖已验收原包。Edge 断网 18/18 检查通过（字幕静音边界、音频驱动、暂停/定位/回放/等待、排版边界、无外网请求、无 CSP/控制台异常、音频失败提示），报告 work/anim-v2/browser/report.json。

## 尚未完成与发布边界

- 首期只有一次函数 func.params@1；不是整个初中数学/物理产品完成。MP4 导出、其他原型和沙箱自定义片段仍属后续阶段。
- 音色仍使用 create 的普通话女教师提示词，不能宣称跨句锁定同一个人；固定音色、人工试听、数学读法与课堂教学质量待教师验收。
- 字幕已用供应商边界，但未做独立音频强制对齐来证明每句误差 ≤200ms；这是待测工程目标。
- 未内嵌通用字体文件，公式由受控 SVG/文本绘制。断网可播放不等于所有机器字形一致；目前验收为 Windows Edge，Chrome/macOS 与字体一致性尚未验收。
- 未启用自动截图模型审查；当前依赖程序参数校验、浏览器布局检查与教师显式审核，不标为全自动质量保证。
- 本次未部署覆盖 work/edu-server.exe，未重启正式后端，未提交或推送 Git。更新部署前应核对既有首页等同期改动并选定候选版本。


## 旧引擎移除与兼容 · 2026-10-15

用户明确要求删除原动画生成代码。已移除 Manim 执行器、旧模型分镜/修复/几何校验、云视频 Ark/MiniMax/DashScope 适配器、专用 Python 脚本与 Docker 构建、旧专用测试和场景 fixture；前端旧生成/验收按钮、环境示例与配置检查、CI 及冒烟脚本同步清理。删除原文件20个、共4134行（含专用测试和fixture；另有局部删改与新增兼容测试），没有删除实际历史作品文件或业务账单。

`POST /api/tasks` 的 animation 未填引擎时归一为 html；显式旧引擎拒绝。`animation_legacy.go` 只保留历史质量字段/关键帧读取与退役接口响应，不生成视频。旧成品验收路由校验当前企业/成员/未删除作品后返回410，阻止再次扣费；取消仍可释放未结算预留。旧分镜保存为原始JSON，历史 MP4、MD、关键帧仍能按归属读取，删除后的访问仍被拒绝。

课件页进入新工作台时带入课题、页面要求、学科、年级与完成课件ID。后端拒绝其他成员、其他企业、未完成或不存在的来源课件，创建时保存来源快照，后续生成读取该快照而非重新读取可变课件。

验证：先复现默认路由仍停用、历史验收仍可达旧逻辑、来源未检查三个失败用例；完成后运行 Go 测试/vet/候选编译、前端构建、真实Vue与fixture API浏览器流程（15项）及Python渲染回归（218 passed、4 skipped）。隔离 PostgreSQL 回归全部通过：默认 HTML 路由、历史读取权限、来源快照、完整 HTML 工作流、失败恢复与取消见 work/conversation-first/animation-retirement-final.log；历史未验收作品不得扣费、课件预检与单次修复（模拟渲染器）、成员额度隔离与作品重命名/删除见 work/conversation-first/animation-retirement-billing.log。浏览器历史MP4采用假文件字节验证鉴权Blob与控件连接，不冒充真实媒体解码验收。

全仓 ponytail 报告见 `ponytail-audit-20261015.md`。非动画的精简候选仅报告，未擅自重构账户、课件或RAG。

部署状态：当前候选为 `work/anim-workflow/edu-animation-candidate.exe`。本轮检查8080/8000均拒绝连接；启动既有渲染服务的命令被自动审批拒绝（blocked by policy，无更具体原因），未尝试绕过。因此未覆盖正式二进制、未重启后端、未修改固定发布源码目录；工作区变化不能称为已经上线。原发布目录仍为 work/conversation-first/backend-go，后续发布须在运行权限恢复后显式更新发布快照并验证，直接按旧start脚本会继续构建旧快照。混合HTTP回归中的课件确认测试因渲染服务未运行而失败，不能将该项标为通过。未再次调用真实付费模型，未提交或推送Git。
