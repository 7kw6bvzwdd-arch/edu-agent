# 首页模型连接修复（2026-10-17）

## 复现与原因

首页输入“制作二次函数课件”，真实浏览器稳定收到“暂时无法连接模型服务，需求已保留，请稍后重新发送”。短请求和简化后的首页请求能成功，不能作为故障已恢复的依据。

捕获测试浏览器实际发送的完整需求参数，在隔离数据库中重放同一 `/api/workspace/brief` HTTP调用：默认连接及显式HTTP/2各两次失败，服务端记录 `received *http2.WindowUpdateFrame before a SETTINGS frame`；HTTP/1.1两次均收到供应商HTTP 200和真实增量内容，其中一次模型回复未通过应用层校验，另一次完整完成。问题与已有动画模型网关的HTTP/2兼容故障相同，不是成员制作积分不足。未修改密钥或供应商账户设置。

## 修改

- `backend-go/providers.go`：DeepSeek `deepseek-flash` 的 `courseware.*` 和 `slide.rewrite` 请求复用已有、验证TLS的HTTP/1.1连接池，覆盖首页需求、独立课件需求和后续课件生成。保留其他模型与服务的传输。
- 不修改全局HTTP/2设置，不自动重发可能计费的模型POST。
- `backend-go/courseware_connection_test.go`：真实鉴权HTTP入口连接TLS供应商fixture；供应商同时支持协议协商但只接受HTTP/1.1，覆盖两个需求入口的流式与非流式响应，断言四次请求没有重放、公共传输未修改、作品与额度不变。先失败，再修复通过。
- `scripts/probe-workspace-live.cjs`：独立浏览器登录本机真实服务，输入上述原句，验证完成回复、两块需求表格、单次请求和业务状态不变；不点击制作按钮。测试账户凭据只在内存使用，测试结束注销自身会话。

## 验证与发布

- 发布快照 `work/animation-bright-release-20261016/backend-go` 仅同步本次传输修改和回归用例，构建发布；不部署根目录其他未完成改动。
- 发布快照五项入口、权限、供应商限额、断流及HTTP/1.1回归通过。
- 根源码的流式增量、停止取消、中文长度、不创建/扣费、路由、权限和新兼容回归均通过。一次较宽的同库串行验收累计触发已有每15分钟20次对话限制；澄清与新兼容回归在新隔离库重跑均通过。隔离数据库全部清理。
- 发布后原场景真实浏览器连续两次成功，耗时约7.6秒和6.7秒，均仅一次请求，显示完整回复及两块需求表格，作品列表与成员余额、预留、可用积分均不变。真实接口模式和模型为 `live` / `deepseek-flash`。
- 本机8080服务健康，PostgreSQL、Redis、MinIO、Milvus均为ok；8000渲染服务保留。PID及二进制hash已写回本机发布记录。保留 `work/edu-server-before-connection.exe` 作为本次回滚副本。
- 临时协议探针从源码删除；原始测试需求仅保留于忽略的本机work目录，不写入日志或版本库。未制作新的付费作品，不代表整套课件内容质量验收。

可重复检查：`node scripts/probe-workspace-live.cjs`。结果见本机 `work/conversation-first/connection/homepage-live.json`，截图见同目录 `homepage-live.png`。
