$ErrorActionPreference = 'Stop'
$fontName = 'fusion-pixel-12px-monospaced-zh_hans.otf'
$sourceFont = Join-Path $PSScriptRoot "assets\fonts\fusion-pixel\$fontName"
$fontDirectory = Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\Fonts'
$installedFont = Join-Path $fontDirectory $fontName
New-Item -ItemType Directory -Path $fontDirectory -Force | Out-Null
if (-not (Test-Path -LiteralPath $installedFont) -or
    (Get-FileHash -LiteralPath $sourceFont).Hash -ne (Get-FileHash -LiteralPath $installedFont).Hash) {
    Copy-Item -LiteralPath $sourceFont -Destination $installedFont -Force
}
$fontRegistry = 'HKCU:\Software\Microsoft\Windows NT\CurrentVersion\Fonts'
New-Item -Path $fontRegistry -Force | Out-Null
New-ItemProperty -Path $fontRegistry -Name 'Fusion Pixel 12px Mono zh-Hans (OpenType)' -Value $installedFont -PropertyType String -Force | Out-Null

if (-not ('Musicfox.FontInstaller' -as [type])) {
    Add-Type @'
using System;
using System.Runtime.InteropServices;
namespace Musicfox {
    public static class FontInstaller {
        [DllImport("gdi32.dll", CharSet = CharSet.Unicode)]
        public static extern int AddFontResourceEx(string path, uint flags, IntPtr reserved);
        [DllImport("user32.dll", CharSet = CharSet.Unicode)]
        public static extern IntPtr SendMessageTimeout(IntPtr window, uint message, UIntPtr wParam,
            IntPtr lParam, uint flags, uint timeout, out UIntPtr result);
    }
}
'@
}
if ([Musicfox.FontInstaller]::AddFontResourceEx($installedFont, 0, [IntPtr]::Zero) -eq 0) {
    throw "Windows could not load the font: $installedFont"
}
[UIntPtr]$fontChangeResult = [UIntPtr]::Zero
[Musicfox.FontInstaller]::SendMessageTimeout([IntPtr]0xffff, 0x001d, [UIntPtr]::Zero,
    [IntPtr]::Zero, 2, 1000, [ref]$fontChangeResult) | Out-Null

# Keep this profile separate from the user's default terminal settings.
$launcher = (Join-Path $PSScriptRoot 'Start-Musicfox.ps1').Replace("'", "''")
$encodedCommand = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes("& '$launcher'"))
$profile = @{
    profiles = @(@{
        guid = '{18dd3594-4c48-49c6-aaf0-17f902644a89}'
        name = 'Musicfox Pixel'
        commandline = "powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -EncodedCommand $encodedCommand"
        startingDirectory = $PSScriptRoot
        font = @{ face = 'Fusion Pixel 12px Mono zh-Hans'; size = 18; weight = 'normal' }
        antialiasingMode = 'aliased'
        intenseTextStyle = 'none'
        background = '#141310'
        foreground = '#c6c1b3'
        cursorColor = '#e58a50'
        padding = '12'
        hidden = $false
        closeOnExit = 'graceful'
    })
}
$fragmentDirectory = Join-Path $env:LOCALAPPDATA 'Microsoft\Windows Terminal\Fragments\Musicfox'
New-Item -ItemType Directory -Path $fragmentDirectory -Force | Out-Null
$fragmentPath = Join-Path $fragmentDirectory 'pixel.json'
[IO.File]::WriteAllText($fragmentPath, ($profile | ConvertTo-Json -Depth 5), [Text.UTF8Encoding]::new($false))
Write-Host 'Installed Fusion Pixel and the Musicfox Pixel terminal profile for this user.'
