package formula

// ExpTable holds the experience required for each level (0-253).
var ExpTable [254]int

func init() {
	for i := range ExpTable {
		ExpTable[i] = int(float64(i*i*i) * 133.1)
	}
}

// ExpForLevel returns the total exp needed to reach a given level.
func ExpForLevel(level int) int {
	if level < 0 || level >= 254 {
		return 0
	}
	return ExpTable[level]
}

// LevelForExp returns the level for a given exp amount.
func LevelForExp(exp int) int {
	for i := 253; i >= 0; i-- {
		if exp >= ExpTable[i] {
			return i
		}
	}
	return 0
}

// BaseDamage calculates simple melee damage. Placeholder formula.
func BaseDamage(str, minDmg, maxDmg int) int {
	base := str/2 + minDmg
	if maxDmg > minDmg {
		base += maxDmg - minDmg
	}
	if base < 1 {
		base = 1
	}
	return base
}
