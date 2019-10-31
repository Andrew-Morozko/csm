# CSM
Channel-based State Machine

Currently in proof-of-concept stage, see example in [csm_test.go](https://github.com/Andrew-Morozko/csm/blob/master/csm_test.go).

This is a prototype of a pattern that emerged while developing [orca](https://github.com/Andrew-Morozko/orca).
CSM is a wrapper around a `select` that automates a bunch of useful patterns:
* On channel recv/send transition to some other state of the state machine
* Channels are actively being `select`ed only in some of the states
* One-Shot channels: after successful recv/send stop `select`ing on this channel
* Automatically stop `select`ing on closed channels
* \+ all of the usual state machine functionality: callbacks on transitions, etc.

Right now the project uses reflection (a lot), and generally not the fastest possible implementation.
Theoretically, CSM can be used as a code generator if anybody is brave enough to implement that.

The secondary goal of this project: allow visualization of the resulting state machine. 
That's why all transitions and channel enable/disable operations could be performed as a `csm.action` in response to some `csm.action`: they could be inspected to build a graph of the transitions. The visualization would probably be performed translating the CSM into `dot` and visualizing with [graphviz](https://www.graphviz.org).