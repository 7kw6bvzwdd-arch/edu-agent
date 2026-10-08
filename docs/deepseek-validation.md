# DeepSeek 官方评审联调 · 2026-10-06

本地已填写独立评审密钥。真实调用官方 `https://api.deepseek.com` 的 `deepseek-flash`，未调用视频生成，未修改真实用户作品或扣业务积分。

## 发现与修复

初次小额度请求不能解析正文。随后最小请求返回 HTTP 200，但 `finish_reason=length`、`content` 为空，500个输出 token 全部是 reasoning token。问题是默认思考模式耗尽测试输出预算，并非密钥或接口不可用。

依据[官方思考模式文档](https://api-docs.deepseek.com/guides/thinking_mode)，仅在 `deepseek-flash` 的 `.review` 操作中发送 `thinking: {type: "disabled"}`，保留 JSON 输出约束。未改变课件生成或其他模型的参数。

## 验证

- 回归测试先模拟思考耗尽预算返回空正文，修复前失败、修复后通过。
- 使用项目 `assessTeaching` 实际代码、隔离测试数据库、合成教学材料进行一次官方联调，约4.3秒完成。结构化报告、五项0–5整数评分、页码和来源ID校验均通过。
- 测试材料含完全浸没后浮力随深度线性增大的错误表述；评审给出相关风险和改写建议。这只是一次接口与样例检查，不构成教学质量全面验收。
- 独立评审路由、生成接口和多智能体路由回归，以及 `go vet`、后端构建通过。
- 真实联调需显式设置 `TEST_DEEPSEEK_LIVE=1`，默认测试不会发出付费评审请求。测试证据保存于本地忽略提交的 `work/deepseek-review-validation.txt`。
- 再次验证扩展到应用HTTP流程：管理员登录 → 评审隔离库合成作品 → 官方模型返回 → 保存带修订号的评审 → 任务详情读取 → 业务账单前后不变，全链路通过（约6.2秒）。相关性过滤与独立密钥路由回归同时通过。证据为 `work/deepseek-http-validation.txt`。当前运行后端仍是15:53启动的旧进程，因此该结果证明最新代码在隔离测试环境可用，不代表现有页面已加载更新。

新后端已构建为 `work/edu-server-next.exe`。当前运行的仍是旧后端；此前自动审批策略拒绝后端替换重启，本轮没有再次尝试。需要在本地依次运行 `scripts/stop.ps1`、`scripts/start.ps1`，才能加载最新代码及 `.env`。
