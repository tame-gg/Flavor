$ErrorActionPreference = "Stop"
$setup = Get-ChildItem dist\*-setup.exe | Select-Object -First 1
$dir = Join-Path $env:LOCALAPPDATA "Flavor"
$runKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
$ctl = Join-Path $dir "flavorctl.exe"

Start-Process $setup.FullName -ArgumentList "/S" -Wait
if (-not (Get-ItemProperty $runKey -ErrorAction SilentlyContinue)."Flavor daemon") { throw "installer did not add the Run entry" }
for ($i = 0; $i -lt 30 -and -not (Get-Process flavord -ErrorAction SilentlyContinue); $i++) { Start-Sleep 1 }
if (-not (Get-Process flavord -ErrorAction SilentlyContinue)) { throw "installer did not start flavord" }

& $ctl --version
for ($i = 0; $i -lt 30; $i++) { & $ctl info; if ($LASTEXITCODE -eq 0) { break }; Start-Sleep 1 }
if ($LASTEXITCODE -ne 0) { throw "flavorctl info failed" }
$diag = & $ctl diag | Out-String
$diag
if ($diag -notmatch "wincred") { throw "diag does not report the Windows Credential Manager" }
if (-not (Test-Path (Join-Path $dir "flavord.log"))) { throw "flavord did not write its log file" }

Start-Process $setup.FullName -ArgumentList "/S" -Wait
for ($i = 0; $i -lt 30 -and -not (Get-Process flavord -ErrorAction SilentlyContinue); $i++) { Start-Sleep 1 }
& $ctl info
if ($LASTEXITCODE -ne 0) { throw "flavord did not come back after reinstalling" }

Start-Process (Join-Path $dir "uninstall.exe") -ArgumentList "/S" -Wait
for ($i = 0; $i -lt 60 -and (Test-Path $ctl); $i++) { Start-Sleep 1 }
if ((Get-ItemProperty $runKey -ErrorAction SilentlyContinue)."Flavor daemon") { throw "uninstall left the Run entry" }
if (Get-Process flavord -ErrorAction SilentlyContinue) { throw "uninstall left flavord running" }
if (-not (Test-Path (Join-Path $dir "database"))) { throw "uninstall removed the network database" }
"installer smoke test passed"
