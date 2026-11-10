package text

// ExpandTabs lets external tests state Truncate's string result in terms of the
// styled line it is derived from.
func ExpandTabs(l Line) string { return expandTabs(l) }
