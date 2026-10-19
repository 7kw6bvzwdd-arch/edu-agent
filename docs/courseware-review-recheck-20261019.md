# 课件缩略图与历史核验恢复（2026-10-19）

截图中的历史课件保存了 content=disabled、visual=checked。当前服务恢复 key 策略后，旧检查不满足教师审核门禁；刷新或重复确认不会补做核验。缩略图直接显示长说明，又继承按钮 nowrap，文字越界叠在相邻页上。

缩略图改用简短状态并允许换行，完整说明保留在 title 和本页核对信息中。旧停用说明明确为生成时状态，避免误称服务当前停用。右侧增加“重新检查内容与画面”，强制调用既有免费 draft/edit 保存与 checkEditedPages 流程，保留教学内容、通过的检查和未变化页面的教师确认。核验问题显示对象、依据和修改建议。补检失败保留原稿，可重试；补检不代替教师确认，不自动扩展或交付。

验证：

- 浏览器先复现 2048 像素下提示宽度 288px 超出 88px 缩略图；修复后 2048/1440/1024/390 像素均不越界。
- 浏览器 fixture 覆盖重新检查失败保留、重试、旧草稿未改内容仍提交、无自动审核/扩展/交付，以及问题说明展示。
- 隔离数据库 HTTP：TestPausedContentReviewResumesSavedPageHTTP、TestCoursewareThreeSlideGateHTTP、TestRemediationReviewPolicyHTTP 通过。模型均使用明确测试响应。
- 扩展阶段的旧测试按稳定页面 ID 核对代表预览内容，适应已有“完整课件恢复教学顺序”的行为；未改生产后端逻辑。
- 截图所示现有历史课件经真实内容与画面补检，三页均 checked、无阻塞问题；revision 11→12，三页 hash 完全一致，仍 awaiting_preview，教师确认数 0，balance/reserved/available 不变。

运行证据位于 work/review-fix-20261019/。本地构建和提交，不新增或推送 Git tag，不推送 GitHub。
