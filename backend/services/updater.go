package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"claude_suite/backend/version"

	"claude_suite/backend/sysproc"
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

// A release carries four .exe assets: the NSIS installer, the portable app,
// and the two companion CLI tools. "First asset ending in .exe" is therefore
// never a safe pick — depending on upload order it can hand the updater
// claude-suite-claim.exe as the new app binary.
//
// An installed copy (under Program Files) must update through the installer,
// which requests elevation itself; a portable copy gets the bare exe swapped
// in place.
func chooseAssetURL(assets []ReleaseAsset, installed bool) string {
	installer, portable := "", ""
	for _, a := range assets {
		name := strings.ToLower(a.Name)
		if filepath.Ext(name) != ".exe" {
			continue
		}
		switch {
		case strings.Contains(name, "installer"):
			if installer == "" {
				installer = a.BrowserDownloadURL
			}
		case strings.Contains(name, "claim"), strings.Contains(name, "tui"):
			// companion CLI tools — never the app update
		default:
			if portable == "" {
				portable = a.BrowserDownloadURL
			}
		}
	}
	if installed {
		if installer != "" {
			return installer
		}
		return portable
	}
	if portable != "" {
		return portable
	}
	return installer
}

// isInstalledCopy reports whether this binary was placed by the installer
// rather than carried around as the portable exe. The NSIS script always
// writes uninstall.exe beside the app, so its presence is the marker. An
// unwritable exe dir counts too: swap-in-place cannot work there regardless
// of how the binary arrived, so the installer path is the only one that can
// succeed.
func isInstalledCopy() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}
	exeDir := filepath.Dir(exePath)
	if _, err := os.Stat(filepath.Join(exeDir, "uninstall.exe")); err == nil {
		return true
	}
	return !dirWritable(exeDir)
}

// isNewerVersion reports whether candidate is a strictly newer release than
// current. `!=` was the old test, which read a yanked newest release as an
// "update" and silently downgraded every install that was already past it.
func isNewerVersion(candidate, current string) bool {
	ca, okC := parseSemverTag(candidate)
	cu, okU := parseSemverTag(current)
	if !okC {
		return false // an unparsable tag is not an update
	}
	if !okU {
		return true // dev/unknown build: any real release is newer
	}
	for i := 0; i < 3; i++ {
		if ca[i] != cu[i] {
			return ca[i] > cu[i]
		}
	}
	return false
}

func parseSemverTag(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	// Any prerelease/build suffix compares as its base: "2.16.0-rc1" → 2.16.0.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// isInstallerAsset decides the handover mode from the asset FILENAME, never
// the whole URL: GitHub download URLs embed the release tag, and a tag like
// v2.16.0-installer-fix would otherwise route a portable exe through the
// installer branch, which runs it once from %TEMP% and updates nothing.
func isInstallerAsset(downloadUrl string) bool {
	p := downloadUrl
	if parsed, err := url.Parse(downloadUrl); err == nil && parsed.Path != "" {
		p = parsed.Path
	}
	return strings.Contains(strings.ToLower(path.Base(p)), "installer")
}

// cleanStaleUpdateFiles sweeps leftovers of past updates out of the temp dir.
// The installer handover cannot delete its own download (the app exits while
// the installer runs from it), so without this every accepted update parks
// another full-size exe in %TEMP% for good. Only files older than a day go:
// a concurrent update's files are in active use.
func cleanStaleUpdateFiles() {
	tmp := os.TempDir()
	for _, pattern := range []string{"ClaudeSuite-update-*.exe", "claude_suite_updater-*.bat"} {
		matches, _ := filepath.Glob(filepath.Join(tmp, pattern))
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
				os.Remove(m)
			}
		}
	}
}

func dirWritable(dir string) bool {
	probe, err := os.CreateTemp(dir, ".claude-suite-write-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return true
}

func (u *UpdaterService) CheckForUpdates() (*UpdateInfo, error) {
	apiUrl := "https://api.github.com/repos/thydynh03/claude_suite/releases/latest"
	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ClaudeSuite-App")

	resp, err := http.DefaultClient.Do(req)
	if err == nil && resp.StatusCode != http.StatusOK {
		// Closed here too: a 403 rate-limit or 404 used to leave the body —
		// and its connection — pinned for the life of the process.
		resp.Body.Close()
	}
	if err == nil && resp.StatusCode == http.StatusOK {
		var rel GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&rel); err == nil && rel.TagName != "" {
			resp.Body.Close()
			curVer := version.GetVersion()
			if isNewerVersion(rel.TagName, curVer) {
				downloadUrl := chooseAssetURL(rel.Assets, isInstalledCopy())
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
			return &UpdateInfo{HasUpdate: false, Version: curVer}, nil
		}
		resp.Body.Close()
	}

	// Fallback: Check tags API if releases API is empty
	tagsUrl := "https://api.github.com/repos/thydynh03/claude_suite/tags"
	tReq, err := http.NewRequest("GET", tagsUrl, nil)
	if err == nil {
		tReq.Header.Set("User-Agent", "ClaudeSuite-App")
		tResp, err := http.DefaultClient.Do(tReq)
		if err == nil && tResp.StatusCode != http.StatusOK {
			tResp.Body.Close()
		}
		if err == nil && tResp.StatusCode == http.StatusOK {
			var tags []GitHubTag
			if err := json.NewDecoder(tResp.Body).Decode(&tags); err == nil && len(tags) > 0 {
				tResp.Body.Close()
				// The semver-max of the list, not tags[0]: the API's ordering
				// is not a contract this decision should rest on.
				latestTag := tags[0].Name
				for _, tag := range tags[1:] {
					if isNewerVersion(tag.Name, latestTag) {
						latestTag = tag.Name
					}
				}
				curVer := version.GetVersion()
				if isNewerVersion(latestTag, curVer) {
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

	return &UpdateInfo{HasUpdate: false, Version: version.GetVersion()}, nil
}

// buildUpdaterBat swaps the downloaded exe over the running one after it
// exits.
//
// The retry loop is bounded: an unelevated write into a protected directory
// fails identically forever, and the unbounded version of this script sat in
// the background retrying for the rest of the session. Giving up still falls
// through to the relaunch — the copy failed, so the old exe is intact, and a
// script that exits without restarting anything looks to the user like the
// update crashed their app.
//
// `copy` rather than `move`: a same-volume move is a rename, and on NTFS a
// renamed file keeps the DACL it had in %TEMP% (owner-only) instead of
// inheriting from the app's directory — on a shared machine every other
// account then loses the ability to launch the app. Overwriting in place
// keeps the destination's ACL.
//
// ping is the sleep — timeout.exe aborts when it has no usable stdin.
func buildUpdaterBat(newExe, oldExe string) string {
	return fmt.Sprintf(`@echo off
set "NEW_EXE=%s"
set "OLD_EXE=%s"
set COUNT=0

ping -n 4 127.0.0.1 > NUL

:RETRY
copy /b /y "%%NEW_EXE%%" "%%OLD_EXE%%" > NUL 2>&1
if not errorlevel 1 goto DONE
set /a COUNT+=1
if %%COUNT%% GEQ 30 goto DONE
ping -n 2 127.0.0.1 > NUL
goto RETRY

:DONE
del "%%NEW_EXE%%" > NUL 2>&1
start "" "%%OLD_EXE%%"
del "%%~f0"
`, newExe, oldExe)
}

func (u *UpdaterService) DownloadAndInstall(downloadUrl string, progressCb func(downloaded, total int64)) error {
	// There used to be a "git pull origin master" fast path here, gated on
	// `.git` existing — in the process CWD, which for a packaged app is
	// wherever the user launched it from. Updating from inside one of their
	// repositories merged origin/master into THAT repo, reported the update
	// as done, and left the app binary untouched, forever re-offering the
	// same version. Developers run `wails dev`; the updater is for users.

	if downloadUrl == "" {
		return fmt.Errorf("URL cập nhật trống")
	}
	if strings.HasSuffix(strings.ToLower(downloadUrl), ".zip") {
		return fmt.Errorf("bản phát hành này không có file .exe để tự cập nhật — vui lòng tải và cài thủ công từ GitHub")
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exeDir := filepath.Dir(exePath)

	cleanStaleUpdateFiles()

	// Download into the temp dir, never into the app's own directory: an
	// installed copy lives under Program Files, where an unelevated write
	// is denied before the update even starts.
	out, err := os.CreateTemp("", "ClaudeSuite-update-*.exe")
	if err != nil {
		return err
	}
	newExePath := out.Name()

	resp, err := http.Get(downloadUrl)
	if err != nil {
		out.Close()
		os.Remove(newExePath)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out.Close()
		os.Remove(newExePath)
		return fmt.Errorf("tải bản cập nhật thất bại: HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, wErr := out.Write(buf[:n])
			if wErr != nil {
				out.Close()
				os.Remove(newExePath)
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
			out.Close()
			os.Remove(newExePath)
			return err
		}
	}
	out.Close()

	// An installer asset elevates itself (UAC) and knows how to migrate the
	// existing installation; hand over to it and quit so it can replace our
	// files. `start` goes through ShellExecute, which is what makes the UAC
	// prompt appear — exec'ing a manifest-admin exe directly fails with
	// ERROR_ELEVATION_REQUIRED.
	if isInstallerAsset(downloadUrl) {
		// Run, not Start: `start` returns only after ShellExecuteEx resolves,
		// which for a manifest-admin exe means after the UAC prompt is
		// answered — install time is not waited on. Exiting before that
		// answer is known turned a declined prompt into an app that closed,
		// updated nothing, and reported success.
		cmd := sysproc.Command("cmd.exe", "/c", "start", "", newExePath)
		if err := cmd.Run(); err != nil {
			os.Remove(newExePath)
			return fmt.Errorf("trình cài đặt không khởi động được — có thể bạn đã bấm No ở hộp thoại UAC. Chưa có gì thay đổi, thử lại khi sẵn sàng (%v)", err)
		}
		os.Exit(0)
		return nil
	}

	// Portable copy: swap the exe in place. Only possible where we can write;
	// otherwise the background script would retry a doomed move with no way
	// to report back.
	if !dirWritable(exeDir) {
		os.Remove(newExePath)
		return fmt.Errorf("không có quyền ghi vào %s — hãy tải file *-installer.exe từ GitHub và chạy nó", exeDir)
	}

	// A unique name per update: cmd.exe re-reads a running bat by byte offset
	// and does not hold it exclusively, so rewriting a fixed path while a
	// previous update's script is still in its retry window hands that
	// interpreter garbled commands.
	batFile, err := os.CreateTemp("", "claude_suite_updater-*.bat")
	if err != nil {
		os.Remove(newExePath)
		return err
	}
	batPath := batFile.Name()
	if _, err := batFile.WriteString(buildUpdaterBat(newExePath, exePath)); err != nil {
		batFile.Close()
		os.Remove(newExePath)
		os.Remove(batPath)
		return err
	}
	batFile.Close()

	cmd := sysproc.Command("cmd.exe", "/c", "start", "/b", "", batPath)
	cmd.Dir = exeDir
	if err := cmd.Start(); err != nil {
		os.Remove(newExePath)
		os.Remove(batPath)
		return err
	}

	os.Exit(0)
	return nil
}
