package appearance

import (
	"strings"

	"charm.land/bubbles/v2/list"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
)

// Subject lists share presentation geometry; the owning models retain all
// filtering, selection, creation, and persistence behavior.
func SubjectListReservedRows(height int) int {
	if height < 18 {
		return 10
	}
	return 12
}

func ConfigureSubjectList(model *list.Model) {
	model.SetShowTitle(false)
	model.SetShowFilter(false)
	model.SetShowStatusBar(false)
	model.SetShowHelp(false)
	model.FilterInput.Prompt = "/ "
	model.FilterInput.Placeholder = "Search Subjects"
	model.FilterInput.Blur()
}

func SubjectListView(model list.Model, width, height int, weight data.BorderWeight, contextTitle, contextDetail, status, actions, keys string) string {
	line := func(text string) string { return "  " + ansi.Truncate(text, max(1, width-4), "…") }
	divider := strings.Repeat(Border(weight).Top, max(1, width))
	heading := line(model.Styles.Title.Render(model.Title))
	search := model.FilterInput.View()
	if model.FilterState() == list.Unfiltered {
		search = "/ Search Subjects"
	} else if !model.SettingFilter() {
		search = "/ " + model.FilterValue()
	}
	header := []string{heading, "", line(ansi.Truncate(search, max(1, width-4), "…")), divider, line(contextTitle), line(contextDetail), ""}
	if height < 18 {
		header = []string{heading, line(ansi.Truncate(search, max(1, width-4), "…")), line(contextTitle), line(contextDetail), divider}
	}
	sections := append(header, model.View(), divider, line(status), line(actions), line(keys), "")
	return strings.Join(sections, "\n")

}

func SubjectListDelegate(checkboxes bool) list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	if !checkboxes {
		delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.PaddingLeft(6)
		delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.PaddingLeft(5)
		delegate.Styles.DimmedTitle = delegate.Styles.DimmedTitle.PaddingLeft(6)
	}
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.PaddingLeft(6)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.PaddingLeft(5)
	delegate.Styles.DimmedDesc = delegate.Styles.DimmedDesc.PaddingLeft(6)
	return delegate
}

func ResizeSubjectList(model *list.Model, width, height int) {
	model.SetSize(width, max(1, height-SubjectListReservedRows(height)))
	model.FilterInput.SetWidth(max(1, width-6))
}
