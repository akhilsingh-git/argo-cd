package hydrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/util/git"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// hydrateToTargetBranch - Fixed version that only modifies specific paths
func (h *Hydrator) hydrateToTargetBranch(ctx context.Context, app *v1alpha1.Application, dryCommit string, hydratedManifests []*unstructured.Unstructured) error {
	logCtx := log.WithFields(log.Fields{
		"app":       app.Name,
		"branch":    app.Spec.SourceHydrator.SyncSource.TargetBranch,
		"path":      app.Spec.SourceHydrator.SyncSource.Path,
		"dryCommit": dryCommit,
	})

	// Validate configuration before proceeding
	if err := h.validateHydratorConfig(app); err != nil {
		return fmt.Errorf("invalid hydrator configuration: %w", err)
	}

	// Clone the repository to a temporary directory
	repoPath, err := h.cloneRepository(ctx, app)
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	defer os.RemoveAll(repoPath)

	// Ensure we're on the target branch
	targetBranch := app.Spec.SourceHydrator.SyncSource.TargetBranch
	if err := h.checkoutOrCreateBranch(repoPath, targetBranch); err != nil {
		return fmt.Errorf("failed to checkout target branch: %w", err)
	}

	// CRITICAL FIX: Only clean the specific target path, not the entire working directory
	targetPath := filepath.Join(repoPath, app.Spec.SourceHydrator.SyncSource.Path)
	if err := h.cleanTargetPath(targetPath); err != nil {
		return fmt.Errorf("failed to clean target path: %w", err)
	}

	// Write hydrated manifests to the specific target path only
	if err := h.writeManifestsToPath(targetPath, hydratedManifests); err != nil {
		return fmt.Errorf("failed to write manifests: %w", err)
	}

	// Add only the specific path to git staging
	if err := h.addPathToGit(repoPath, app.Spec.SourceHydrator.SyncSource.Path); err != nil {
		return fmt.Errorf("failed to add path to git: %w", err)
	}

	// Commit changes with proper metadata
	commitMessage := h.generateCommitMessage(app, dryCommit)
	if err := h.commitChanges(repoPath, commitMessage, dryCommit); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	// Push to remote
	if err := h.pushBranch(repoPath, targetBranch); err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}

	logCtx.Info("Successfully hydrated manifests to target branch")
	return nil
}

// cleanTargetPath - NEW: Only clean the specific target path, not entire repository
func (h *Hydrator) cleanTargetPath(targetPath string) error {
	// Check if the target path exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		// Path doesn't exist, create it
		return os.MkdirAll(targetPath, 0755)
	} else if err != nil {
		return fmt.Errorf("failed to stat target path: %w", err)
	}

	// Remove contents of target path only, not parent directories or sibling paths
	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return fmt.Errorf("failed to read target path: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(targetPath, entry.Name())
		if err := os.RemoveAll(entryPath); err != nil {
			return fmt.Errorf("failed to remove entry %s: %w", entryPath, err)
		}
	}

	return nil
}

// addPathToGit - NEW: Add only specific path to git, not entire working directory
func (h *Hydrator) addPathToGit(repoPath, relativePath string) error {
	gitCmd := git.NewCommand("add")
	
	// Clean the relative path to avoid issues
	cleanPath := filepath.Clean(relativePath)
	if cleanPath == "." || cleanPath == "/" {
		return fmt.Errorf("invalid path: cannot add root directory")
	}

	gitCmd.AddArguments(cleanPath)
	
	// Execute git add command
	_, err := gitCmd.RunInDir(repoPath)
	if err != nil {
		return fmt.Errorf("git add failed for path %s: %w", cleanPath, err)
	}

	return nil
}

// validateHydratorConfig - NEW: Validation to prevent path conflicts
func (h *Hydrator) validateHydratorConfig(app *v1alpha1.Application) error {
	if app.Spec.SourceHydrator == nil {
		return fmt.Errorf("sourceHydrator configuration is required")
	}

	syncSource := app.Spec.SourceHydrator.SyncSource
	if syncSource == nil {
		return fmt.Errorf("syncSource configuration is required")
	}

	// Validate path is not root or empty
	path := strings.TrimSpace(syncSource.Path)
	if path == "" || path == "." || path == "/" {
		return fmt.Errorf("syncSource.path cannot be empty, '.', or '/'")
	}

	// Ensure path doesn't contain dangerous patterns
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("syncSource.path cannot contain '..' segments")
	}

	return nil
}

// checkoutOrCreateBranch - Improved branch handling
func (h *Hydrator) checkoutOrCreateBranch(repoPath, branchName string) error {
	// First try to checkout existing branch
	checkoutCmd := git.NewCommand("checkout", branchName)
	if _, err := checkoutCmd.RunInDir(repoPath); err == nil {
		return nil // Successfully checked out existing branch
	}

	// Branch doesn't exist locally, check if it exists remotely
	remoteBranchCmd := git.NewCommand("ls-remote", "--heads", "origin", branchName)
	output, err := remoteBranchCmd.RunInDir(repoPath)
	if err != nil {
		return fmt.Errorf("failed to check remote branches: %w", err)
	}

	if strings.TrimSpace(output) != "" {
		// Remote branch exists, checkout and track it
		trackCmd := git.NewCommand("checkout", "-b", branchName, fmt.Sprintf("origin/%s", branchName))
		if _, err := trackCmd.RunInDir(repoPath); err != nil {
			return fmt.Errorf("failed to checkout remote branch: %w", err)
		}
	} else {
		// Create new branch
		createCmd := git.NewCommand("checkout", "-b", branchName)
		if _, err := createCmd.RunInDir(repoPath); err != nil {
			return fmt.Errorf("failed to create new branch: %w", err)
		}
	}

	return nil
}

// writeManifestsToPath - Improved manifest writing with path validation
func (h *Hydrator) writeManifestsToPath(targetPath string, manifests []*unstructured.Unstructured) error {
	// Ensure target directory exists
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Write manifest file
	manifestPath := filepath.Join(targetPath, "manifest.yaml")
	
	var manifestContent strings.Builder
	for i, manifest := range manifests {
		if i > 0 {
			manifestContent.WriteString("\n---\n")
		}
		
		yamlData, err := yaml.Marshal(manifest.Object)
		if err != nil {
			return fmt.Errorf("failed to marshal manifest: %w", err)
		}
		manifestContent.Write(yamlData)
	}

	if err := os.WriteFile(manifestPath, []byte(manifestContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to write manifest file: %w", err)
	}

	return nil
}

// commitChanges - Improved commit with proper git trailers
func (h *Hydrator) commitChanges(repoPath, message, dryCommit string) error {
	// Check if there are any changes to commit
	statusCmd := git.NewCommand("status", "--porcelain")
	output, err := statusCmd.RunInDir(repoPath)
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	if strings.TrimSpace(output) == "" {
		log.Debug("No changes to commit")
		return nil // No changes to commit
	}

	// Commit with proper git trailers for traceability
	commitCmd := git.NewCommand("commit", "-m", message)
	
	// Add git trailers for traceability
	if dryCommit != "" {
		trailerMessage := fmt.Sprintf("%s\n\nHydrated-From: %s", message, dryCommit)
		commitCmd = git.NewCommand("commit", "-m", trailerMessage)
	}

	if _, err := commitCmd.RunInDir(repoPath); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	return nil
}

// generateCommitMessage - Improved commit message generation
func (h *Hydrator) generateCommitMessage(app *v1alpha1.Application, dryCommit string) string {
	return fmt.Sprintf("chore: hydrate manifests for %s\n\nPath: %s\nApplication: %s\nNamespace: %s",
		app.Name,
		app.Spec.SourceHydrator.SyncSource.Path,
		app.Name,
		app.Namespace,
	)
}

// isPathSafe - Utility to validate path safety
func isPathSafe(path string) bool {
	cleanPath := filepath.Clean(path)
	return !strings.Contains(cleanPath, "..") && 
	       cleanPath != "." && 
	       cleanPath != "/" && 
	       !strings.HasPrefix(cleanPath, "/")
}
