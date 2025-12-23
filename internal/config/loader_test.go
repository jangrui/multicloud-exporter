package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFile(t *testing.T) {
	// 创建临时目录和文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")
	testContent := []byte("test: content")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name         string
		path         string
		defaultPaths []string
		wantContent  []byte
		wantErr      bool
	}{
		{
			name:         "load from specified path",
			path:         testFile,
			defaultPaths: []string{},
			wantContent:  testContent,
			wantErr:      false,
		},
		{
			name:         "load from default path",
			path:         "",
			defaultPaths: []string{testFile},
			wantContent:  testContent,
			wantErr:      false,
		},
		{
			name:         "file not found in default paths",
			path:         "",
			defaultPaths: []string{"/nonexistent/path/file.yaml"},
			wantContent:  nil,
			wantErr:      true,
		},
		{
			name:         "specified path not found",
			path:         "/nonexistent/path/file.yaml",
			defaultPaths: []string{},
			wantContent:  nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, actualPath, err := LoadConfigFile(tt.path, tt.defaultPaths)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfigFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if string(content) != string(tt.wantContent) {
					t.Errorf("LoadConfigFile() content = %q, want %q", string(content), string(tt.wantContent))
				}
				if actualPath == "" {
					t.Errorf("LoadConfigFile() actualPath should not be empty")
				}
			}
		})
	}
}

func TestLoadConfigFile_MultipleDefaultPaths(t *testing.T) {
	// 创建临时目录和文件
	tmpDir := t.TempDir()
	firstFile := filepath.Join(tmpDir, "first.yaml")
	secondFile := filepath.Join(tmpDir, "second.yaml")

	// 只创建第二个文件
	secondContent := []byte("second: content")
	if err := os.WriteFile(secondFile, secondContent, 0644); err != nil {
		t.Fatalf("Failed to create second file: %v", err)
	}

	// 应该使用第一个存在的文件（第二个）
	content, actualPath, err := LoadConfigFile("", []string{firstFile, secondFile})
	if err != nil {
		t.Errorf("LoadConfigFile() error = %v, want nil", err)
	}
	if string(content) != string(secondContent) {
		t.Errorf("LoadConfigFile() content = %q, want %q", string(content), string(secondContent))
	}
	if actualPath != secondFile {
		t.Errorf("LoadConfigFile() actualPath = %q, want %q", actualPath, secondFile)
	}
}
