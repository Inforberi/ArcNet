package routes

import (
	"errors"
	"fmt"
)

// State belongs to a simulation, not the live network. Callers must not use
// it as evidence of a working VPN path or as authorization to publish.
type State struct {
	ActiveExit   string `json:"active_exit"`
	Failures     int    `json:"failures"`
	Recoveries   int    `json:"recoveries"`
	LastSwitchAt int64  `json:"last_switch_at"`
}

type Sample struct {
	At int64 `json:"at"`
	// States are verified external observations supplied by the caller:
	// healthy, unhealthy, unknown. This simulator does not perform probes.
	Health map[string]string `json:"health"`
}

type Decision struct {
	At            int64  `json:"at"`
	ActiveExit    string `json:"active_exit,omitempty"`
	EffectivePath string `json:"effective_path"` // direct | cascade | blocked | fail_open
	Switched      bool   `json:"switched"`
	Reason        string `json:"reason"`
	State         State  `json:"state"`
}

func Simulate(p Policy, samples []Sample) ([]Decision, error) {
	if err := Validate(p); err != nil {
		return nil, err
	}
	if len(samples) < 1 || len(samples) > 128 {
		return nil, errors.New("simulation requires 1–128 observations")
	}
	s := State{}
	out := make([]Decision, 0, len(samples))
	var previous int64
	for i, sample := range samples {
		if sample.At <= 0 || (i > 0 && sample.At <= previous) {
			return nil, errors.New("observation timestamps must strictly increase")
		}
		previous = sample.At
		if len(sample.Health) > MaxBackups+2 {
			return nil, errors.New("too many health inputs")
		}
		for id, status := range sample.Health {
			if !nodeIDPattern.MatchString(id) || (status != "healthy" && status != "unhealthy" && status != "unknown") {
				return nil, fmt.Errorf("invalid health sample for %q", id)
			}
		}
		decision := Step(p, s, sample)
		s = decision.State
		out = append(out, decision)
	}
	return out, nil
}

// Step is deterministic and side-effect-free. Unknown health is never healthy.
func Step(p Policy, before State, sample Sample) Decision {
	state := before
	status := func(id string) string {
		v := sample.Health[id]
		if v == "" {
			return "unknown"
		}
		return v
	}
	d := Decision{At: sample.At, EffectivePath: "blocked", Reason: "no verified route"}
	if p.Mode == "direct" {
		state.ActiveExit = p.EntryNode
		if status(p.EntryNode) == "healthy" {
			d.EffectivePath = "direct"
			d.Reason = "verified direct route"
		} else {
			d.Reason = "direct entry has no healthy observation"
		}
		d.ActiveExit = state.ActiveExit
		d.State = state
		return d
	}
	// Discard malformed external state before using it in a decision.
	valid := state.ActiveExit == "" || state.ActiveExit == p.PrimaryExit
	for _, b := range p.BackupExits {
		valid = valid || state.ActiveExit == b
	}
	if !valid || state.Failures < 0 || state.Recoveries < 0 || state.LastSwitchAt > sample.At {
		state = State{}
	}
	canSwitch := state.LastSwitchAt == 0 || sample.At-state.LastSwitchAt >= int64(p.CooldownSeconds)
	switchTo := func(id string) {
		state.ActiveExit = id
		state.Failures = 0
		state.Recoveries = 0
		state.LastSwitchAt = sample.At
		d.Switched = true
	}
	firstHealthyBackup := func(excluding string) string {
		for _, b := range p.BackupExits {
			if b != excluding && status(b) == "healthy" {
				return b
			}
		}
		return ""
	}
	switch state.ActiveExit {
	case "":
		if status(p.PrimaryExit) == "healthy" {
			switchTo(p.PrimaryExit)
			d.Reason = "initial primary verified"
		} else if status(p.PrimaryExit) == "unhealthy" {
			state.Failures++
			if state.Failures >= p.FailuresToSwitch {
				if b := firstHealthyBackup(""); b != "" {
					switchTo(b)
					d.Reason = "initial primary failed; backup selected"
				}
			}
		}
	case p.PrimaryExit:
		if status(p.PrimaryExit) == "healthy" {
			state.Failures = 0
			state.Recoveries = 0
		} else if status(p.PrimaryExit) == "unhealthy" {
			state.Failures++
			if state.Failures >= p.FailuresToSwitch && canSwitch {
				if b := firstHealthyBackup(""); b != "" {
					switchTo(b)
					d.Reason = "primary failed; backup selected"
				}
			}
		}
	default:
		if status(state.ActiveExit) == "healthy" {
			state.Failures = 0
		} else if status(state.ActiveExit) == "unhealthy" {
			state.Failures++
			if state.Failures >= p.FailuresToSwitch && canSwitch {
				if b := firstHealthyBackup(state.ActiveExit); b != "" {
					switchTo(b)
					d.Reason = "backup failed; alternate selected"
				}
			}
		}
		if !d.Switched {
			if status(p.PrimaryExit) == "healthy" {
				state.Recoveries++
			} else {
				state.Recoveries = 0
			}
			if state.Recoveries >= p.SuccessesToRecover && canSwitch {
				switchTo(p.PrimaryExit)
				d.Reason = "primary recovered after hysteresis"
			}
		}
	}
	d.ActiveExit = state.ActiveExit
	if status(p.EntryNode) != "healthy" {
		d.EffectivePath = "blocked"
		d.Reason = "entry has no verified healthy observation"
	} else if d.ActiveExit != "" && status(d.ActiveExit) == "healthy" {
		d.EffectivePath = "cascade"
		if d.Reason == "no verified route" {
			d.Reason = "active exit healthy"
		}
	} else if p.FailureMode == "open" {
		d.EffectivePath = "fail_open"
		d.Reason = "explicit fail-open policy; unsafe direct fallback"
	} else if d.Reason == "no verified route" {
		d.Reason = "no verified healthy exit; fail-closed"
	}
	d.State = state
	return d
}
