@echo off
REM start.bat — launch PolyProxy on Windows.
REM Usage: start.bat [--config PATH] [extra args...]
setlocal

set SCRIPT_DIR=%~dp0
set ROOT=%SCRIPT_DIR%..

REM Detect arch
if "%PROCESSOR_ARCHITECTURE%"=="AMD64" set HOST_ARCH=amd64
if "%PROCESSOR_ARCHITECTURE%"=="ARM64" set HOST_ARCH=arm64

set BIN=%ROOT%\bin\polyproxy-windows-%HOST_ARCH%.exe

REM Default config bootstrap
set NEEDS_CONFIG=1
set "CFG_PATH=%APPDATA%\PolyProxy\config.yaml"
if not "%~1"=="-config" goto :parse_args
set NEEDS_CONFIG=0
:parse_args
if not exist "%CFG_PATH%" if "%NEEDS_CONFIG%"=="1" (
    if not exist "%APPDATA%\PolyProxy" mkdir "%APPDATA%\PolyProxy"
    copy /Y "%ROOT%\configs\config.example.yaml" "%CFG_PATH%" >nul
    echo Bootstrapped config at %CFG_PATH% — edit and re-run.
    exit /b 0
)

if exist "%BIN%" (
    echo Launching %BIN% %*
    "%BIN%" %*
    exit /b %ERRORLEVEL%
)

where go >nul 2>nul
if %ERRORLEVEL%==0 (
    echo No prebuilt binary, falling back to: go run
    go run "%ROOT%\cmd\proxypool" %*
    exit /b %ERRORLEVEL%
)

echo ERROR: no binary at %BIN% and go not installed.
exit /b 1