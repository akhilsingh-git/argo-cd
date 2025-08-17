package hydrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCleanTargetPath(t *testing.T) {
	h := &Hydrator{}

	t.Run("Clean existing directory with files", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "hydrator-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		targetPath := filepath.Join(tempDir, "target")
		require.NoError(t, os.MkdirAll(targetPath, 0755))

		// Create files in target path
		require.NoError(t, os.WriteFile(filepath.Join(targetPath, "file1.yaml"), []byte("content1"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(targetPath, "file2.yaml"), []byte("content2"), 0644))

		err = h.cleanTargetPath(targetPath)
		require.NoError(t, err)

		entries, err := os.ReadDir(targetPath)
		require.NoError(t, err)
		assert.Empty(t, entries, "Target path should be empty after cleaning")
	})

	t.Run("Clean non-existent directory", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "hydrator-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		targetPath := filepath.Join(tempDir, "nonexistent")
		err = h.cleanTargetPath(targetPath)
		require.NoError(t, err)

		stat, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.True(t, stat.IsDir())
	})
}

func TestValidateHydratorConfig(t *testing.T) {
	h := &Hydrator{}

	t.Run("Valid configuration", func(t *testing.T) {
		app := &v1alpha1.Application{
			Spec: v1alpha1.ApplicationSpec{
				SourceHydrator: &v1alpha1.SourceHydrator{
					SyncSource: &v1alpha1.SyncSource{
						TargetBranch: "temp",
						Path:         "apps/myapp",
					},
				},
			},
		}

		err := h.validateHydratorConfig(app)
		assert.NoError(t, err)
	})

	t.Run("Invalid path - empty", func(t *testing.T) {
		app := &v1alpha1.Application{
			Spec: v1alpha1.ApplicationSpec{
				SourceHydrator: &v1alpha1.SourceHydrator{
					SyncSource: &v1alpha1.SyncSource{
						Path: "",
					},
				},
			},
		}

		err := h.validateHydratorConfig(app)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "syncSource.path cannot be empty")
	})

	t.Run("Invalid path - contains ..", func(t *testing.T) {
		app := &v1alpha1.Application{
			Spec: v1alpha1.ApplicationSpec{
				SourceHydrator: &v1alpha1.SourceHydrator{
					SyncSource: &v1alpha1.SyncSource{
						Path: "apps/../root",
					},
				},
			},
		}

		err := h.validateHydratorConfig(app)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "syncSource.path cannot contain '..' segments")
	})
}

func TestIsPathSafe(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"Valid relative path", "apps/myapp", true},
		{"Valid nested path", "root/subdir/app", true},
		{"Invalid root path", "/", false},
		{"Invalid current dir", ".", false},
		{"Invalid parent traversal", "../other", false},
		{"Invalid absolute path", "/root/app", false},
		{"Valid single directory", "app", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPathSafe(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
