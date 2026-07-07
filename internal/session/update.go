package session

import "fmt"

// Update runs a read-modify-write sequence against the state.toml at
// statePath under WithLock: it reads the current state, hands a
// pointer to mutate, and (only when mutate succeeds) writes the
// mutated state back atomically. This makes the "read, mutate, write"
// shape unrepresentable as anything other than a single locked
// operation — callers that previously hand-rolled
// WithLock(ReadState/mutate/WriteState) should migrate to Update
// unless they have side effects that must run mid-critical-section
// (e.g. a network call between the read and the write), in which case
// they stay on WithLock directly.
func Update(statePath string, mutate func(*State) error) error {
	return WithLock(statePath, func() error {
		state, err := ReadState(statePath)
		if err != nil {
			return fmt.Errorf("update: %w", err)
		}
		err = mutate(&state)
		if err != nil {
			return err
		}
		err = WriteState(statePath, state)
		if err != nil {
			return fmt.Errorf("update: %w", err)
		}
		return nil
	})
}
