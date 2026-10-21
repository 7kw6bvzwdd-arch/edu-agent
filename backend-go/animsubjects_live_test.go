package main

import "testing"

// Real providers and isolated HTTP lifecycle; subject names are test selectors.
// These are integration probes, not teacher or factual-quality acceptance.
func TestHTMLSubjectsLiveProbe(t *testing.T) {
	for _, subject := range []struct{ name, subject, topic, requirements string }{
		{"physics", "物理", "凸透镜成像：物距改变时像怎样变化", "用薄透镜专用图示，焦距10厘米、物高3厘米，物距从30变到15厘米，展示光线、像位置和大小变化；两处均是倒立实像，不跨焦点，不能把实像说成正立。代表幕为物距变化，不用纯文字卡。"},
		{"biology", "生物", "植物细胞结构与功能", "用典型植物细胞专用图示，按旁白依次高亮细胞壁、细胞膜、细胞核和叶绿体；说明典型含叶绿体细胞，不说所有植物细胞都有叶绿体。代表幕为叶绿体高亮，不能用文字替代结构。"},
		{"geography", "地理", "水循环：水从哪里来又到哪里去", "用水循环示意或受控图形表现蒸发、凝结、降水、汇流，至少一幕有非文字对象运动。说明是过程示意、非真实比例。代表幕观察水的路径，不用纯文字卡。"},
		{"history", "历史", "丝绸之路：交流如何发生", "用受控图形展示简化商路上货物与人的双向移动，用标注解释交流。无地图素材时只画路线示意，不伪装真实地图、不编造史料或具体年代。代表幕要有非文字对象沿路线移动，结尾提问交流带来什么。"},
		{"chinese", "语文", "比喻：本体与喻体的相似点", "以弯弯的月亮像小船为例，用受控图形实际画出弯月与小船，借展示、移动或高亮说明形状相似，指出本体、喻体和比喻词；这是自拟例句，不冒充课文原文。代表幕必须有非文字图形动作，不能只放两行例句。"},
		{"english", "英语", "位置表达：in、on、under", "用受控图形画盒子、桌面与球，实际移动球展示in、on、under与位置对应。英语例句分别朗读The ball is in the box. The ball is on the table. The ball is under the table. 英语原句与中文解释分开，英语保留原文。代表幕选择under的位置变化，不能仅用词卡。"},
	} {
		t.Run(subject.name, func(t *testing.T) {
			in := TaskInput{Subject: subject.subject, Grade: "七年级", Topic: subject.topic, Requirements: "四幕，每幕两至三句，自动播放60至90秒；先自然引出问题，再观察变化，最后提一个可回答问题，保留有意义的观察停顿。" + subject.requirements, AnimationOpening: "natural", AnimationMinSeconds: 60, AnimationMaxSeconds: 90}
			testHTMLAnimationLiveProbe(t, subject.name, in)
		})
	}
}
