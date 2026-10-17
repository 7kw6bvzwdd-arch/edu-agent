# 首页配色入口与滚动上下文 · 2026-10-17

用户反馈原有配色设置难以找到，并要求首页长对话自动压缩。核对现有首页和独立制作组件发现：7种视觉主题及真实渲染配色均仍存在，但入口被放进默认折叠的教学辅助设置；长对话只发送最近31条消息，另带最新教学要求，没有独立的滚动记忆。

## 本次变更

- 首页输入区下方显式提供“课件配色”，独立制作页也提供同一选择。沿用7种现有主题、当前默认值及任务theme字段。配色仅影响后续制作，不能把更换配色当作消除固定版式或重绘历史作品。
- 真实对话达到25条有效消息（约12轮）或累计12000字符时，请求现有对话模型在同一次回复末尾生成1–3200字context_summary。reply仍为首字段，保持真实增量SSE，不额外串行调用摘要模型。
- 完成并验证成功后才保存新记忆；下一轮使用滚动记忆、最近约3轮消息、当前表单、教师手工摘要与本轮输入。原始聊天和嵌入作品保留。最新明确修改优先，取消内容、未答问题和建议须区分；历史制作命令不构成新授权。
- 摘要缺失、超限、停止、断流或格式失败都不替换旧记忆或有效设置；输入保留可重试。记忆属于当前应用会话，切换成员或企业清空，不存共享服务端会话。浏览器刷新恢复仍未实现。
- health增加workspace_context_version=rolling-summary-v1。没有调整内容审核、教学图解选型、固定版式或图表数量提示。

## 验证

先增加浏览器检查并运行，失败于“Homepage exposes courseware colors without opening auxiliary settings”；增加HTTP检查后运行，失败于“completed context memory was not returned”。实现后根源码和正式候选各9组顶层HTTP回归通过，包含流式完成前增量、停止断连、服务商失败、权限和教学要求校验。后续补充超长模型摘要及成员上下文隔离，专项HTTP通过。

16项浏览器验收通过：7种配色入口、配色随输入传递、任意学科、桌面/移动显示、原对话保留、滚动压缩后二次请求缩短、摘要失败及重试、无重复任务、动画专用成员权限。浏览器模型内容为fixture，未标为真实AI结果。

真实8000渲染两份单页PPTX，分别确认ocean的164B9B和coral的B84F3F原生颜色存在于slide1.xml。测试PPTX为本地固定样例，不是模型生成内容。

另在隔离测试成员上调用真实DeepSeek流式压缩一次：返回295字摘要，保留史料讨论、已取消表演及答案放教师备注；action为discuss，未创建任务、未改变成员业务积分。该试测证明此输入可用，不保证任意长会话摘要永远无遗漏。

证据：work/conversation-first/context-red.log、context-green.log、context-candidate.log、context-final.log；work/needs-dialogue/browser/checks.json；work/home-context-20261017/verify-colors.py。所有临时数据库已清理，无密钥写入日志。

## 发布

2026-10-17约13:20（Asia/Shanghai）已发布8080；发布前活动制作/资料/对话任务0。以正式all-subjects快照为基线，只带入courseware_brief.go和health新增版本字段，未整体发布根源码其他改动。后端PID11292，SHA256为5728C1E9956A264DA4AEE7625A8C92D484489BEAA621314CC75BC8015D525D52；8000渲染PID16368及.env均保持。

正式源码：work/home-context-20261017/backend-go。发布脚本与hash清单位于该release目录；回滚目录为work/home-context-20261017/rollback-20261017-132045。前端入口和实际资源HTTP一致，全部既有health能力逐项保持。发布后真实账号只读UI、作品/积分未变检查记录在work/home-context-20261017/browser/checks.json。
