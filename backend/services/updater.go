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

type GitHubTag struct {
	Name string `json:"name"`
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
	if err == nil && resp.StatusCode == http.StatusOK {
		var rel GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&rel); err == nil && rel.TagName != "" {
			resp.Body.Close()
			if rel.TagName != version.CurrentVersion {
				var downloadUrl string
				for _, asset := range rel.Assets {
					if filepath.Ext(asset.Name) == ".exe" {
						downloadUrl = asset.BrowserDownloadURL
						break
					}
				}
				if downloadUrl == "" {
					downloadUrl = "https://github.com/thydynh03/claude_suite/archive/refs/tags/" + rel.TagName + ".zip"
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
		resp.Body.Close()
	}

	// Fallback: Check tags API if releases API is empty
	tagsUrl := "https://api.github.com/repos/thydynh03/claude_suite/tags"
	tReq, err := http.NewRequest("GET", tagsUrl, nil)
	if err == nil {
		tReq.Header.Set("User-Agent", "ClaudeSuite-App")
		tResp, err := http.DefaultClient.Do(tReq)
		if err == nil && tResp.StatusCode == http.StatusOK {
			var tags []GitHubTag
			if err := json.NewDecoder(tResp.Body).Decode(&tags); err == nil && len(tags) > 0 {
				tResp.Body.Close()
				latestTag := tags[0].Name
				if latestTag != version.CurrentVersion {
					return &UpdateInfo{
						HasUpdate:   true,
						Version:     latestTag,
						DownloadURL: "https://github.com/thydynh03/claude_suite/archive/refs/tags/" + latestTag + ".zip",
						Body:        "Bản phát hành mới " + latestTag + " trên GitHub Repository.",
					}, nil
				}
			}
			tResp.Body.Close()
		}
	}

	return &UpdateInfo{HasUpdate: false, Version: version.CurrentVersion}, nil
}

func (u *UpdaterService) DownloadAndInstall(downloadUrl string, progressCb func(downloaded, total int64)) error {
	// 1. Try Git Pull first if inside local git repository
	if _, err := os.Stat(".git"); err == nil {
		cmd := exec.Command("git", "pull", "origin", "master")
		if out, err := cmd.CombinedOutput(); err == nil {
			fmt.Println("Git pull auto-update output:", string(out))
			return nil
		}
	}

	if downloadUrl == "" {
		return fmt.Errorf("URL cập nhật trống")
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
	batContent := fmt.Sprintf(`@echo off
set "NEW_EXE=%s"
set "OLD_EXE=%s"

timeout /t 3 /nobreak > NUL

:RETRY
move /y "%%NEW_EXE%%" "%%OLD_EXE%%" > NUL 2>&1
if errorlevel 1 (
    timeout /t 1 /nobreak > NUL
    goto RETRY
)

start "" "%%OLD_EXE%%"
del "%%~f0"
`, newExePath, exePath)

	if err := os.WriteFile(batPath, []byte(batContent), 0755); err != nil {
		return err
	}

	cmd := exec.Command("cmd.exe", "/c", "start", "/b", "", batPath)
	cmd.Dir = exeDir
	if err := cmd.Start(); err != nil {
		return err
	}

	os.Exit(0)
	return nil
}

