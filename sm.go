package csm

import (
	"fmt"
	"reflect"
)

var emptyChan = reflect.ValueOf((chan interface{})(nil))
var emptyVal = reflect.Value{}

//////////////////////////////////////////

type StateMachineState int

const (
	creatingStates StateMachineState = iota
	statesLocked
)

type StateMachine struct {
	states      []*State
	endStates   map[*State]struct{}
	transitions map[*State]map[*State][]func(*StateMachine)
	currState   *State
	prevState   *State
	nextState   *State

	smState StateMachineState

	cases      []reflect.SelectCase
	channelOps []channelOp
}

func New() *StateMachine {
	return &StateMachine{
		transitions: make(map[*State]map[*State][]func(*StateMachine)),
		endStates:   make(map[*State]struct{}),
		currState:   initState,
	}
}
func (sm *StateMachine) AddEndState(states ...*State) {
	for _, state := range states {
		sm.endStates[state] = struct{}{}
	}
}
func (sm *StateMachine) CommitStates() {
	if sm.smState == creatingStates {
		sm.smState = statesLocked
	}
}

func (sm *StateMachine) addTransitionEvent(from, to *State, act func(*StateMachine)) {
	var toMap map[*State][]func(*StateMachine)

	if toMap = sm.transitions[from]; toMap == nil {
		toMap = make(map[*State][]func(*StateMachine))
		sm.transitions[from] = toMap
	}
	toMap[to] = append(toMap[to], act)
}

func (sm *StateMachine) triggerTransitionTo(nextState *State) {
	sm.prevState = sm.currState
	sm.currState = nil
	sm.nextState = nextState
	// execute transition callbacks
	for _, act := range sm.transitions[sm.prevState][sm.nextState] {
		act(sm)
	}
	for _, act := range sm.transitions[AnyState][sm.nextState] {
		act(sm)
	}
	for _, act := range sm.transitions[sm.prevState][AnyState] {
		act(sm)
	}

	// execute channel diff
	for chanOp, _ := range sm.prevState.chanOps {
		if _, found := sm.nextState.chanOps[chanOp]; !found {
			chanOp.unregister(sm)
		}
	}
	for chanOp, _ := range sm.nextState.chanOps {
		// If any callbacks changed chanOp's internal channel - update now
		chanOp.register(sm)
	}

	sm.prevState = nil
	sm.currState = nextState
	sm.nextState = nil
}

func (sm *StateMachine) doSelect() {
	fmt.Println("doSelect: selecting")
	chosen, value, ok := reflect.Select(sm.cases)
	fmt.Println("doSelect: selected")

	switch chanOp := sm.channelOps[chosen].(type) {
	case *ChannelReader:
		chanOp.valueRecv(sm, value, ok)
	case *ChannelWriter:
		chanOp.valueSent(sm)
	}
	fmt.Println("doSelect")
}

func (sm *StateMachine) Run(initState *State) {
	sm.triggerTransitionTo(initState)
	fmt.Println(sm.cases)
	_, isEndState := sm.endStates[sm.currState]
	for !isEndState {
		sm.doSelect()
		_, isEndState = sm.endStates[sm.currState]
	}
}

////////////////////////////////////

type event interface {
	register(*StateMachine)
	unregister(*StateMachine)
}

type channelOp interface {
	event
	SetChannel(ch interface{})
}

type ChannelReader struct {
	ch            reflect.Value
	dest          *interface{}
	isClosed      bool
	recvActions   []action
	closedActions []action
	idx           int
	needsReg      bool
}

func (cr *ChannelReader) register(sm *StateMachine) {
	if cr.needsReg {
		sm.cases[cr.idx].Chan = cr.ch
		cr.needsReg = false
	}
}

func (cr *ChannelReader) unregister(sm *StateMachine) {
	sm.cases[cr.idx].Chan = emptyChan
	cr.needsReg = true
}

func (cr *ChannelReader) valueRecv(sm *StateMachine, recv reflect.Value, recvOK bool) {
	if cr.dest != nil {
		*cr.dest = recv.Interface()
	}
	if !recvOK {
		cr.isClosed = true

		for _, act := range cr.closedActions {
			act.perform(sm)
		}
		cr.unregister(sm)
	} else {
		for _, act := range cr.recvActions {
			act.perform(sm)
		}
	}
}

func (cr *ChannelReader) OnRecv(actions ...action) *ChannelReader {
	cr.recvActions = append(cr.recvActions, actions...)
	return cr
}

func (cr *ChannelReader) OnClosed(actions ...action) *ChannelReader {
	cr.closedActions = append(cr.closedActions, actions...)
	return cr
}
func (cr *ChannelReader) SetChannel(ch interface{}) {
	if ch == nil {
		cr.ch = emptyChan
	} else {
		// Tnx to https://github.com/eapache/channels for reflection magic ahead
		val := reflect.ValueOf(ch)
		valType := val.Type()
		if valType.Kind() != reflect.Chan || valType.ChanDir()&reflect.RecvDir != reflect.RecvDir {
			panic("sm: input to NewChannelReader must be readable channel")
		}
		cr.ch = val
	}
	cr.needsReg = true
}

func NewChannelReader(ch interface{}, dest *interface{}) *ChannelReader {
	cr := &ChannelReader{
		dest:     dest,
		needsReg: true,
	}
	cr.SetChannel(ch)
	return cr
}

////////////////////////////////////

type ChannelWriter struct {
	ch  reflect.Value
	src *interface{}

	sentActions []action
	idx         int
	needsReg    bool
	needsValUpd bool
}

func (cw *ChannelWriter) register(sm *StateMachine) {
	if cw.needsReg {
		sm.cases[cw.idx].Chan = cw.ch
		cw.needsReg = false
	}
	if cw.needsValUpd {
		sm.cases[cw.idx].Send = reflect.ValueOf(*cw.src)
	}
}

func (cw *ChannelWriter) unregister(sm *StateMachine) {
	sm.cases[cw.idx].Chan = emptyChan
	cw.needsReg = true
}

func (cw *ChannelWriter) valueSent(sm *StateMachine) {
	for _, act := range cw.sentActions {
		act.perform(sm)
	}
}

func (cw *ChannelWriter) OnSent(actions ...action) *ChannelWriter {
	cw.sentActions = append(cw.sentActions, actions...)
	return cw
}

func (cw *ChannelWriter) SetChannel(ch interface{}) {
	if ch == nil {
		cw.ch = emptyChan
	} else {
		// Tnx to https://github.com/eapache/channels for reflection magic ahead
		val := reflect.ValueOf(ch)
		valType := val.Type()
		if valType.Kind() != reflect.Chan || valType.ChanDir()&reflect.SendDir != reflect.SendDir {
			panic("sm: input to ChannelWriter.SetChannel must be writable channel")
		}

		cw.ch = val
	}
	cw.needsReg = true
}

func NewChannelWriter(ch interface{}, src *interface{}) *ChannelWriter {
	if src == nil {
		panic("sm: ChannelWriter must have a source")
	}

	cw := &ChannelWriter{
		src:         src,
		needsReg:    true,
		needsValUpd: true,
	}
	cw.SetChannel(ch)
	typeOfChanelItems := cw.ch.Type().Elem()
	if !reflect.TypeOf(*src).AssignableTo(typeOfChanelItems) {
		panic("sm: ChannelWriter: source and channel type missmatch")
	}

	return cw
}

func NewChannelConstWriter(ch interface{}, src interface{}) *ChannelWriter {
	cw := &ChannelWriter{
		src:         &src,
		needsReg:    true,
		needsValUpd: false,
	}
	cw.SetChannel(ch)
	typeOfChanelItems := cw.ch.Type().Elem()
	if !reflect.TypeOf(src).AssignableTo(typeOfChanelItems) {
		panic("sm: ChannelWriter: source and channel type missmatch")
	}

	return cw
}

////////////////////////////////////
type action interface {
	perform(*StateMachine)
}

type transitionAction struct {
	state *State
}

func (ta *transitionAction) perform(sm *StateMachine) {
	fmt.Println("Transition Triggered, cur state", sm.currState.name)
	sm.triggerTransitionTo(ta.state)
	fmt.Println("Transition Completed, cur state", sm.currState.name)

}

func Transition(state *State) action {
	ta := &transitionAction{
		state: state,
	}
	return ta
}

////////////////////////////////////
type doAction struct {
	callback func()
}

func (da *doAction) perform(sm *StateMachine) {
	da.callback()
}

func Do(callback func()) action {
	da := &doAction{
		callback: callback,
	}
	return da
}
