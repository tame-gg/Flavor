$ErrorActionPreference = "Stop"
$bin = Resolve-Path $args[0]
$helper = Join-Path $bin "flavor-netd.exe"
$installed = Join-Path $env:ProgramFiles "Flavor"
$log = Join-Path $env:RUNNER_TEMP "flavord-e2e.log"
$daemon = $null

function Wait-Until($what, [scriptblock]$check, $seconds = 30) {
    for ($i = 0; $i -lt $seconds; $i++) {
        if (& $check) { return }
        Start-Sleep 1
    }
    throw "timed out waiting for $what"
}

$nrpt = "HKLM:\SYSTEM\CurrentControlSet\Services\Dnscache\Parameters\DnsPolicyConfig"

function Flavor-Rules {
    Get-ChildItem $nrpt -ErrorAction SilentlyContinue | Get-ItemProperty | Where-Object { $_.Name -contains ".flavor.internal" }
}

try {
    & $helper install
    if ($LASTEXITCODE -ne 0) { throw "flavor-netd install failed" }
    if ((Get-Service flavor-netd).Status -ne "Running") { throw "flavor-netd is not running" }
    & $helper check
    if ($LASTEXITCODE -ne 0) { throw "check must pass right after install" }

    $daemon = Start-Process (Join-Path $bin "flavord.exe") -ArgumentList "--secret-store", "memory", "--log-level", "debug" -RedirectStandardError $log -PassThru -NoNewWindow
    Wait-Until "the Flavor adapter" { Get-NetAdapter -Name Flavor -ErrorAction SilentlyContinue | Where-Object Status -eq Up }
    Wait-Until "the NRPT rule" { Flavor-Rules }
    $servers = @((Flavor-Rules | Select-Object -First 1).GenericDNSServers -split "; ")
    "NRPT servers: $servers"
    $v4 = $servers | Where-Object { $_ -match "^198\.1[89]\." } | Select-Object -First 1
    if (-not $v4) { throw "no IPv4 resolver in the NRPT rule" }

    $answer = $null
    try {
        Resolve-DnsName -Name "nothing.flavor.internal" -Server $v4 -DnsOnly -QuickTimeout -ErrorAction Stop | Out-Null
    } catch {
        $answer = $_.Exception.Message
    }
    "resolver answered: $answer"
    if ($answer -notmatch "does not exist") { throw "the resolver behind the adapter did not answer NXDOMAIN" }

    Stop-Process -Id $daemon.Id -Force
    $daemon = $null
    Wait-Until "the adapter to go away" { -not (Get-NetAdapter -Name Flavor -ErrorAction SilentlyContinue) }
    Wait-Until "the NRPT rule to go away" { -not (Flavor-Rules) }

    & $helper uninstall
    if ($LASTEXITCODE -ne 0) { throw "flavor-netd uninstall failed" }
    if (Get-Service flavor-netd -ErrorAction SilentlyContinue) { throw "uninstall left the service" }
    if (Test-Path $installed) { throw "uninstall left $installed" }
    "windows dns helper e2e passed"
} catch {
    if ($daemon) { Stop-Process -Id $daemon.Id -Force -ErrorAction SilentlyContinue }
    & "$env:SystemRoot\System32\whoami.exe" /groups
    Get-ChildItem $nrpt -ErrorAction SilentlyContinue | Get-ItemProperty | Format-List | Out-String
    try { Get-DnsClientNrptPolicy -ErrorAction Stop | Format-List | Out-String } catch { $_.Exception | Format-List -Force | Out-String }
    query session
    Get-Content $log -Tail 40 -ErrorAction SilentlyContinue
    Get-Content (Join-Path $installed "flavor-netd.log") -Tail 40 -ErrorAction SilentlyContinue
    throw
}
