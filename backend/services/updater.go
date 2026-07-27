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

	"agent_center/backend/version"

	"agent_center/backend/sysproc"
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

// http.DefaultClient has no timeout. A connection that stalls after the
// handshake — captive portal, a firewall that drops instead of refusing —
// parked the update check forever, and the UI spun with no error. The
// download gets its own, far longer budget: it moves tens of megabytes.
var (
	updateCheckClient    = &http.Client{Timeout: 20 * time.Second}
	updateDownloadClient = &http.Client{Timeout: 15 * time.Minute}
)

// updateGaveUpMarker is written by the swap script when it could not replace
// the exe. Reported once, on the next check, so a silently-lost update stops
// looking like an update that never happened.
func updateGaveUpMarker() string {
	return filepath.Join(os.TempDir(), "agent_center_update_gaveup.txt")
}

// TakeFailedSwapNotice reports (and clears) a previous update whose file swap
// never succeeded — the exe stayed locked past the retry window, usually
// Defender scanning the fresh download or a syncing folder.
func (u *UpdaterService) TakeFailedSwapNotice() bool {
	marker := updateGaveUpMarker()
	if _, err := os.Stat(marker); err != nil {
		return false
	}
	os.Remove(marker)
	return true
}

// A release carries four .exe assets: the NSIS installer, the portable app,
// and the two companion CLI tools. "First asset ending in .exe" is therefore
// never a safe pick — depending on upload order it can hand the updater
// agent-center-claim.exe as the new app binary.
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
	for _, pattern := range []string{"AgentCenter-update-*.exe", "agent_center_updater-*.bat"} {
		matches, _ := filepath.Glob(filepath.Join(tmp, pattern))
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
				os.Remove(m)
			}
		}
	}
}

func dirWritable(dir string) bool {
	probe, err := os.CreateTemp(dir, ".agent-center-write-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return true
}

func (u *UpdaterService) CheckForUpdates() (*UpdateInfo, error) {
	// Why the check can fail is reported rather than swallowed: every failure
	// used to return "no update available", so an offline first launch was
	// told "Already up to date." — a false assurance that a proxy-blocked
	// machine would repeat forever.
	var lastErr error

	apiUrl := "https://api.github.com/repos/thydynh03/agent_center/releases/latest"
	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AgentCenter-App")

	resp, err := updateCheckClient.Do(req)
	if err != nil {
		lastErr = err
	}
	if err == nil && resp.StatusCode != http.StatusOK {
		lastErr = fmt.Errorf("GitHub trả về HTTP %d", resp.StatusCode)
		// Closed here too: a 403 rate-limit or 404 used to leave the body —
		// and its connection — pinned for the life of the process.
		resp.Body.Close()
	}
	if err == nil && resp.StatusCode == http.StatusOK {
		var rel GitHubRelease
		decErr := json.NewDecoder(resp.Body).Decode(&rel)
		if decErr != nil {
			lastErr = decErr
		}
		if decErr == nil && rel.TagName != "" {
			resp.Body.Close()
			curVer := version.GetVersion()
			if isNewerVersion(rel.TagName, curVer) {
				downloadUrl := chooseAssetURL(rel.Assets, isInstalledCopy())
				body := rel.Body
				if downloadUrl == "" {
					// No .exe in the release: the install button can only ever
					// fail on this, so say so up front instead of letting the
					// user discover it by pressing it.
					downloadUrl = "https://github.com/thydynh03/agent_center/archive/refs/tags/" + rel.TagName + ".zip"
					body = manualOnlyNotice(rel.TagName) + body
				}
				return &UpdateInfo{
					HasUpdate:   true,
					Version:     rel.TagName,
					DownloadURL: downloadUrl,
					Body:        body,
				}, nil
			}
			return &UpdateInfo{HasUpdate: false, Version: curVer}, nil
		}
		resp.Body.Close()
	}

	// Fallback: Check tags API if releases API is empty
	tagsUrl := "https://api.github.com/repos/thydynh03/agent_center/tags"
	tReq, err := http.NewRequest("GET", tagsUrl, nil)
	if err == nil {
		tReq.Header.Set("User-Agent", "AgentCenter-App")
		tResp, err := updateCheckClient.Do(tReq)
		if err != nil {
			lastErr = err
		}
		if err == nil && tResp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("GitHub trả về HTTP %d", tResp.StatusCode)
			tResp.Body.Close()
		}
		if err == nil && tResp.StatusCode == http.StatusOK {
			var tags []GitHubTag
			decErr := json.NewDecoder(tResp.Body).Decode(&tags)
			if decErr != nil {
				lastErr = decErr
			}
			if decErr == nil && len(tags) > 0 {
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
						DownloadURL: "https://github.com/thydynh03/agent_center/archive/refs/tags/" + latestTag + ".zip",
						Body:        manualOnlyNotice(latestTag) + "Bản phát hành mới " + latestTag + " trên GitHub Repository.",
					}, nil
				}
			}
			tResp.Body.Close()
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("không kiểm tra được bản cập nhật (kiểm tra kết nối mạng): %w", lastErr)
	}
	return &UpdateInfo{HasUpdate: false, Version: version.GetVersion()}, nil
}

// manualOnlyNotice heads the release notes when the only thing to download is
// a source archive: DownloadAndInstall refuses .zip, so the update exists but
// cannot be installed from inside the app.
func manualOnlyNotice(tag string) string {
	return "⚠️ Bản " + tag + " chưa có file .exe để tự cập nhật — hãy tải và cài thủ công tại " +
		"https://github.com/thydynh03/agent_center/releases/latest\n\n"
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
//
// `chcp 65001` comes first because cmd.exe decodes a .bat in the console's
// OEM codepage, not UTF-8: a Vietnamese user profile (%TEMP% carries the
// account name) or a folder like D:\Phần mềm\ arrived at the copy as
// mojibake, so the swap failed 30 times and the app — already exited —
// never came back. The preamble is pure ASCII, so it decodes identically in
// any codepage and switches the interpreter before it reads a path.
//
// A literal % in either path is doubled: batch eats single percents while
// expanding variables, which silently truncated the path.
func buildUpdaterBat(newExe, oldExe string) string {
	esc := func(p string) string { return strings.ReplaceAll(p, "%", "%%") }
	marker := esc(updateGaveUpMarker())
	return fmt.Sprintf(`@echo off
chcp 65001 > NUL
set "NEW_EXE=%s"
set "OLD_EXE=%s"
set "GAVEUP=%s"
set COUNT=0

ping -n 4 127.0.0.1 > NUL

:RETRY
copy /b /y "%%NEW_EXE%%" "%%OLD_EXE%%" > NUL 2>&1
if not errorlevel 1 goto DONE
set /a COUNT+=1
if %%COUNT%% GEQ 60 goto GAVEUP
ping -n 2 127.0.0.1 > NUL
goto RETRY

:GAVEUP
echo swap failed> "%%GAVEUP%%"

:DONE
del "%%NEW_EXE%%" > NUL 2>&1
start "" "%%OLD_EXE%%"
del "%%~f0"
`, esc(newExe), esc(oldExe), marker)
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
	out, err := os.CreateTemp("", "AgentCenter-update-*.exe")
	if err != nil {
		return err
	}
	newExePath := out.Name()

	dlReq, err := http.NewRequest("GET", downloadUrl, nil)
	if err != nil {
		out.Close()
		os.Remove(newExePath)
		return err
	}
	dlReq.Header.Set("User-Agent", "AgentCenter-App")
	resp, err := updateDownloadClient.Do(dlReq)
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
	batFile, err := os.CreateTemp("", "agent_center_updater-*.bat")
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
