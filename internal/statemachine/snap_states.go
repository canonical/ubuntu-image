package statemachine

import "fmt"

func (stateMachine *StateMachine) prepareImageSnap() error {
	fmt.Println("Doing image preparation for snap")
	return nil
}
