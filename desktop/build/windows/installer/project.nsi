Unicode true

# Peregrine: LZMA, solid, with a dictionary big enough to span ffmpeg.exe so ffprobe.exe
# (almost the same binary) compresses nearly for free. About half the size of the default
# zlib installer (ADR 0015). Unpacking needs ~128 MB of RAM for a few seconds.
SetCompressor /SOLID lzma
SetCompressorDictSize 128

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

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

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

# Peregrine: close running copies before files are replaced or removed. On Windows the app
# keeps running in the tray after its window closes, and Chrome keeps peregrine-host.exe
# running while the extension is connected. Windows won't let setup overwrite a running exe,
# so without this an update stops with "Error opening file for writing", or leaves the old
# version running. Downloads in progress resume from their .part files on the next start.
!macro peregrine.closeRunning
    nsExec::Exec `"$SYSDIR\cmd.exe" /c ""$SYSDIR\tasklist.exe" /FI "IMAGENAME eq ${PRODUCT_EXECUTABLE}" /NH | "$SYSDIR\find.exe" /I "${PRODUCT_EXECUTABLE}""`
    Pop $0
    ${If} $0 == 0
        ${IfNot} ${Silent}
            MessageBox MB_OKCANCEL|MB_ICONINFORMATION "${INFO_PRODUCTNAME} is running and will be closed to continue.$\r$\n$\r$\nDownloads in progress continue when you start ${INFO_PRODUCTNAME} again." IDOK peregrine_close_ok
            Abort
            peregrine_close_ok:
        ${EndIf}
        # /T also ends the yt-dlp and ffmpeg processes the app started.
        nsExec::Exec `"$SYSDIR\taskkill.exe" /F /T /IM "${PRODUCT_EXECUTABLE}"`
        Pop $0
        Sleep 1000
    ${EndIf}
    # The browser starts the host again the next time the extension needs it.
    nsExec::Exec `"$SYSDIR\taskkill.exe" /F /IM "peregrine-host.exe"`
    Pop $0
!macroend

Function .onInit
   !insertmacro wails.checkArchitecture
   !insertmacro peregrine.closeRunning
FunctionEnd

Function un.onInit
   !insertmacro peregrine.closeRunning
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    # Peregrine: browser-extension bridge (native messaging host). The app also re-registers
    # it on every start, so this is just so the extension works before the first launch.
    File "..\..\bin\peregrine-host.exe"
    nsExec::Exec '"$INSTDIR\peregrine-host.exe" install'
    # Unpacked browser extension, offered by the app's "Connect your browser" prompt.
    File /r "..\..\bin\extension"
    # Video-site downloads (ADR 0013): yt-dlp and ffmpeg, pinned and checksum-verified by
    # the release workflow. /nonfatal keeps local installer builds without them working.
    File /nonfatal "..\..\bin\yt-dlp.exe"
    File /nonfatal "..\..\bin\ffmpeg.exe"
    File /nonfatal "..\..\bin\ffprobe.exe"
    File /nonfatal "..\..\bin\ffmpeg-LICENSE.txt"
    File /nonfatal "..\..\bin\THIRD-PARTY-NOTICES.txt"

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    # Peregrine: remove the browser registration. Downloads and settings in
    # %APPDATA%\Peregrine are kept on purpose.
    nsExec::Exec '"$INSTDIR\peregrine-host.exe" uninstall'

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
