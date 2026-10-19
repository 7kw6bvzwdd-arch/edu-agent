# 首页对话阅读改良 · 2026-10-19

原实现已在25条有效消息或12000字符时滚动压缩模型记忆，但每轮仍展开两张完整需求表、确认要求、建议及历史问题。新增浏览器断言修改前失败：聊天内可见表格2张，期望0张。

## 改动

- 沿用Vue、原生details、SSE及压缩协议，无新依赖。完整当前需求位于聊天上方唯一折叠栏，可编辑八项设置、已确认要求及手工摘要。
- 每轮保留自然回复、本轮变更字段及最新追问；建议默认折叠且未确认。历史需求是只读折叠快照，过时问题不再提供操作。
- 默认显示最近六条消息，旧记录可全部展开。用v-show保持嵌入课件/动画挂载，不清理sessionStorage聊天。
- 上翻阅读期间流式输出不强制滚动，主动返回最新恢复跟随。直接制作、停止及异常输入保留沿用原逻辑。
- 模型后续回复只针对新增/修改及教学影响，通常120字内；问题选项只放questions。完整已确认要求仍以requirements字段传递。

## 验证

- `node scripts/browser-needs-dialogue.cjs`：22组检查通过，包括真实ReadableStream分块、唯一默认折叠编辑器、多轮无重复表格、四种视口、停止/错误恢复、双工具嵌入作品、长对话压缩失败重试、历史展开、流式阅读保持位置、刷新和成员隔离。API全部fixture，不创建正式任务。
- 隔离PostgreSQL HTTP：TestWorkspaceContextCompactionHTTP、TestCoursewareBriefStreamsBeforeCompletionHTTP、TestWorkspaceBriefClarifiesBeforeGeneratingHTTP通过。随机数据库创建后清理。
- 真实DeepSeek隔离试测：TestLiveWorkspaceNeedsHTTP三轮和TestLiveWorkspaceContextCompactionHTTP通过；保留史料讨论、已取消活动和答案教师备注，无制作任务或成员积分变化，模型调用有供应商成本。
- 旧HTTP测试首次失败源于取基线后切换sample至live，模拟支付开关按设计变化。改为在取基线前设live，仍完整比较账务响应和任务列表，生产账务逻辑未变。
- Vite构建、浏览器脚本语法和diff空白检查通过。运行切换及正式页验证单独保存在work证据。

证据：work/home-dialogue-20261019/；fixture截图：work/needs-dialogue/browser/。不以fixture截图代表真实账号。
