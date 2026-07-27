Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "{{.Info.ProductVersion}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"
!include "LogicLib.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

# Reopen the app when the update is done.
#
# The in-app updater exits the running app and hands over to this installer, so
# that it can replace the files the app is holding open. Without this the whole
# update ends with no app on screen and nothing said about it — the user is left
# to work out for themselves that the thing they were using is now in the Start
# menu, one version newer.
#
# Launched through explorer.exe rather than MUI's default Exec. This installer
# is RequestExecutionLevel admin, so Exec would start the app with the
# installer's elevated token, and that is not a cosmetic difference: sub-agents
# run with --dangerously-skip-permissions, so an elevated app means every agent
# it spawns is elevated too. An elevated window also silently refuses
# drag-and-drop from Explorer (UIPI). explorer.exe already runs as the logged-in
# user, so the child it starts is back at medium integrity.
#
# A silent install (/S — the release workflow uses it to verify the payload)
# shows no pages at all, so nothing is launched on CI.
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchAppAsUser
!define MUI_FINISHPAGE_RUN_TEXT "Mở Claude Suite"

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

# See MUI_FINISHPAGE_RUN above for why this goes through explorer.exe instead of
# starting the executable directly.
Function LaunchAppAsUser
    Exec '"$WINDIR\explorer.exe" "$INSTDIR\${PRODUCT_EXECUTABLE}"'
FunctionEnd

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
!ifdef WAILS_INSTALL_SCOPE
  !if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
  !else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
  !endif
!else
  InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture

   # Everything below reads keys that wails_tools.nsh wrote under
   # `SetRegView 64` into HKLM (this installer is RequestExecutionLevel admin).
   # .onInit runs before any Section, so neither the view nor the shell
   # context has been set yet: the first version of this check read SHCTX
   # (= HKCU here) in the 32-bit view and could never fire.
   SetRegView 64

   # Builds up to v2.14.1 installed under the author's GitHub handle
   # ("...\thydynh03\Claude Suite"). companyName is the product name now, so
   # without this a machine ends up with two installations and two entries in
   # Apps & features, and the old Start-menu shortcut keeps launching the old
   # build. Declining is allowed: it is the user's machine.
   # The key name keeps the space: UNINST_KEY_NAME is the bare concatenation
   # of companyName and productName, which were "thydynh03" + "Claude Suite".
   ReadRegStr $R0 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\thydynh03Claude Suite" "UninstallString"
   ${If} $R0 != ""
       # /SD IDNO: a silent install (/S — CI uses it) must not pop a dialog;
       # declining is the safe default there.
       MessageBox MB_YESNO|MB_ICONQUESTION \
           "Đã có bản Claude Suite cũ cài ở thư mục khác. Gỡ bản cũ trước khi cài bản mới?" \
           /SD IDNO IDNO skip_old_uninstall
       # _?= makes the uninstaller run in place instead of copying itself to
       # %TEMP% and returning immediately — without it ExecWait comes back in
       # milliseconds and the still-running old uninstall deletes the "Claude
       # Suite.lnk" shortcuts AFTER this install has just created them (both
       # builds share the shortcut names). The old dir is derived from the
       # UninstallString's exact shape `"<dir>\uninstall.exe"`.
       StrCpy $R2 $R0 "" 1
       StrCpy $R2 $R2 -15
       ExecWait '$R0 /S _?=$R2' $0
       # In-place mode also means it cannot delete itself; finish for it.
       Delete "$R2\uninstall.exe"
       RMDir "$R2"
       skip_old_uninstall:
   ${EndIf}

   # An update must land where the app already is, not in the default dir:
   # the in-app updater hands over to this installer, and a copy installed
   # via the directory chooser would otherwise get a second installation in
   # Program Files while the chosen one is orphaned. InstallLocation is
   # written by our own install Section below; older installs only have
   # UninstallString, whose exact shape is `"$INSTDIR\uninstall.exe"` —
   # 1 leading quote to skip, 15 trailing characters to drop.
   ReadRegStr $R1 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${UNINST_KEY_NAME}" "InstallLocation"
   ${If} $R1 == ""
       ReadRegStr $R1 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${UNINST_KEY_NAME}" "UninstallString"
       ${If} $R1 != ""
           StrCpy $R1 $R1 "" 1
           StrCpy $R1 $R1 -15
       ${EndIf}
   ${EndIf}
   ${If} $R1 != ""
       StrCpy $INSTDIR $R1
   ${EndIf}
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    # The companion command-line tools ship beside the app. claude-suite-claim is
    # the one a teammate's agent runs to file a claim, and the join command the
    # app hands out names this exact path — without it that line only works for
    # someone who built the project from source.
    #
    # This file was untracked for three releases: .gitignore ignored all of
    # build/, so CI never saw it and wails regenerated the stock template on every
    # run. Adding these two lines locally therefore changed nothing, and the
    # give-away was an installer that grew by 92 bytes after two binaries
    # totalling 29MB were supposedly added to it. If this file ever stops being
    # tracked, that is the failure to expect.
    #
    # ${__FILEDIR__} anchors the paths to this script rather than to makensis's
    # working directory. No /nonfatal, so a missing tool fails the build instead
    # of silently producing an installer without it.
    File "${__FILEDIR__}\..\..\bin\claude-suite-claim.exe"
    File "${__FILEDIR__}\..\..\bin\claude-suite-tui.exe"

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller

    # Where this install landed, so the next update's .onInit can come back
    # to it. wails.writeUninstaller records everything about this key except
    # the one value that says where the installation is.
    WriteRegStr SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\${UNINST_KEY_NAME}" "InstallLocation" "$INSTDIR"
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
