# 网页端教学图表接入

2026-10-15 已将教学图表能力接入课件网页工作台和 PPTX 导出链路。

## 网页流程

教师从首页对话描述课题、函数或实验数据。生成草稿后，在页面右侧选中课件页即可直接编辑图表：添加实验数据图、选择散点图或散点加误差线、修改真实横坐标和纵坐标、填写来源及误差说明，然后保存当前版本并重新下载 PPTX。无效数值会在页面提示，并由后端再次校验。

实验图的横坐标按数值比例绘制，例如 1、2、5 的间距不是等分。误差线必须由用户或来源数据提供，系统不会猜测实测误差。初始示例明确标为“示意数据，非实测”。

## 技术栈

- Vue 3 + Vite：`TeachingPlotEditor.vue` 负责编辑表格、校验提示和保存前的交互；`TeachingChart.vue` 负责网页 SVG 预览。
- Go HTTP 服务：复用现有任务、版本、成员隔离和额度规则；`courseware_plot.go` 校验图表结构，并在渲染服务缺少 `matplotlib-v1` 时拒绝静默降级。
- Python FastAPI + Matplotlib Agg：`teaching_plots.py` 使用 Figure/Axes 对象绘制函数图、散点图和误差线图，输出高清 PNG 嵌入 PPTX；不执行用户或模型生成的 Python 代码。
- python-pptx：保留 `bar`/`line` 原生可编辑图表；Matplotlib 图以带参数描述的图片形状写入 PPTX，便于网页重画和追溯。

## 验证与运行

Python 渲染回归：144 passed、3 skipped。Go HTTP 测试覆盖认证、企业/成员隔离、保存、旧版本冲突、下载和额度不变。网页浏览器回归覆盖桌面、短屏和移动端布局。运行健康标识为 `courseware_plot_version: matplotlib-v1`；DeepSeek 内容审核仍关闭，动画服务仍停用。
