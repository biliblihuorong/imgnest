package service

import "testing"

// UseProductionPasswordCost hashes at the production bcrypt cost until t ends,
// for the tests that assert what a stored hash looks like. It must not be
// combined with t.Parallel.
func UseProductionPasswordCost(t *testing.T) {
	t.Helper()
	previous := passwordCost
	passwordCost = productionPasswordCost
	t.Cleanup(func() { passwordCost = previous })
}
