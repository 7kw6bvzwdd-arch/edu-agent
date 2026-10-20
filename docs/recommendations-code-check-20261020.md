# 改进建议源码复核（2026-10-20）

本记录仅核对当前工作区源码，不是运行验收。未启动服务、未调用外网或收费模型、未修改业务代码。以下路径均为本机绝对路径，行号对应本次读取。

## 1. 目标映射与依赖检查

- **目标映射仍未实现为结构化覆盖约束。** `TeachingPlan` 只有 `summary`、`conventions`、`pages`；`TeachingPage` 已有单页 `goal`、`task`、`visual`、`minutes`、`depends_on`、`image_need`，但无 `learning_goals`、`goal_refs`、`checks_goals`。在 `backend-go` 和 `frontend-vue/src` 的 Go、TS、Vue 源码中检索后三个字段未找到匹配。依据：`D:/tools/harness/edu-agent/backend-go/courseware_plan.go:13`、`:61`。因此“每个核心目标是否被讲解和练习覆盖”的确定性检查仍是建议项，不能将已有逐页 `goal` 等同于已实现。
- **已有依赖范围及自依赖检查，没有多页环路检测。** `validateTeachingPlan` 检查关联页在 1..页数范围内且不等于本页；未进行 DFS、拓扑排序或其他有向环检测，因此 1→2、2→1 这样的跨页循环不会被这一验证器拒绝。依据：`D:/tools/harness/edu-agent/backend-go/courseware_plan.go:67`、`:79`。
- 现有依赖可用于编辑后审核/教师批准失效传播；该传播采用最多页数次迭代，不是环路拒绝规则。依据：`D:/tools/harness/edu-agent/backend-go/courseware_page_review.go:139`、`:153`。

## 2. 交付前事实核对与审核门禁

- **有逐页内容审核和真实 PPT 截图审核，没有发现独立的交付前全课跨页事实核对步骤。** 逐页审核提示词明确“只审核当前 slides 页面”，`pageForReview` 仅保留本页规划与整课 `conventions`；生成阶段会提供相邻页上下文及全课规划。这些属于一致性约束与上下文辅助，不能等同于对最终全课实际内容逐项比较。依据：`D:/tools/harness/edu-agent/backend-go/courseware_page_review.go:380`、`:415`、`:420`；`D:/tools/harness/edu-agent/backend-go/courseware_checkpoint.go:104`、`:263`。
- **新式有规划课件的 `assessTeaching` 是逐页审核汇总。** `d.Plan != nil` 分支遍历保存的 `PageChecks`、检查 hash 与是否通过，汇总已核查页数及建议后直接返回，不发起新的全课模型审核。旧的无规划分支仍可能执行方案辅助评审，但调用在成品结算之后，不能视为交付前门禁。依据：`D:/tools/harness/edu-agent/backend-go/assessment.go:30` 至 `:65`、`:78`；`D:/tools/harness/edu-agent/backend-go/agents.go:84` 至 `:94`；`D:/tools/harness/edu-agent/backend-go/review.go:17`。
- **实际交付门禁验证页面齐全、审核 hash 当前有效、逐页审核通过。** 未发现该门禁进行全课事实比较或检查目标覆盖率。依据：`D:/tools/harness/edu-agent/backend-go/billing.go:263` 至 `:282`。
- **`uncertain` 不作为阻塞错误。** `parsePageReview` 会把 `uncertain` 的严重程度转为 `suggestion`；`blockingPageIssues` 只收集 `error`。因此其余条件满足时，存在不确定建议的课件可以通过；这不代表该疑点已经事实核实。依据：`D:/tools/harness/edu-agent/backend-go/courseware_page_review.go:249`、`:587`。
- **`disabled` 可在显式关闭内容审核时通过，但视觉审核仍须成功且无阻塞问题。** 配置 `CoursewareContentReviewDisabled`，或策略 `none`，允许内容状态 `disabled`；策略 `key` 下，非重点页允许 `skipped`。这些状态不能宣称全部内容完成核验，汇总报告明确返回 `partial`。内容审核关闭时仍运行已有程序检查。依据：`D:/tools/harness/edu-agent/backend-go/courseware_page_review.go:56`、`:399`、`:637`；`D:/tools/harness/edu-agent/backend-go/assessment.go:50` 至 `:65`。
- 页面模型调用失败、审核状态 `unavailable`，与 `uncertain` 不是一回事：前者不能直接通过正常页面审核门禁。依据：`D:/tools/harness/edu-agent/backend-go/courseware_page_review.go:631` 至 `:658`。

## 3. 图片检索开关与视觉调用上限

- **自动配图受 `UseWeb` 控制，手动检索不读该开关。** 自动入口 `addCoursewareImage` 在 `UseWeb=false` 时设置 `disabled` 并返回；规划提示词也要求关闭联网时不产生 `image_need`。手动 HTTP 入口校验当前成员作品、版本、状态、页面及图形冲突后，直接调用统一图片检索，并设置手动审核上下文，没有检查原任务的 `UseWeb`。这意味着“创建任务时不联网”不会阻止教师之后主动点击手动联网检索。依据：`D:/tools/harness/edu-agent/backend-go/courseware_images.go:71`、`:84`、`:122`、`:165`；`D:/tools/harness/edu-agent/backend-go/courseware_plan.go:111`。
- **网页为主、Commons 备用已接通。** 统一入口先给网页最多 65 秒，网页没有可直接采用图时再给 Commons 最多 25 秒，整体受 90 秒上限约束；网页有待确认候选仍会尝试备用来源。依据：`D:/tools/harness/edu-agent/backend-go/courseware_web_images.go:24` 至 `:65`。
- **“每次图片检索最多 3 次视觉审核”不准确：两种来源各有独立计数器，合计理论最多 6 次。** 网页的 `checked` 在下载和格式通过后递增、最多 3 次视觉调用；Commons 另建 `checked`，在下载前递增，最多尝试 3 张并至多调用 3 次视觉审核。统一入口没有共享计数器；网页三张均未达自动采用条件且备用来源可用时，合计可能 3+3。实际次数还会被超时、候选数提前满足、格式/下载失败及供应商预算所降低。依据：`D:/tools/harness/edu-agent/backend-go/courseware_web_images.go:259`、`:297`、`:310`；`D:/tools/harness/edu-agent/backend-go/courseware_images.go:332`、`:364` 至 `:377`；统一调用链见 `D:/tools/harness/edu-agent/backend-go/courseware_web_images.go:31`、`:41`。

## 对昨天建议的修正

目标映射与交付前跨页事实检查仍应作为新增项；不能因为有逐页目标、全课约定和审核汇总便标为完成。现有分级审核、学科程序检查、视觉审核及图片链路已经存在，应以验收与补齐明确缺口为主。图片相关成本说明须按“网页最多 3 次、备用 Commons 最多 3 次”表述，或后续改为共享总预算；联网关闭时的手动检索是另一个显式动作，产品文案应说明。
