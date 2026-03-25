package main

func stableListOffset(currentOffset, selectedIndex, visibleRows, totalRows int) int {
	if totalRows <= 0 || visibleRows <= 0 {
		return 0
	}
	maxOffset := maxInt(0, totalRows-visibleRows)
	offset := clampInt(currentOffset, 0, maxOffset)
	selected := clampInt(selectedIndex, 0, totalRows-1)
	if selected < offset {
		offset = selected
	}
	if selected >= offset+visibleRows {
		offset = selected - visibleRows + 1
	}
	return clampInt(offset, 0, maxOffset)
}
