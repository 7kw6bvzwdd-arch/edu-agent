# 通义万相接入与实测 · 2026-10-06

用户提供了 wan2.7-t2v 的视频创建示例。项目已增加 dashscope 供应商，使用既有 VIDEO_API_KEY，地址更新为 https://maas.qianwenaiapi.com/api/v1，保留5秒默认时长。

## 协议与边界

- 创建：POST /api/v1/services/aigc/video-generation/video-synthesis，X-DashScope-Async: enable，模型 wan2.7-t2v，input.prompt，parameters 为720P、16:9、prompt_extend=true、watermark=true、duration=5。
- 查询：GET /api/v1/tasks/{task_id}；映射 PENDING/RUNNING/SUCCEEDED/FAILED/CANCELED/UNKNOWN。
- 创建不自动重试；远端ID持久化。此通道未接入远端取消，本地取消不承诺供应商免计费。
- 密钥仅发给配置的平台API，下载不带该密钥；复用公开地址限制、30MB限制、静音转换及完整帧解码检查。
- 创建参数依据用户示例，查询/响应依据[通义万相文生视频文档](https://help.aliyun.com/zh/model-studio/text-to-video-api-reference)。此次真实请求验证了该兼容平台的创建和查询协议。

## 真实调用结果

- 本地任务：687ed35eef6d6b420a6e8c642f4f3d4f，管理员账号，木块漂浮情境；分镜来自真实文本模型，确认时使用已审核的5秒分镜。来源不足明确标待核验。
- 外部任务：a8dad425-fab2-49eb-987e-7f29535211c3；只创建一次，无第二次付费视频生成。
- 平台返回开始2026-10-06 14:53:47，结束15:11:17，约17分29秒；最终 SUCCEEDED，usage.output_video_duration=5。
- 原项目7分钟查询超时先结束了本地任务，原任务为 failed/released，没有交付结算。后续只读查询查到云端成功，随后通过正式下载和媒体检查函数取回成品。
- 恢复文件：work/wan-recovered/1/wan-existing.mp4，5.034秒、1280×720、H.264、无音轨、1,755,893字节，完整解码通过。
- Edge本地文件加载、播放时间推进、无运行异常三项通过。截帧可见水槽、木块及AI生成水印；这不等同于物理精确仿真或教学内容审核通过。
- 未把原失败任务改为成功、未重新预留或追扣积分。供应商实际费用以平台账单为准。

## 超时修复

DashScope云端轮询上限改为30分钟，对应渲染工作上下文32分钟、任务租约33分钟，其他供应商仍保持原有时限。过程记录提示实测耗时和勿重复提交。租约和上下文必须一并扩大，不能只改轮询超时。

新增真实HTTP工作流测试在供应商提交时读取租约，修复前约600秒并失败，修复后须超过32分钟。30分钟仍是有界等待，不保证所有平台任务都能在此时间内完成。单worker串行执行和服务重启后不自动恢复外部任务的限制保留；本次未新增任务调度框架。

本次真实测试是“单次创建→云端成功→只读取回→正式媒体检查→浏览器播放”；由于原任务提前超时，不声称已完成真实业务成功结算闭环。修复后业务成功/失败/取消/损坏视频/成员隔离/单次结算由本地模拟供应商与真实MP4的HTTP回归覆盖，避免为重复验收再支付一次生成费。

证据：work/wan-remote-status.json、wan-browser-validation.json、wan-live-result.json、wan-regression-go.log。恢复用例归档在 work/wan-recovered/wan_recovery_debug_test.go.txt；未保存API密钥。
