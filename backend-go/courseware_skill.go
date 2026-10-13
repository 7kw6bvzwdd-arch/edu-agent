package main

import _ "embed"

const coursewareSkillVersion = "edu-courseware-v3"
const coursewarePageRepairVersion = "page-checkpoint-v3"

//go:embed skills/edu-courseware/references/teaching-design.md
var coursewareTeachingSkill string

//go:embed skills/edu-courseware/references/page-repair.md
var coursewarePageRepairSkill string
