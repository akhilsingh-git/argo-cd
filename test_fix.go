package main

import (
	"fmt"
	"os"
	"path/filepath"
	"io/fs"
)

// Simple filesystem implementation for testing
type testFS struct {
	files map[string]bool
}

func (tfs *testFS) Stat(name string) (fs.FileInfo, error) {
	if tfs.files[name] {
		return nil, nil // File exists
	}
	return nil, os.ErrNotExist
}

// isValidKustomizeComponent checks if a directory contains a valid Kustomize component
// by looking for kustomization.yaml, kustomization.yml, or Kustomization files
func isValidKustomizeComponent(root interface{}, componentPath string) bool {
	type statFunc func(string) (fs.FileInfo, error)
	var stat statFunc
	
	if tfs, ok := root.(*testFS); ok {
		stat = tfs.Stat
	} else {
		return false
	}
	
	// Check for kustomization.yaml
	if _, err := stat(filepath.Join(componentPath, "kustomization.yaml")); err == nil {
		return true
	}
	
	// Check for kustomization.yml
	if _, err := stat(filepath.Join(componentPath, "kustomization.yml")); err == nil {
		return true
	}
	
	// Check for Kustomization (capital K)
	if _, err := stat(filepath.Join(componentPath, "Kustomization")); err == nil {
		return true
	}
	
	return false
}

func main() {
	// Test case 1: Directory with kustomization.yaml (should be valid)
	tfs1 := &testFS{
		files: map[string]bool{
			"valid-component/kustomization.yaml": true,
			"valid-component/some-file.txt":     true,
		},
	}
	
	result1 := isValidKustomizeComponent(tfs1, "valid-component")
	fmt.Printf("Test 1 - Directory with kustomization.yaml: %v (expected: true)\n", result1)
	
	// Test case 2: Directory with kustomization.yml (should be valid)
	tfs2 := &testFS{
		files: map[string]bool{
			"valid-component/kustomization.yml": true,
			"valid-component/some-file.txt":     true,
		},
	}
	
	result2 := isValidKustomizeComponent(tfs2, "valid-component")
	fmt.Printf("Test 2 - Directory with kustomization.yml: %v (expected: true)\n", result2)
	
	// Test case 3: Directory with Kustomization (capital K) (should be valid)
	tfs3 := &testFS{
		files: map[string]bool{
			"valid-component/Kustomization":  true,
			"valid-component/some-file.txt": true,
		},
	}
	
	result3 := isValidKustomizeComponent(tfs3, "valid-component")
	fmt.Printf("Test 3 - Directory with Kustomization: %v (expected: true)\n", result3)
	
	// Test case 4: Directory without any kustomization files (should be invalid)
	tfs4 := &testFS{
		files: map[string]bool{
			"invalid-component/some-file.txt":     true,
			"invalid-component/another-file.yaml": true,
		},
	}
	
	result4 := isValidKustomizeComponent(tfs4, "invalid-component")
	fmt.Printf("Test 4 - Directory without kustomization files: %v (expected: false)\n", result4)
	
	// Test case 5: Empty directory (should be invalid)
	tfs5 := &testFS{
		files: map[string]bool{},
	}
	
	result5 := isValidKustomizeComponent(tfs5, "empty-component")
	fmt.Printf("Test 5 - Empty directory: %v (expected: false)\n", result5)
	
	// Verify all tests pass
	if result1 && result2 && result3 && !result4 && !result5 {
		fmt.Println("\n✅ All tests passed! The fix is working correctly.")
	} else {
		fmt.Println("\n❌ Some tests failed. The fix needs adjustment.")
	}
}