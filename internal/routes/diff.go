package routes

import (
	"reflect"
	"strconv"
	"strings"
)

// Change is a human-readable description of a draft change, never an apply plan.
type Change struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func Diff(old *Policy, next Policy) []Change {
	changes := []Change{}
	if old == nil {
		changes = append(changes, Change{Field: "policy", Before: "not present", After: "new draft"})
		old = &Policy{}
	}
	add := func(field, before, after string) {
		if before != after {
			changes = append(changes, Change{Field: field, Before: before, After: after})
		}
	}
	add("name", old.Name, next.Name)
	add("mode", old.Mode, next.Mode)
	add("entry_node", old.EntryNode, next.EntryNode)
	add("primary_exit", old.PrimaryExit, next.PrimaryExit)
	if !reflect.DeepEqual(old.BackupExits, next.BackupExits) {
		add("backup_exits", strings.Join(old.BackupExits, ","), strings.Join(next.BackupExits, ","))
	}
	add("failure_mode", old.FailureMode, next.FailureMode)
	add("failures_to_switch", strconv.Itoa(old.FailuresToSwitch), strconv.Itoa(next.FailuresToSwitch))
	add("successes_to_recover", strconv.Itoa(old.SuccessesToRecover), strconv.Itoa(next.SuccessesToRecover))
	add("cooldown_seconds", strconv.Itoa(old.CooldownSeconds), strconv.Itoa(next.CooldownSeconds))
	return changes
}
