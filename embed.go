// Package cfexcli carries files that ship inside the cfex binary.
package cfexcli

import _ "embed"

// Skill is the agent skill installed by `cfex skill install`.
//
//go:embed skills/cfex/SKILL.md
var Skill []byte
