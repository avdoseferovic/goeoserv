package protocol

import "math/rand/v2"

var rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))

// GenerateSwapMultipleValue generates a random encryption multiple in range [3, 252].
func GenerateSwapMultipleValue() int {
	return rng.IntN(250) + 3
}
