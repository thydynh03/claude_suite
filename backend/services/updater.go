package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"claude_suite/backend/version"
)

type UpdateInfo struct {
	HasUpdate   bool   `json:"has_update"`
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	Body        string `json:"body"`
}

type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type GitHubRelease struct {
	TagName string         `json:"tag_name"`
	Body    string         `json:"body"`
	Assets  []ReleaseAsset `json:"assets"`
}

type UpdaterService struct{}

func NewUpdaterService() *UpdaterService {
	return &UpdaterService{}
}

func (u *UpdaterService) CheckForUpdates() (*UpdateInfo, error) {
	apiUrl := "https://api.github.com/repos/thydynh03/claude_suite/releases/latest"
	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ClaudeSuite-App")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}

	if rel.TagName != "" && rel.TagName != version.CurrentVersion {
		var downloadUrl string
		for _, asset := range rel.Assets {
			if filepath.Ext(asset.Name) == ".exe" {
				downloadUrl = asset.BrowserDownloadURL
				break
			}
		}
		return &UpdateInfo{
			HasUpdate:   true,
			Version:     rel.TagName,
			DownloadURL: downloadUrl,
			Body:        rel.Body,
		}, nil
	}

	return &UpdateInfo{HasUpdate: false, Version: version.CurrentVersion}, nil
}

func (u *UpdaterService) DownloadAndInstall(downloadUrl string, progressCb func(downloaded, total int64)) error {
	if downloadUrl == "" {
		return fmt.Errorf("empty download url")
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exeDir := filepath.Dir(exePath)
	newExePath := filepath.Join(exeDir, "ClaudeSuite_new.exe")

	resp, err := http.Get(downloadUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(newExePath)
	if err != nil {
		return err
	}
	defer out.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, wErr := out.Write(buf[:n])
			if wErr != nil {
				return wErr
			}
			downloaded += int64(n)
			if progressCb != nil {
				progressCb(downloaded, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	out.Close()

	batPath := filepath.Join(exeDir, "updater.bat")
	// Wait 3s for the old process to fully exit, then move new exe over old one,
	// then start the updated app. /b flag keeps the bat's console hidden.
	batContent := fmt.Sprintf(`@echo off
set "NEW_EXE=%s"
set "OLD_EXE=%s"

rem Wait for the running process to fully exit
timeout /t 3 /nobreak > NUL

:RETRY
move /y "%%NEW_EXE%%" "%%OLD_EXE%%" > NUL 2>&1
if errorlevel 1 (
    timeout /t 1 /nobreak > NUL
    goto RETRY
)

rem Relaunch the updated app
start "" "%%OLD_EXE%%"
del "%%~f0"
`, newExePath, exePath)

	if err := os.WriteFile(batPath, []byte(batContent), 0755); err != nil {
		return err
	}

	// Start the bat as a completely detached process so it survives this process exiting
	cmd := exec.Command("cmd.exe", "/c", "start", "/b", "", batPath)
	cmd.Dir = exeDir
	if err := cmd.Start(); err != nil {
		return err
	}

	os.Exit(0)
	return nil
}

