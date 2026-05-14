package logo

import "fmt"

func wordmarkSpan(stretch bool, base, minStretch, maxStretch int) int {
	if !stretch {
		return base
	}
	if maxStretch < minStretch {
		minStretch, maxStretch = maxStretch, minStretch
	}
	if maxStretch == minStretch {
		return minStretch
	}
	return cachedRandN(maxStretch-minStretch) + minStretch
}

func LetterHWordmark(stretch bool) string {
	n := wordmarkSpan(stretch, 3, 4, 8)
	bar := repeat("-", n)
	gap := repeat(" ", n)
	return fmt.Sprintf("|%s|\n|%s|\n|%s|", gap, bar, gap)
}

func LetterIWordmark(stretch bool) string {
	n := wordmarkSpan(stretch, 5, 6, 10)
	top := repeat("-", n)
	pad := repeat(" ", max(0, (n-1)/2))
	return fmt.Sprintf("%s\n%s|\n%s", top, pad, top)
}

func LetterAWordmark(stretch bool) string {
	n := wordmarkSpan(stretch, 3, 4, 8)
	bar := repeat("-", n)
	gap := repeat(" ", n)
	return fmt.Sprintf("/%s\\\n|%s|\n|%s|", bar, bar, gap)
}

func LetterGWordmark(stretch bool) string {
	n := wordmarkSpan(stretch, 3, 4, 8)
	bar := repeat("-", n)
	leftGap := repeat(" ", max(0, n-2))
	return fmt.Sprintf("/%s\n| %s|\n\\%s", bar, leftGap+"-", bar)
}

func LetterEWordmark(stretch bool) string {
	n := wordmarkSpan(stretch, 4, 5, 9)
	bar := repeat("-", n)
	return fmt.Sprintf("|%s\n|%s\n|%s", bar, repeat("-", max(2, n-1)), bar)
}

func LetterNWordmark(stretch bool) string {
	n := wordmarkSpan(stretch, 3, 4, 8)
	gapA := repeat(" ", n)
	gapB := repeat(" ", max(0, n-1))
	return fmt.Sprintf("|\\%s|\n| \\%s|\n|%s\\|", gapA, gapB, gapA)
}

func LetterTWordmark(stretch bool) string {
	n := wordmarkSpan(stretch, 5, 6, 10)
	top := repeat("-", n)
	pad := repeat(" ", max(0, (n-1)/2))
	return fmt.Sprintf("%s\n%s|\n%s|", top, pad, pad)
}

func LetterYWordmark(stretch bool) string {
	n := wordmarkSpan(stretch, 3, 4, 8)
	gap := repeat(" ", n)
	pad := repeat(" ", max(0, n/2))
	return fmt.Sprintf("\\%s/\n %s/\n %s|", gap, pad, pad)
}

func LetterPWordmark(stretch bool) string {
	n := wordmarkSpan(stretch, 3, 4, 8)
	bar := repeat("-", n)
	gap := repeat(" ", n)
	return fmt.Sprintf("|%s\\\n|%s/\n|%s", bar, bar, gap)
}

func LetterRWordmark(stretch bool) string {
	n := wordmarkSpan(stretch, 3, 4, 8)
	bar := repeat("-", n)
	gap := repeat(" ", max(0, n-1))
	return fmt.Sprintf("|%s\\\n|%s/\n|%s\\", bar, bar, gap)
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
