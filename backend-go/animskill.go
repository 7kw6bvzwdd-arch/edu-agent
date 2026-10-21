package main

import (
	_ "embed"
	"strings"
)

const animSkillVersion = "edu-animation-direct-v1"

const animCarbonGuidance = `结构决定性质课题须讲出具体性质及结构联系，例如金刚石空间共价网络与高硬度、石墨层间作用较弱与易滑动；只有“结构不同性质不同”不算完成解释。构建前可呈现中心原子或真实对照，不要开场空白。主体用足高度，标注靠近结构；三结构对照给各模型单独名称。示意和非比例图下注一次即可，不要每幕朗读免责声明。C60旋转是观察方法，不是材料性质。只用既有动作，不伪造断键或力学实验。`

//go:embed skills/edu-animation/references/teaching-composition.md
var animTeachingSkill string

// Topic-specific examples are selected from the teacher's brief, never applied
// as a universal storyboard. HTTP generation, local repair and CLI share this.
func animAuthorInstructions(in TaskInput, instruction string) string {
	return animSkillVersion + "\n" + directAuthoringSkill + "\n技术契约：\n" + directAnimationContract
}

func legacyAnimAuthorInstructions(in TaskInput, instruction string) string {
	brief := strings.ToLower(in.Topic + " " + in.Requirements + " " + in.Objectives + " " + instruction)
	prompt := animAuthorPrompt
	for _, rule := range []struct {
		terms []string
		text  string
	}{
		{[]string{"位置介词", "方位介词", "on/in/under", "in/on/under", "under"}, animPositionGuidance},
		{[]string{"透镜", "焦距"}, animLensGuidance},
		{[]string{"细胞"}, animCellGuidance},
		{[]string{"比喻", "弯月", "小船"}, animMetaphorGuidance},
		{[]string{"双向", "返程", "丝绸之路"}, animRouteGuidance},
		{[]string{"碳单质", "金刚石", "石墨", "c60", "富勒烯"}, animCarbonGuidance},
	} {
		for _, term := range rule.terms {
			if strings.Contains(brief, term) {
				prompt += "\n课题专项提示：" + rule.text
				break
			}
		}
	}
	return prompt
}

const animPositionGuidance = `英语位置课的on表示球与桌面上表面接触，不是悬在桌子上方。归一化绘图区内球中心y必须等于桌面中心y减桌面height一半再减球height一半；in的球轮廓全部在盒内，under的球在桌面下且处于两条桌腿之间。盒子不填充以免遮住内部球。完整英文例句单独放narration，图中只放in/on/under等短标注；card.items每条最多28个字符，英文空格和标点也计数，under完整例句不要再加中文塞进一条。`

const animLensGuidance = `科学提问必须限定本片实际演示的条件。例如仅比较凸透镜物距大于焦距的实像时，末题明确“仍大于焦距”，不能暗示跨焦点结论不变。`

const animCellGuidance = `需要依次高亮多个细胞结构时，按实际讲解顺序安排单个目标并锚定对应旁白；覆盖需求中指定的结构，不用同时高亮冒充依次，不固定幕数。`

const animMetaphorGuidance = `图形须呈现实际比喻的相似点，取材服从教师给出的例句。若例句涉及弯月与小船，用可辨认的弯曲轮廓表现共同点；polygon沿外弧到另一端，再沿凹入的内弧返回，闭合且不自交，不能用凸多边形或矩形代替凹曲轮廓。坐标按本幕构图确定，不照搬固定位置；不要用遮盖块挡住另一图形。图形、原文和附着标注使用scene契约，几何近似明确是示意，不冒充真实比例或教材原图。`

const animRouteGuidance = `双向交流须有方向相反的两组move/follow_path，返程另建points反序的path；移动物的初始中心必须等于所沿路径首点，不能靠初始摆在终点改变路径方向。同向对象错开开始时间或分轨，避免重叠。非地图路线注明“路线示意，非地理方位”，不用未解释的反向东西标记。随物体移动的名称标注用text.attach_to引用图形ID，x/y仍为标注初始中心，程序自动继承目标平移和显隐，不给附着标注另加变换动作。固定说明放图区底部，避免标注漂在旧位置。`
