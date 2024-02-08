package statemachine

import "github.com/canonical/ubuntu-image/internal/commands"

// RootfsStateMachine embeds ClassicStateMachine
type RootfsStateMachine struct {
	ClassicStateMachine
}

// SetCommonOpts is forcing the RootfsStateMachine to stop at a specific state
// so only the rootfs is generated
func (s *RootfsStateMachine) SetCommonOpts(commonOpts *commands.CommonOpts,
	stateMachineOpts *commands.StateMachineOpts) {
	s.StateMachine.SetCommonOpts(commonOpts, stateMachineOpts)

	s.StateMachine.stateMachineFlags.Thru = preseedClassicImageState.name
}
