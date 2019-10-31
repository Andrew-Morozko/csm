package csm

import "reflect"

type State struct {
	sm      *StateMachine
	name    string
	chanOps map[channelOp]struct{}
}

var AnyState *State = &State{name: "ANYSTATE"}
var initState *State = &State{name: "INITSTATE"}

func (s *State) AddEvents(events ...event) {
	for _, evt := range events {
		if chanOP, ok := evt.(channelOp); ok {
			switch val := chanOP.(type) {
			case *ChannelReader:
				val.idx = len(s.sm.cases)
				s.sm.cases = append(s.sm.cases, reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: emptyChan,
					Send: emptyVal,
				})

				s.sm.channelOps = append(s.sm.channelOps, val)
			case *ChannelWriter:
				val.idx = len(s.sm.cases)
				s.sm.cases = append(s.sm.cases, reflect.SelectCase{
					Dir:  reflect.SelectSend,
					Chan: emptyChan,
					Send: emptyVal,
				})
				if val.needsValUpd == false {
					// const writer, set value right now
					s.sm.cases[val.idx].Send = reflect.ValueOf(*val.src)
				}
			}
			s.chanOps[chanOP] = struct{}{}
		} else {
			//?
			s.sm.addTransitionEvent(AnyState, s, evt.register)
			s.sm.addTransitionEvent(s, AnyState, evt.unregister)
		}
	}
}

func (sm *StateMachine) NewState(name string) *State {
	if sm.smState != creatingStates {
		panic("StateMachine: attempted to add state after commiting states")
	}
	ns := &State{
		sm:      sm,
		name:    name,
		chanOps: make(map[channelOp]struct{}),
	}
	sm.states = append(sm.states, ns)
	return ns
}

type StateList []*State

func (sl StateList) AddEvents(events ...event) {
	for _, state := range sl {
		state.AddEvents(events...)
	}
}

func (sm *StateMachine) StateSelector(condition StateSelector) StateList {
	var res []*State
	// Yep, this is bad. A lot of wasted time and space
	// TODO: Condition check should return StateList, not bool per every state...
	for _, state := range sm.states {
		if condition.Check(state) {
			res = append(res, state)
		}
	}
	return res
}

type StateSelector interface {
	Check(state *State) bool
}

//////////////////////////////////// Bool condition operations

type andSelector struct {
	conditions []StateSelector
}

func (ss *andSelector) Check(state *State) bool {
	for _, cond := range ss.conditions {
		if !cond.Check(state) {
			return false
		}
	}
	return true
}

func And(conditions ...StateSelector) StateSelector {
	return &andSelector{
		conditions: conditions,
	}
}

func Any() StateSelector {
	return And()
}

type orSelector struct {
	conditions []StateSelector
}

func (ss *orSelector) Check(state *State) bool {
	for _, cond := range ss.conditions {
		if cond.Check(state) {
			return true
		}
	}
	return false
}

func Or(conditions ...StateSelector) StateSelector {
	return &orSelector{
		conditions: conditions,
	}
}

type notSelector struct {
	cond StateSelector
}

func (ss *notSelector) Check(state *State) bool {
	return !ss.cond.Check(state)
}
func Not(condition StateSelector) StateSelector {
	return &notSelector{
		cond: condition,
	}
}

/////////
type stateListSelector struct {
	states map[*State]struct{}
}

func (ss *stateListSelector) Check(state *State) bool {
	_, found := ss.states[state]
	return found
}

func InState(states ...*State) StateSelector {
	ss := &stateListSelector{
		states: make(map[*State]struct{}),
	}
	for _, state := range states {
		ss.states[state] = struct{}{}
	}
	return ss
}

// Todo: more interesting operations like "is state reachable"

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// func (sm *StateMachine) TransitionSelector(condition StateSelector) StateList {
// 	var res []*State
// 	for _, state := range sm.states {
// 		if condition.Check(state) {
// 			res = append(res, state)
// 		}
// 	}
// 	return res
// }

// type transitionSelector struct {
// 	from StateSelector
// 	to   StateSelector
// }

// func (ss *transitionSelector) Check(sm *StateMachine, lookAt smStateKind) bool {
// 	return ss.from.Check(sm, prevState) && ss.to.Check(sm, nextState)
// }
// func OnTransition(from StateSelector, to StateSelector) StateSelector {
// 	return &transitionSelector{
// 		from: from,
// 		to:   to,
// 	}
// }
