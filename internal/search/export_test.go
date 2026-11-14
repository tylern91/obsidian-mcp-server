package search

// SetCountTokens swaps the token counter and returns a restore func.
func SetCountTokens(fn func(string) int) (restore func()) {
	old := countTokens
	countTokens = fn
	return func() { countTokens = old }
}
