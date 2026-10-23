package appearance

import (
	"charm.land/lipgloss/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

// Every style occupies one terminal cell on each edge, preserving layout geometry.
func Border(weight data.BorderWeight) lipgloss.Border {
	switch weight {
	case data.BorderHeavy:
		return lipgloss.ThickBorder()
	case data.BorderExtraHeavy:
		return lipgloss.BlockBorder()
	}
	return lipgloss.RoundedBorder()
}
