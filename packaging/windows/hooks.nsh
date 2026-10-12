!macro NSIS_HOOK_PREINSTALL
  nsExec::Exec 'taskkill /F /IM flavord.exe'
!macroend

!macro NSIS_HOOK_POSTINSTALL
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Flavor daemon" '"$INSTDIR\flavord.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Flavor" '"$INSTDIR\${MAINBINARYNAME}.exe" --hidden'
  nsExec::Exec '"$INSTDIR\flavor-netd.exe" check'
  Pop $0
  ${If} $0 != 0
  ${AndIfNot} ${Silent}
    ExecShellWait "runas" "$INSTDIR\flavor-netd.exe" "install" SW_HIDE
  ${EndIf}
  Exec '"$INSTDIR\flavord.exe"'
  DeleteRegKey HKCU "Software\lunarlabs\Flavor\"
  DeleteRegKey /ifempty HKCU "Software\lunarlabs\"
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  nsExec::Exec 'taskkill /F /IM flavord.exe'
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Flavor daemon"
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Flavor"
  ${If} $UpdateMode <> 1
  ${AndIfNot} ${Silent}
    ExecShellWait "runas" "$INSTDIR\flavor-netd.exe" "uninstall" SW_HIDE
  ${EndIf}
!macroend
