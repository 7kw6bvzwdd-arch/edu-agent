# 课件内容模型审核临时停用 · 2026-10-15

用户要求暂时断开DeepSeek审核。正式8080后端已按此指示更新；只应用该开关及相关检查状态，8000渲染器保持原PID和文件hash，P0/P1整批候选仍未发布。

- 配置：`COURSEWARE_CONTENT_REVIEW_DISABLED=true`。默认false；原模型地址和密钥保留，不改为生成模型代审。
- 停用期间不调用课件内容模型审核或旧课件辅助评审。页面内容审核记录为`disabled`；仍需程序校验、真实截图视觉审核和教师逐页确认。
- 前端、作品说明及评审汇总明确标记停用，不能将其称为双审核通过。重新启用后，`disabled`记录不能满足内容审核关卡。
- 原任务`94c91b07f5fea7ac7f91646afbb83817`在等待恢复且没有执行中任务时维护，保留任务、页面、charge cycle与预留；只调整恢复调度时间，没有重新提交任务或新增业务积分预留。

## 验证

先复现停用后仍阻塞的HTTP用例，再实现对应行为。隔离数据库中`TestPausedContentReviewResumesSavedPageHTTP`通过：恢复已保存页面、三页真实PPTX预览、停用后零内容模型调用、明确disabled标记、重新启用后拦截旧记录、跨成员访问拦截，以及未配置内容模型时仍可创建任务。原有`TestCoursewareThreeSlideGateHTTP`与`TestHTTPMemberQuotaAndIsolation`通过，覆盖教师确认、交付和额度。前端构建、审核职责回归及Go vet通过。

实际服务健康检查确认内容审核关闭、截图审核配置有效，PostgreSQL/MinIO/Redis/Milvus与渲染器正常。原任务第1页已完成截图审核，内容标记disabled，内容hash保持原值，开始第2页制作；上线后的DeepSeek审核调用数为0，任务仍为原20积分预留状态，尚未交付结算。

证据：`work/content-review-paused/disabled-final.log`、`disabled-green.log`（保留其中已修复的说明覆盖失败）、`live-verification.json`、`task-before-restart.json`及正式部署记录。测试使用模拟模型和真实渲染；实际原任务使用真实GLM与Qwen截图审核。本记录不代表整套课件或教师课堂验收通过。
