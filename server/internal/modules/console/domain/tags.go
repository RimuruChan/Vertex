package domain

// TransformTags retains the first position of each resulting label, including
// when renaming onto an existing label. Published snapshots are not inputs here.
func TransformTags(tags []string, oldName, newName string, remove bool) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if tag == oldName {
			if remove {
				continue
			}
			tag = newName
		}
		if !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}
	return result
}
