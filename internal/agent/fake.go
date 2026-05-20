package agent

import (
	"context"
	"strconv"
)

// Fake is an in-memory Agent implementation for tests.
type Fake struct {
	name      string
	available bool
}

// NewFake returns an available fake provider named name.
func NewFake(name string) *Fake {
	return &Fake{name: name, available: true}
}

// SetAvailable configures whether the fake reports itself available.
func (fake *Fake) SetAvailable(available bool) {
	fake.available = available
}

// Name returns the fake provider name.
func (fake *Fake) Name() string {
	return fake.name
}

// Binary returns the fake provider binary name.
func (fake *Fake) Binary() string {
	return fake.name
}

// IsAvailable returns the configured availability value.
func (fake *Fake) IsAvailable(_ context.Context) bool {
	return fake.available
}

// LaunchCmd returns a deterministic fake launch command.
func (fake *Fake) LaunchCmd(_ LaunchOpts) []string {
	return []string{fake.name, "launch"}
}

// ResumeCmd returns a deterministic fake resume command.
func (fake *Fake) ResumeCmd(_ ResumeOpts) []string {
	return []string{fake.name, "resume"}
}

// PRCmd returns a deterministic fake PR command.
func (fake *Fake) PRCmd(prNumber int, _ LaunchOpts) ([]string, bool) {
	return []string{fake.name, "pr", strconv.Itoa(prNumber)}, true
}

// BodyCmd returns a deterministic fake body-generation command.
func (fake *Fake) BodyCmd(_ BodyOpts) ([]string, bool) {
	return []string{fake.name, "body"}, true
}

// SessionLogPaths returns no fake log paths.
func (*Fake) SessionLogPaths(_, _ string) []string {
	return nil
}
