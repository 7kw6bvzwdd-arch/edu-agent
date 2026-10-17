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
| 9 统一发布 | 保存原工作区；固定完整commit或已有tag构建全部组件并校验SHA256及二进制身份 | 既有tag不移动、不新增；后续本地提交与验收，不自动推送；新候选发布状态见下文 |
| 10 CI | Linux/Windows；随机隔离库凭据；Go、Python、Vue、浏览器、真实MP4及进程恢复；Windows逻辑按平台文件 | YAML已解析、本地分项验收通过；远端执行状态另记 |
| 11 认证最小权限 | 固定search_path SECURITY DEFINER证明校验；撤销runtime对密码/恢复码及sessions直读；禁schema CREATE；显式随机密码provision | 最小权限、NULL证明、全新隔离PG容器从无edu_app初始化通过；凭据轮换状态另记 |
| 12 运维 | slog JSON/request ID/panic恢复/SSE；DB/storage/MinIO prefix校验备份、随机隔离恢复；每日03:30计划 | 停写备份及476对象恢复通过；当前计划需用户已登录，备份不自动清理；跨存储一致时点须停写 |
| 13 前端 | hash保存页面/task；按企业成员sessionStorage恢复会话、草稿及设置；中断回复标记；迟到响应身份保护；恢复focus-visible；格式化并拆出账单、会话持久化、审核资料组件 | 首页20项/动画49项浏览器通过。App与Studio仍较大，已拆独立职责，未宣称全部组件重构 |
| 14 文档与导出 | 文档改为宿主机受控HTML+Chrome+FFmpeg；临时cwd/环境白名单；Windows拒绝提权，Linux拒绝root，清继承/ambient capabilities并启用no-new-privs | 真实MP4 HTTP鉴权/重复导出不扣费/删除失效通过183.02秒；不是独立账户或文件系统沙箱 |

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

## 真实整课测速

生成使用DeepSeek deepseek-flash，内容审核使用独立GLM，截图审核使用视觉模型；三种策略均独立生成同一历史课题“辛亥革命的背景与影响”五页，实际交付PPTX。

| 策略 | 整课秒数 | 模型调用 | 内容审核调用 | 页面生成调用 |
|---|---:|---:|---:|---:|
| none | 169.17 | 12 | 0 | 5 |
| key | 253.27 | 24 | 5 | 8 |
| all | 203.41 | 19 | 5 | 6 |

历史五页均涉及重点史实，key同样核验五页；各次修复次数不同，单次对照不能证明key一定更快。默认key按风险选择审核范围，不承诺固定提速比例。语文“静夜思中的意象与情感”五页key约182.95秒，化学“化学反应中的质量守恒”五页key约176秒，两者真实生成/审核/交付均通过。教师审核由测试自动操作，不表示教师已认可课堂质量。

证据：work/release-verification/live-matrix.json、full-deck-policy-benchmark.json及work/courseware-full-quality各科report.json和complete.pptx；五个隔离测试库均清理，未占用正式成员额度。

## 发布进度

2026-10-17 18:11（Asia/Shanghai）已切换本机8080与8000到固定提交d9e878768a0d3a352e570677ef18943ec65ace6a，发布目录work/releases/commit-d9e878768a0d。88个文件SHA256、二进制身份及在线前端资源哈希一致。目录名及health中的release_tag是兼容字段，不代表创建新Git tag。

远端CI 39022701328的Linux与Windows均成功，对应baa96b83c81093bc85ea891b7f75784f1e166676。覆盖Python、Go、HTTP认证/隔离/额度/恢复、Vue、浏览器、真实MP4编码/解码及下载鉴权，Windows保留管理员导出拒绝检查并完成固定commit构建。此后发现旧文件采用相对路径，切换工作目录导致下载500；d9e8787固定原数据路径基准，显式调用发布内Office脚本。本地Go test/vet、发布CLI、构建、真实Office截图、正式HTTP及浏览器验收通过。此修复依用户要求仅本地提交，不能称其通过远端CI。

切换前任务执行与排队均为0。停写备份work/backups/20261017T100036Z-ce24ff实际恢复验证users5、members6、tasks104、ledger244，额度不变量异常0；本地文件及476个MinIO对象恢复到隔离位置并清理。随后轮换edu/edu_app随机密码并同步私有配置，新凭据连接成功，两个旧凭据均为密码认证失败28P01，runtime无管理员或绕过RLS权限。恢复材料受本机ACL保护，不提交密钥。

发布后真实账号检查：13条可见任务、1项教师待审均保留；balance940/reserved20/available920与切换前一致。旧PPTX为89001字节，SHA256 f78519520246d191f4fb6c1f82ef2f93f5ed83533a5f265d327de314dfc7a226，前后完全相同。匿名任务/额度/SSE/下载均401。重点内容核验已启用，生成仍为DeepSeek，2个制作worker和1个视频worker生效，四项依赖正常，live自助模拟充值关闭。

真实浏览器验证刷新恢复未发送草稿、hash恢复当前课件及旧预览下载，任务和额度不变，未调用付费模型。证据：work/release-verification/production-after.json、browser-production/production-browser.json、credential-verification.json、ci-current.json和work/active-release.json。

11个旧工作目录内的12个backend-go/frontend-vue源码目录已核对无运行进程引用，移到work/retired-source-snapshots/20261017，保留旧日志、样例及构建产物；移动清单见work/release-verification/retired-source-snapshots.json。后续只从统一构建目录启动。保留旧产物不代表可跳过数据库兼容检查，禁止直接重启会恢复旧认证权限的基线。

日常修改按用户要求只本地提交和验收，不自动推送GitHub，不新增或移动tag。远端CI修复的实际问题包括共享测试队列、恢复测试旧状态竞态、Linux普通用户无权修改bounding set、课件打开迟到响应及Windows提权身份验收。

## 质量边界

模型核验不是事实正确保证；全科真实课堂验收需教师完成。历史快照只有在无进程/脚本引用并保留可回滚产物后才可清理。旧基线数据库迁移会扩大权限，因此禁止直接用旧版启动正式库做回滚验收。
