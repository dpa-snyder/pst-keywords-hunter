//go:build !darwin

package scanner

// PrepareMacOutputTree is a no-op outside macOS.
func PrepareMacOutputTree() {}

// CleanupAppleDoubleArtifacts is a no-op outside macOS.
func CleanupAppleDoubleArtifacts(root string) error { return nil }
