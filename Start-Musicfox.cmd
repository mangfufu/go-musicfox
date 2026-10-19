@echo off
setlocal
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0Start-Musicfox.ps1" -Pixel %*
set "musicfoxExitCode=%ERRORLEVEL%"
if not "%musicfoxExitCode%"=="0" (
    echo.
    echo Musicfox failed to start. Copy the error above when reporting this problem.
    pause
)
exit /b %musicfoxExitCode%
