package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/parse"
	"github.com/wimmme/shellylanman/internal/service"
)

// plan is what a control tool will do: commands on one module of one device.
// The tools run it at once; scenes store it (sceneActions) and run it later.
type plan struct {
	device  string
	channel *int
	kinds   []string
	cmds    []service.Command
}

// planners turn the arguments of a control tool into a plan; scenes accept
// the same tools and arguments.
var planners = map[string]func(json.RawMessage) (plan, error){
	"shelly_switch":     planSwitch,
	"shelly_light":      planLight,
	"shelly_cover":      planCover,
	"shelly_thermostat": planThermostat,
}

// maxTimer: Shelly's flip-back timers take at most a day here.
const maxTimer = 86400

func planSwitch(args json.RawMessage) (plan, error) {
	var a struct {
		Device, Action string
		Channel        *int
		TimerS         *float64 `json:"timer_s"`
	}
	if err := decode(args, &a); err != nil {
		return plan{}, err
	}
	if !slices.Contains([]string{service.ActionOn, service.ActionOff, service.ActionToggle}, a.Action) {
		return plan{}, fmt.Errorf("%w: action must be on, off or toggle", errUser)
	}
	if a.TimerS != nil && (*a.TimerS <= 0 || *a.TimerS > maxTimer) {
		return plan{}, fmt.Errorf("%w: timer_s must be 1–%d seconds", errUser, maxTimer)
	}
	return plan{device: a.Device, channel: a.Channel, kinds: []string{parse.KindRelay},
		cmds: []service.Command{{Action: a.Action, Timer: a.TimerS}}}, nil
}

func planLight(args json.RawMessage) (plan, error) {
	var a struct {
		Device       string
		Channel      *int
		On           *bool
		Brightness   *float64
		Gain         *float64
		RGB          []int
		White        *int
		TemperatureK *float64 `json:"temperature_k"`
		TransitionS  *float64 `json:"transition_s"`
	}
	if err := decode(args, &a); err != nil {
		return plan{}, err
	}
	if a.TransitionS != nil && (*a.TransitionS < 0 || *a.TransitionS > 60) {
		return plan{}, fmt.Errorf("%w: transition_s must be 0–60 seconds", errUser)
	}
	var cmds []service.Command
	if a.Brightness != nil {
		cmds = append(cmds, service.Command{Action: service.ActionBrightness, Value: a.Brightness})
	}
	if a.Gain != nil {
		cmds = append(cmds, service.Command{Action: service.ActionGain, Value: a.Gain})
	}
	if a.RGB != nil {
		if len(a.RGB) != 3 {
			return plan{}, fmt.Errorf("%w: rgb needs three values", errUser)
		}
		cmds = append(cmds, service.Command{Action: service.ActionColor, RGB: a.RGB, White: a.White})
	} else if a.White != nil {
		v := float64(*a.White)
		cmds = append(cmds, service.Command{Action: service.ActionWhite, Value: &v})
	}
	if a.TemperatureK != nil {
		cmds = append(cmds, service.Command{Action: service.ActionTemp, Value: a.TemperatureK})
	}
	if a.On != nil {
		act := service.ActionOff
		if *a.On {
			act = service.ActionOn
		}
		cmds = append(cmds, service.Command{Action: act})
	}
	if len(cmds) == 0 {
		return plan{}, fmt.Errorf("%w: nothing to change", errUser)
	}
	for i := range cmds {
		cmds[i].Transition = a.TransitionS
	}
	return plan{device: a.Device, channel: a.Channel, kinds: lightKinds, cmds: cmds}, nil
}

func planCover(args json.RawMessage) (plan, error) {
	var a struct {
		Device, Action string
		Channel        *int
		Position       *float64
	}
	if err := decode(args, &a); err != nil {
		return plan{}, err
	}
	if !slices.Contains([]string{service.ActionOpen, service.ActionClose, service.ActionStop, service.ActionPosition}, a.Action) {
		return plan{}, fmt.Errorf("%w: action must be open, close, stop or position", errUser)
	}
	if a.Action == service.ActionPosition && a.Position == nil {
		return plan{}, fmt.Errorf("%w: position is required", errUser)
	}
	return plan{device: a.Device, channel: a.Channel, kinds: []string{parse.KindCover},
		cmds: []service.Command{{Action: a.Action, Value: a.Position}}}, nil
}

func planThermostat(args json.RawMessage) (plan, error) {
	var a struct {
		Device  string
		Channel *int
		TargetC *float64 `json:"target_c"`
		Enabled *bool
	}
	if err := decode(args, &a); err != nil {
		return plan{}, err
	}
	if a.TargetC == nil && a.Enabled == nil {
		return plan{}, fmt.Errorf("%w: nothing to change", errUser)
	}
	var cmds []service.Command
	if a.Enabled != nil {
		v := 0.0
		if *a.Enabled {
			v = 1
		}
		cmds = append(cmds, service.Command{Action: service.ActionEnable, Value: &v})
	}
	if a.TargetC != nil {
		cmds = append(cmds, service.Command{Action: service.ActionTarget, Value: a.TargetC})
	}
	return plan{device: a.Device, channel: a.Channel, kinds: []string{parse.KindThermostat}, cmds: cmds}, nil
}

// target resolves the plan's device and module.
func (p plan) target(s Service) (model.Device, parse.Module, error) {
	d, err := resolve(s, p.device)
	if err != nil {
		return d, parse.Module{}, err
	}
	m, err := module(d, p.channel, p.kinds...)
	return d, m, err
}

// runPlan sends the commands in order and returns the module as it is afterwards.
func runPlan(ctx context.Context, s Service, p plan) (any, error) {
	d, m, err := p.target(s)
	if err != nil {
		return nil, err
	}
	for _, c := range p.cmds {
		c.Key = m.Key
		if err := s.Command(ctx, d.ID, c); err != nil {
			return nil, err
		}
	}
	after := d
	if again, err := resolve(s, d.ID); err == nil {
		after = again
	}
	for _, x := range after.Modules {
		if x.Key == m.Key {
			return map[string]any{"device": d.ID, "name": label(d), "channel": x}, nil
		}
	}
	return map[string]any{"device": d.ID, "name": label(d), "done": len(p.cmds)}, nil
}

// planTool: the run function of a control tool.
func planTool(name string) func(context.Context, Service, json.RawMessage) (any, error) {
	return func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
		p, err := planners[name](args)
		if err != nil {
			return nil, err
		}
		return runPlan(ctx, s, p)
	}
}
