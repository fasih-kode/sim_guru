package main

import "path/filepath"

func photoDirectoriesOverlap(first, second string) bool {
	if first == "" || second == "" {
		return false
	}
	first, second = filepath.Clean(first), filepath.Clean(second)
	within := func(path, root string) bool {
		relative, err := filepath.Rel(root, path)
		return err == nil && relative != ".." && !filepath.IsAbs(relative) && (relative == "." || len(relative) > 3 && relative[:3] != ".."+string(filepath.Separator))
	}
	return within(first, second) || within(second, first)
}
