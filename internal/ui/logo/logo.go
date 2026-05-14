// Package logo renders a HiAgent wordmark in a stylized way.
package logo

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/xiehqing/hiagent-core/internal/ui/styles"
)

// letterform represents a letterform. It can be stretched horizontally by
// a given amount via the boolean argument.
type letterform func(bool) string

const diag = `/`

// Opts are the options for rendering the HiAgent title art.
type Opts struct {
	FieldColor   color.Color // diagonal lines
	TitleColorA  color.Color // left gradient ramp point
	TitleColorB  color.Color // right gradient ramp point
	CharmColor   color.Color // Charm text color
	VersionColor color.Color // version text color
	Width        int         // width of the rendered logo, used for truncation
	Hyper        bool        // whether it is HiAgent or Hyperhiagent

	// When true, stretch a random letterform on each render. Has no effect in
	// compact mode. Mainly for testing. In production you will want to cache
	// the stretched letterform to keep the logo from jittering on resize.
	Unstable bool
}

// Render renders the HiAgent logo. Set the argument to true to render the narrow
// version, intended for use in a sidebar.
//
// The compact argument determines whether it renders compact for the sidebar
// or wider for the main pane.
func Render(base lipgloss.Style, version string, compact bool, o Opts) string {
	charm := "Charm"
	if !o.Hyper {
		charm = " " + charm
	}

	fg := func(c color.Color, s string) string {
		return lipgloss.NewStyle().Foreground(c).Render(s)
	}

	// Title.
	const spacing = 1
	var hyperLetterforms []letterform
	if o.Hyper {
		hyperLetterforms = []letterform{
			LetterHWordmark,
			LetterYWordmark,
			LetterPWordmark,
			LetterEWordmark,
			LetterRWordmark,
		}
	}
	hiagentLetterforms := []letterform{
		LetterHWordmark,
		LetterIWordmark,
		LetterAWordmark,
		LetterGWordmark,
		LetterEWordmark,
		LetterNWordmark,
		LetterTWordmark,
	}
	if o.Hyper && !compact {
		hiagentLetterforms = append(hyperLetterforms, hiagentLetterforms...)
	}

	stretchIndex := -1 // -1 means no stretching.
	if !compact && !o.Unstable {
		// Always stretch the same letterform, which is picked once at random.
		stretchIndex = cachedRandN(len(hiagentLetterforms))
	} else if !compact && o.Unstable {
		// Stretch a random letterform on every render.
		stretchIndex = rand.IntN(len(hiagentLetterforms))
	}
	hiagent := renderWord(spacing, stretchIndex, hiagentLetterforms...)
	if o.Hyper && compact {
		hiagent = renderWord(spacing, stretchIndex, hyperLetterforms...) + "\n" + hiagent
	}
	hiagentWidth := lipgloss.Width(hiagent)
	b := new(strings.Builder)
	for r := range strings.SplitSeq(hiagent, "\n") {
		fmt.Fprintln(b, styles.ApplyForegroundGrad(base, r, o.TitleColorA, o.TitleColorB))
	}
	hiagent = b.String()

	// Charm and version.
	metaRowGap := 1
	maxVersionWidth := hiagentWidth - lipgloss.Width(charm) - metaRowGap
	version = ansi.Truncate(version, maxVersionWidth, "...") // truncate version if too long.
	if o.Hyper && compact {
		version += " "
	}
	gap := max(0, hiagentWidth-lipgloss.Width(charm)-lipgloss.Width(version))
	metaRow := fg(o.CharmColor, charm) + strings.Repeat(" ", gap) + fg(o.VersionColor, version)

	// Join the meta row and big HiAgent title.
	hiagent = strings.TrimSpace(metaRow + "\n" + hiagent)

	// Narrow version. If this is Hyperhiagent, this is also a stacked version.
	if compact {
		field := fg(o.FieldColor, strings.Repeat(diag, hiagentWidth))
		return strings.Join([]string{field, field, hiagent, field, ""}, "\n")
	}

	fieldHeight := lipgloss.Height(hiagent)

	// Left field.
	const leftWidth = 6
	leftFieldRow := fg(o.FieldColor, strings.Repeat(diag, leftWidth))
	leftField := new(strings.Builder)
	for range fieldHeight {
		fmt.Fprintln(leftField, leftFieldRow)
	}

	// Right field.
	rightWidth := max(15, o.Width-hiagentWidth-leftWidth-2) // 2 for the gap.
	const stepDownAt = 0
	rightField := new(strings.Builder)
	for i := range fieldHeight {
		width := rightWidth
		if i >= stepDownAt {
			width = rightWidth - (i - stepDownAt)
		}
		fmt.Fprint(rightField, fg(o.FieldColor, strings.Repeat(diag, width)), "\n")
	}

	// Return the wide version.
	const hGap = " "
	logo := lipgloss.JoinHorizontal(lipgloss.Top, leftField.String(), hGap, hiagent, hGap, rightField.String())
	if o.Width > 0 {
		// Truncate the logo to the specified width.
		lines := strings.Split(logo, "\n")
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, o.Width, "")
		}
		logo = strings.Join(lines, "\n")
	}
	return logo
}

// SmallRender renders a smaller version of the HiAgent logo, suitable for
// smaller windows or sidebar usage.
func SmallRender(t *styles.Styles, width int) string {
	title := t.Logo.SmallCharm.Render("Charm")
	title = fmt.Sprintf("%s %s", title, styles.ApplyBoldForegroundGrad(t.Logo.GradCanvas, "HiAgent", t.Logo.SmallGradFromColor, t.Logo.SmallGradToColor))
	remainingWidth := width - lipgloss.Width(title) - 1 // 1 for the space after "HiAgent"
	if remainingWidth > 0 {
		lines := strings.Repeat("/", remainingWidth)
		title = fmt.Sprintf("%s %s", title, t.Logo.SmallDiagonals.Render(lines))
	}
	return title
}
