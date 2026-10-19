# Ponytail 精简及运行验收 · 2026-10-19

## 本次变更

- 删除无人调用的 parseTime 和首页 price 状态。
- 合并动画 CLI/HTTP 的模型 payload、响应结构及估算时间轴检查；保留各自调用预算、取消、修复与版本检查。
- 删除没有生产调用者的通用生成兜底分支，明确只接受 courseware，旧动画拒绝逻辑保留。引用来源映射仍供修复检查和页脚使用，保留。
- 21 个浏览器脚本共用 35 行 CDP 通信工具；各脚本的场景、模拟接口和验收断言分别保留。
- 删除 176 行重复画布脚本，原断言由已实际通过的 browser-courseware-inline-text.cjs 覆盖并扩展。
- 更新过时测试：资料外层展开、现行独立制作表单和画布编辑入口、保存后关闭编辑器、hash 页面恢复、检查点续作状态。旧行为在精简前对照同样失败，未通过修改业务来迎合旧断言。
- 业务与检查脚本合计增加190行、删除416行，净减226行；未新增或移除依赖。
- 保留 Poppler：reference_extract.py 的真实参考文件处理仍调用 pdftoppm。旧审查把候选 PyMuPDF 路径误认为已替代生产路径，本次未按该建议删除。

## 构建及功能验收

- Go test 全套通过（158条通过事件，167条因缺少显式隔离数据库环境而跳过；含子测试，不能当作独立功能数），go vet通过。首次收尾重跑有一次 Windows TempDir 清理占用失败，无代码变更重跑通过。
- 另建随机隔离数据库逐项执行11组HTTP检查，全部通过：模型协议/伪造引用、认证/成员额度隔离、三类动画、worker续租与优雅退出、长期待审、过期租约、登录限流、课件编辑预览下载、并发续作预留。未在正式业务库执行测试写入。
- 前端 Prettier 和 Vite 构建通过；24份浏览器脚本语法检查及课堂答案检查通过。
- 浏览器110项通过：首页流式需求20、动画49、内联编辑13、完整画布10、账户恢复11、会话续期3、作品轮询4。接口全部为显式fixture，无真实收费模型调用。
- Python渲染209项通过；另显式启用已安装PowerPoint，3项真实文字/原生公式排版边界检查通过。依赖产生1条Starlette弃用警告。
- 实际MP4导出、H264/AAC、10条音轨、完整时长、断网资源限制，以及浏览器播放和4处seek检查通过；音频为明确标注的测试音频。浏览器记录了本机安全软件注入的样式CSP警告，编码与播放断言通过。
- 发布工具精确commit构建及完整性/篡改/路径越界检查通过。

## 运行切换

运行切换前：104条任务、执行中0、队列0、待教师审核1、异常预留0。教师可见13条任务，余额940、预留20、可用920。

已于2026-10-19 09:56（Asia/Shanghai）切换本机8080/8000，运行代码commit为0f0bfaa7557f67acfa72a0a21fd410cfae560eae，固定产物目录work/releases/commit-0f0bfaa7557f。目录及兼容元数据中的commit前缀只是发布标识，未创建Git tag。

- 88个产物文件校验通过，后端报告commit、实际前端HTML及两份JS/CSS资产哈希与manifest一致。
- PostgreSQL、Redis、MinIO、Milvus均健康；仍为DeepSeek deepseek-flash、重点内容核验开启、2个制作worker及1个视频worker。
- 重启后104条任务、执行中0、队列0、待教师审核1、异常预留0；教师可见13条任务，余额940、预留20、可用920，与切换前完全一致。
- 已有PPTX下载仍为89001字节，SHA256 f78519520246d191f4fb6c1f82ef2f93f5ed83533a5f265d327de314dfc7a226，切换前后相同。匿名任务、额度、SSE和下载入口拒绝访问。
- 真实账号浏览器：未发送草稿刷新恢复、hash恢复旧课件工作台、实际历史预览下载通过；任务与额度未改变，无付费需求对话或生成。
- 当前运行的8000渲染接口新生成3页真实PPTX（47864字节），包内页数检查通过。
- 启动后后端和renderer日志无ERROR/panic/Traceback标记。
- 生产只读核验及上述渲染检查未创建正式任务、未消费模型额度。真实模型全学科内容质量不在本次技术验收结论内。

证据位于 work/ponytail-20261019/：http-final-results.json、go-final.jsonl、go-vet.log、frontend-format.log、frontend-build.log、render-after.log、office-after.log、mp4-after.log、各浏览器日志；production-after.json、production-browser.json、database-after.json及service-switch.log记录运行后证据。

未新增或推送Git tag，未推送GitHub。此次为技术运行验收，不代表真实AI全学科课堂内容已由教师验收。
