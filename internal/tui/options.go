package tui

// BrowserOptionsEnabledMsg enables the browser-owned Options page. Native
// terminals retain an explanatory Options screen without image controls.
type BrowserOptionsEnabledMsg struct{}

// OpenBrowserOptionsMsg asks the browser adapter to show its settings page.
// It carries no appearance settings, assets, or entity mutations.
type OpenBrowserOptionsMsg struct{}
