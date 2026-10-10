!macro NSIS_HOOK_PREINSTALL
  nsExec::Exec 'taskkill /F /IM flavord.exe'
!macroend

!macro NSIS_HOOK_POSTINSTALL
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Flavor daemon" '"$INSTDIR\flavord.exe"'
  Exec '"$INSTDIR\flavord.exe"'
  DeleteRegKey HKCU "Software\lunarlabs\Flavor\"
  DeleteRegKey /ifempty HKCU "Software\lunarlabs\"
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  nsExec::Exec 'taskkill /F /IM flavord.exe'
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Flavor daemon"
!macroend
