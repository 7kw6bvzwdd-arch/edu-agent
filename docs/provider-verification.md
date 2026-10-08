# 接口核验记录

核验日期：2026-10-05。

用户提供：`https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks`。仅地址，不包含API key、Model ID、搜索服务信息。

通过火山引擎官方Python SDK GitHub仓库获取并核对了源码：

- https://github.com/volcengine/volcengine-python-sdk/blob/master/volcenginesdkarkruntime/resources/content_generation/tasks.py 使用 `/contents/generations/tasks` 及任务ID路径。
- https://github.com/volcengine/volcengine-python-sdk/blob/master/volcenginesdkarkruntime/resources/chat/completions.py 使用 `/chat/completions`。

所以课件客户端填写基础URL `https://ark.cn-beijing.volces.com/api/v3`，调用文本生成路由。既定动画路径仍是文本模型生成Manim代码 + 隔离渲染，没有擅自把动画实现改成另收费的远程视频任务接口。

真实响应参数、所选模型JSON稳定性、token账单、权限和配额尚待可用模型凭证确认，不能以源码路径核验代替真实联调。

竞品参考：尝试 `https://teacher.xfyun.cn/` DNS未解析；`https://xinghuo.xfyun.cn/` 返回200且标题为“讯飞星火-每个人的AI助手”，但这不能证明星火教师助手的具体交互功能。当前前端是按备课流程设计的原创界面，不声称还原或完整调研了该竞品。
