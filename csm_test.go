package csm

import (
	"fmt"
	"testing"
)

func TestChanMachine(t *testing.T) {
	sm := New()
	a := sm.NewState("A")
	b := sm.NewState("B")
	endState := sm.NewState("end")
	sm.AddEndState(endState)
	sm.CommitStates()

	// created all states

	ca := make(chan struct{})
	cb := make(chan struct{})

	// Example of a complex selector
	sm.StateSelector(And(Not(InState(a, b)))).AddEvents()

	a.AddEvents(
		NewChannelReader(ca, nil).
			OnRecv(
				Do(func() {
					fmt.Println("CA read")
				}),
				Transition(b),
			).
			OnClosed(
				Do(func() {
					fmt.Println("CA closed")
				}),
				Transition(endState),
			),
	)
	b.AddEvents(
		NewChannelReader(cb, nil).
			OnRecv(
				Do(func() {
					fmt.Println("CB read")
				}),
				Transition(a),
			).
			OnClosed(
				Do(func() {
					fmt.Println("CB closed")
				}),
				Transition(endState),
			),
	)

	go func() {
		for i := 0; i < 5; i++ {
			select {
			case ca <- struct{}{}:
			case cb <- struct{}{}:
			}
		}
		close(ca)
		close(cb)
	}()

	sm.Run(a)
	// Right now debugging using tests, so failing to view the output
	// I know it's wrong ;)
	t.Fail()
}
