# Ponytail 全仓库审查 · 2026-10-15

对象为当前工作区（包含尚未提交的 HTML 动画和首页改动）。扫描 Go 后端、Vue/JS/CSS、Python 渲染、PowerShell/浏览器脚本、CI、构建依赖与调用点；不包含依赖目录、生成产物、storage、work 历史部署快照。这里只报告复杂度，未批量改动课件、账户、检索或渲染行为。

## 本次已按用户要求完成的删除

移除旧 Manim 执行器、分镜生成/修复/几何校验、云视频 Ark/MiniMax/DashScope 适配器、专用渲染脚本/镜像、专用测试和旧场景 fixture。旧前端新建/确认/结算入口、供应商配置和 CI 镜像构建一并移除；历史作品读取、下载、鉴权及账务数据保留。删除 20 个旧文件、原计 4134 行（含专用测试/fixture；另有局部删改与新增兼容测试），不把旧接口的拒绝与历史读取兼容代码算作生成引擎。

## 可继续精简（按预计规模排序，尚未实施）

- shrink: 合并 HTML CLI 与 HTTP 中重复的模型请求 payload、唯一响应解析和估算时间轴校验；提取小的纯函数即可，保留 HTTP 持久调用预算、版本/权限检查和 CLI 诊断文件，避免统一成可配置流水线。`backend-go/animmodel.go:67–121`、`backend-go/animworkflow.go:684–711,742–750`，预计净减约 25 行。
- delete: `generate` 中非 courseware、非 animation 的兜底模型生成分支仅由未填写 Type 的旧测试触发；生产调用在 `runCoursewareAgent` 已设定 courseware。将测试显式设为 courseware，保留边界拒绝后可删重复 prompt/JSON 分支，预计净减约 10 行。`backend-go/providers.go:232–242`、`backend-go/providers_test.go:137,142`。需先将测试迁至现行解析及预检入口。
- delete: `parseTime` 在全仓 Go 源码与测试均无调用；删除函数及只为它存在的 encoding/json、time 导入，净减 9 行。`backend-go/billing.go:361–367`。
- delete: 当前 PDF 提取和预览已使用 PyMuPDF，业务代码与脚本没有 pdftoppm 调用。可移除 Docker/CI 安装的 poppler-utils、CI command -v 检查和两行旧说明，净减 3 行及 1 个系统软件包；保留 fonts-noto-cjk。`render-service-py/Dockerfile:2`、`.github/workflows/ci.yml:35,40–41`、`render-service-py/reference_extract.py:17–24`。正式渲染部署仍固定历史快照，必须同步新渲染镜像后再删除运行环境依赖。

## 保留理由

- pgx、JWT、crypto、x/net/html、MinIO、Redis、Vue、图标库、python-pptx、PyMuPDF、Matplotlib 均有实际调用，不能因存在两套存储/检索实现就删已启用功能。pypdf 虽主要用于测试，参与构造特殊 PDF 内容流与提取边界用例，未计作无用依赖。
- `coursewareProviderLimit` 和 `reviewPPTPage` 在生产代码无直接调用，但有明确回归/真实视觉测试用途；不以单次文本搜索把它们误判成可删功能。
- 旧动画历史字段、成员隔离、释放预留、文件清理和 HTML 调用计数属于兼容或正确性要求，不为缩短代码移除。
- 前端旧课件工作台仍用于历史课件；不能用“新工作台已存在”删除它。启动脚本的固定发布源码目录仍是既有部署控制，当前没有偷偷切换或清空该快照。

行数为可审查的保守估算，未实施部分不是收益实测；本次执行不额外引入依赖。

net: -47 lines, -1 deps possible.
