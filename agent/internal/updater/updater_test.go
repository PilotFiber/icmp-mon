package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNew(t *testing.T) {
	cfg := Config{
		InstallDir: "/tmp/test",
		BinaryName: "test-agent",
		Logger:     testLogger(),
	}

	u := New(cfg)

	if u.installDir != "/tmp/test" {
		t.Errorf("expected install dir /tmp/test, got %s", u.installDir)
	}
	if u.binaryName != "test-agent" {
		t.Errorf("expected binary name test-agent, got %s", u.binaryName)
	}
	if u.currentPath != "/tmp/test/test-agent" {
		t.Errorf("expected current path /tmp/test/test-agent, got %s", u.currentPath)
	}
}

func TestNew_Defaults(t *testing.T) {
	cfg := Config{}

	u := New(cfg)

	if u.installDir != "/usr/local/bin" {
		t.Errorf("expected default install dir /usr/local/bin, got %s", u.installDir)
	}
	if u.binaryName != "icmpmon-agent" {
		t.Errorf("expected default binary name icmpmon-agent, got %s", u.binaryName)
	}
}

func TestIsUpdating(t *testing.T) {
	u := New(Config{Logger: testLogger()})

	if u.IsUpdating() {
		t.Error("expected IsUpdating to be false initially")
	}

	// Simulate updating state
	u.mu.Lock()
	u.updating = true
	u.mu.Unlock()

	if !u.IsUpdating() {
		t.Error("expected IsUpdating to be true")
	}
}

func TestLastError(t *testing.T) {
	u := New(Config{Logger: testLogger()})

	if u.LastError() != nil {
		t.Error("expected LastError to be nil initially")
	}
}

func TestVerifyChecksum(t *testing.T) {
	// Create temp file with known content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "testfile")
	content := []byte("test content for checksum")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Calculate expected checksum
	h := sha256.Sum256(content)
	expectedChecksum := hex.EncodeToString(h[:])

	u := New(Config{Logger: testLogger()})

	// Test valid checksum
	err := u.verifyChecksum(testFile, expectedChecksum)
	if err != nil {
		t.Errorf("expected checksum to verify, got error: %v", err)
	}

	// Test with sha256: prefix
	err = u.verifyChecksum(testFile, "sha256:"+expectedChecksum)
	if err != nil {
		t.Errorf("expected checksum with prefix to verify, got error: %v", err)
	}

	// Test invalid checksum
	err = u.verifyChecksum(testFile, "invalid_checksum")
	if err == nil {
		t.Error("expected error for invalid checksum")
	}
}

func TestDownloadBinary(t *testing.T) {
	// Create test server
	testContent := []byte("fake binary content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(testContent)
	}))
	defer server.Close()

	u := New(Config{Logger: testLogger()})
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded")

	err := u.downloadBinary(context.Background(), server.URL, destPath)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	// Verify content
	downloaded, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(downloaded) != string(testContent) {
		t.Errorf("expected %q, got %q", testContent, downloaded)
	}
}

func TestDownloadBinary_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	u := New(Config{Logger: testLogger()})
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded")

	err := u.downloadBinary(context.Background(), server.URL, destPath)
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "src")
	dstPath := filepath.Join(tmpDir, "dst")

	content := []byte("test content")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("failed to write src file: %v", err)
	}

	u := New(Config{Logger: testLogger()})
	err := u.copyFile(srcPath, dstPath)
	if err != nil {
		t.Fatalf("copy failed: %v", err)
	}

	// Verify content
	copied, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}
	if string(copied) != string(content) {
		t.Errorf("expected %q, got %q", content, copied)
	}
}

func TestUpdate_AlreadyUpdating(t *testing.T) {
	u := New(Config{Logger: testLogger()})
	u.mu.Lock()
	u.updating = true
	u.mu.Unlock()

	err := u.Update(context.Background(), &types.UpdateInfo{Version: "1.0.0"})
	if err == nil {
		t.Error("expected error when already updating")
	}
}

func TestRollback_VersionNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	u := New(Config{
		InstallDir: tmpDir,
		BinaryName: "agent",
		Logger:     testLogger(),
	})

	err := u.Rollback("1.0.0")
	if err == nil {
		t.Error("expected error for nonexistent version")
	}
}

func TestAtomicUpdate_SymlinkSwap(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake binary
	binaryContent := []byte("fake binary")
	newBinaryPath := filepath.Join(tmpDir, "new-binary")
	if err := os.WriteFile(newBinaryPath, binaryContent, 0755); err != nil {
		t.Fatalf("failed to write binary: %v", err)
	}

	u := New(Config{
		InstallDir: tmpDir,
		BinaryName: "agent",
		Logger:     testLogger(),
	})

	err := u.atomicUpdate(newBinaryPath, "1.0.0")
	if err != nil {
		t.Fatalf("atomic update failed: %v", err)
	}

	// Verify versioned binary exists
	versionedPath := filepath.Join(tmpDir, "agent-v1.0.0")
	if _, err := os.Stat(versionedPath); os.IsNotExist(err) {
		t.Error("versioned binary not created")
	}

	// Verify symlink exists and points to versioned binary
	linkPath := filepath.Join(tmpDir, "agent")
	linkTarget, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	if linkTarget != versionedPath {
		t.Errorf("symlink points to %s, expected %s", linkTarget, versionedPath)
	}
}

func TestAtomicUpdate_ExistingSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial versioned binary and symlink
	oldVersionPath := filepath.Join(tmpDir, "agent-v0.9.0")
	if err := os.WriteFile(oldVersionPath, []byte("old"), 0755); err != nil {
		t.Fatalf("failed to write old binary: %v", err)
	}
	agentPath := filepath.Join(tmpDir, "agent")
	if err := os.Symlink(oldVersionPath, agentPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Create new binary
	newBinaryPath := filepath.Join(tmpDir, "new-binary")
	if err := os.WriteFile(newBinaryPath, []byte("new"), 0755); err != nil {
		t.Fatalf("failed to write new binary: %v", err)
	}

	u := New(Config{
		InstallDir: tmpDir,
		BinaryName: "agent",
		Logger:     testLogger(),
	})

	err := u.atomicUpdate(newBinaryPath, "1.0.0")
	if err != nil {
		t.Fatalf("atomic update failed: %v", err)
	}

	// Verify symlink now points to new version
	linkTarget, err := os.Readlink(agentPath)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	expectedTarget := filepath.Join(tmpDir, "agent-v1.0.0")
	if linkTarget != expectedTarget {
		t.Errorf("symlink points to %s, expected %s", linkTarget, expectedTarget)
	}

	// Verify old version still exists (for rollback)
	if _, err := os.Stat(oldVersionPath); os.IsNotExist(err) {
		t.Error("old version should be preserved for rollback")
	}
}

func TestRollback_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create versioned binaries
	v1Path := filepath.Join(tmpDir, "agent-v1.0.0")
	v2Path := filepath.Join(tmpDir, "agent-v2.0.0")
	if err := os.WriteFile(v1Path, []byte("v1"), 0755); err != nil {
		t.Fatalf("failed to write v1: %v", err)
	}
	if err := os.WriteFile(v2Path, []byte("v2"), 0755); err != nil {
		t.Fatalf("failed to write v2: %v", err)
	}

	// Create symlink to v2
	agentPath := filepath.Join(tmpDir, "agent")
	if err := os.Symlink(v2Path, agentPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	u := New(Config{
		InstallDir: tmpDir,
		BinaryName: "agent",
		Logger:     testLogger(),
	})

	// Rollback to v1
	err := u.Rollback("1.0.0")
	if err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	// Verify symlink now points to v1
	linkTarget, err := os.Readlink(agentPath)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	if linkTarget != v1Path {
		t.Errorf("symlink points to %s, expected %s", linkTarget, v1Path)
	}
}
