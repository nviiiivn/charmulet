// Package layout is the minimal CHARMULET layout engine: block joins and
// grids on top of lipgloss. Component registry + page model land in P4.
package layout

import "charm.land/lipgloss/v2"

// H joins blocks side by side, top-aligned.
func H(blocks ...string) string { return lipgloss.JoinHorizontal(lipgloss.Top, blocks...) }

// V joins blocks vertically, left-aligned.
func V(blocks ...string) string { return lipgloss.JoinVertical(lipgloss.Left, blocks...) }

// Grid wraps blocks into rows of cols columns.
func Grid(cols int, blocks ...string) string {
	if cols < 1 {
		cols = 1
	}
	var rows []string
	for i := 0; i < len(blocks); i += cols {
		end := min(i+cols, len(blocks))
		rows = append(rows, H(blocks[i:end]...))
	}
	return V(rows...)
}
