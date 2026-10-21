package main

import _ "embed"

const coursewareSkillVersion = "edu-courseware-direct-v1"
const coursewarePageRepairVersion = "page-checkpoint-v3"

//go:embed skills/edu-courseware/references/teaching-design.md
var coursewareTeachingReference string

// Shared by whole-lesson planning and every existing page authoring entrypoint.
//
//go:embed skills/edu-courseware/references/composition.md
var coursewareCompositionSkill string

//go:embed skills/direct-authoring.md
var directAuthoringSkill string

var coursewareTeachingSkill = directAuthoringSkill

//go:embed skills/edu-courseware/references/page-repair.md
var legacyCoursewarePageRepairSkill string

var coursewarePageRepairSkill = directCoursewareContract + "\n只修复当前指定页，根据实际教学要求自主绘制完整html；不改其他页，notes保留依据与完整讲解。"
