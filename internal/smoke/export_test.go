package smoke

// ADR-068 exit-code constants re-exported for the external test
// package (smoke_test): the compliant-binary fake must emulate the
// same codes the step table expects, and referencing the package
// constants instead of hardcoding the numbers keeps the two from
// drifting if the contract ever changes.
const (
	ExitUsageForTest   = exitUsage
	ExitNoInputForTest = exitNoInput
)
