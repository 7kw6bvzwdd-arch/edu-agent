---
name: edu-animation
description: Author and revise teaching animations in edu-agent using controlled AnimScript data, meaningful visual motion and sentence anchors.
---

# 项目动画技能

读取 [教学构图指导](references/teaching-composition.md)，结合实际教师需求选择教学对象、观察任务和动作。模型仅输出现有 AnimScript；技术契约由后端单独注入并优先执行。

参考 HyperFrames 的 hyperframes-creative、hyperframes-animation 与 faceless-explainer，来源及取舍见 [来源记录](SOURCES.md)。这是后端适配规范，不是 HyperFrames HTML 工作流；采用现有受控播放器、句子时间轴及审阅关口。

manim-video 与 frontend-slides 的教学节拍、分部揭示和视口容量原则已蒸馏进上述指导；安装、归档指纹及取舍见 [新增来源](../skill-distillation-20261021.md)。后端编译嵌入，开发者安装全局技能不是线上生效条件。

完成条件：实际发送给模型的请求带有本规范；关键解释由可辨认图形及语义动作承载；逐句定位、回退播放、离线 HTML 和 MP4 与同一作者数据一致。技术检查通过不等于教师验收。
