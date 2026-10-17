# 2026-10-17 整改与发布记录

用户要求将14项问题一起处理。此文区分代码实现、自动验收、真实模型试测和正式发布；最终运行身份以 /api/health 与 work/active-release.json 为准。

## 逐项整改

| 项 | 实现 | 验收及边界 |
|---|---|---|
| 1 全科核验 | key/all/none 分级策略；例题、答案、推导、史实、原文、安全等重点页送独立模型；按学科注册程序检查；逐页核验状态 | 数学/化学/历史 HTTP；真实审核阻断语文、历史及化学已知错误。17组学科接口、语文/英语/历史真实PPTX使用模型fixture，不等于课堂质量验收 |
| 2 中断恢复 | 60秒续三分钟租约；attempt fencing；正常退出回队，异常退出租约后恢复；独立扫描 | HTTP续租与检查点通过；真实子进程退出/强杀/重启通过56.91秒，异常用例主动缩短租约 |
| 3 教师待审 | 移除24小时待审自动失败，保留预留直到取消或交付 | 两天待审及取消只释放一次HTTP通过 |
| 4 并发公平 | 两个制作worker，企业轮转；独立一个视频worker；动画等待两分钟后进入制作池避免饥饿 | 两企业并发及续租HTTP、动画排队红绿回归通过；不是无限吞吐 |
| 5 速度成本 | 规划请求6分钟；扩展按剩余页预算，上限3小时；重点审核减少内容调用 | 同三页历史实际审核 none 5.7345秒/key 8.0026秒/all 10.4964秒，截图缓存预热；整课生成测速另见下文，不能混作整课用时 |
| 6 旧API | 所有新课件统一staged流程，省略preview_first不能绕过检查 | 旧请求HTTP改验三页预览、审核、扩展及交付 |
| 7 认证限流 | 登录/改密IP及账号+IP两级计数；不存在账号同cost10 bcrypt | 认证/找回/会话/限流HTTP通过；保留真实业务隔离 |
| 8 模拟支付 | live默认拒绝自助模拟入账；显式演示开关可启用；sample模拟支付与管理员分配保留 | live拒绝/显式演示/幂等入账HTTP通过；未接真实支付 |
| 9 统一发布 | 保存原工作区；发布基线tag；精确tag构建全部组件并校验SHA256及二进制身份 | baseline隔离启动与PPTX通过；新候选发布状态见下文，不移动baseline tag |
| 10 CI | Linux/Windows；随机隔离库凭据；Go、Python、Vue、浏览器、真实MP4及进程恢复；Windows逻辑按平台文件 | YAML已解析、本地分项验收通过；远端执行状态另记 |
| 11 认证最小权限 | 固定search_path SECURITY DEFINER证明校验；撤销runtime对密码/恢复码及sessions直读；禁schema CREATE；显式随机密码provision | 最小权限、NULL证明、全新隔离PG容器从无edu_app初始化通过；凭据轮换状态另记 |
| 12 运维 | slog JSON/request ID/panic恢复/SSE；DB/storage/MinIO prefix校验备份、随机隔离恢复；每日03:30计划 | 两次实际恢复通过，最新476对象；当前计划需用户已登录，备份不自动清理；跨存储一致时点须停写 |
| 13 前端 | hash保存页面/task；按企业成员sessionStorage恢复会话、草稿及设置；中断回复标记；迟到响应身份保护；恢复focus-visible；格式化并拆出账单、会话持久化、审核资料组件 | 首页20项/动画49项浏览器通过。App与Studio仍较大，已拆独立职责，未宣称全部组件重构 |
| 14 文档与导出 | 文档改为宿主机受控HTML+Chrome+FFmpeg；临时cwd/环境白名单；Windows拒绝提权，Linux拒绝root并清capabilities | 真实MP4 HTTP鉴权/重复导出不扣费/删除失效通过183.02秒；不是独立账户或文件系统沙箱 |

## 本地证据

- Go test/vet，release CLI/完整性通过；Python209通过3跳过；Vue格式及生产构建通过。
- work/conversation-first/remediation-process.log：实际进程恢复。
- work/conversation-first/remediation-final-green.log：真实MP4及动画排队。
- work/conversation-first/remediation-animation-green.log：三类动画完整HTTP及恢复/取消。
- work/conversation-first/remediation-subjects.log：17组学科及语文/英语/历史各五页真实PPTX；最后资料用例总超时未算整次通过，资料已单独通过 remediation-materials.log。
- work/release-verification/fresh-database.json：全新隔离PG初始化，业务库写入0。
- work/release-verification/review-policies-live.json：真实同页审核对比及已知错误拦截。
- work/needs-dialogue/browser/checks.json、work/anim-workflow/browser/report.json：浏览器fixture。
- work/backups/20261017T072007Z-2e6d53/restore-drill.json：数据库/storage/MinIO恢复。

## 发布进度

当前尚待统一候选tag构建、隔离启动/回滚、正式停写备份、数据库凭据轮换与服务切换。切换后追加版本身份和只读检查，不以文件存在代替发布验收。

## 质量边界

模型核验不是事实正确保证；全科真实课堂验收需教师完成。历史快照只有在无进程/脚本引用并保留可回滚产物后才可清理。旧基线数据库迁移会扩大权限，因此禁止直接用旧版启动正式库做回滚验收。
