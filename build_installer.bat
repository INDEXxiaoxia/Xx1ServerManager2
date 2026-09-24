@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion
cd /d "%~dp0prj"

echo ==========================================================
echo   构建 ServerManager2 安装版（NSIS 安装器）
echo   产物: prj\build\bin\ServerManager2-amd64-installer.exe
echo         prj\build\bin\ServerManager2.exe（安装器内含的应用本体）
echo ==========================================================
echo.

rem ---- 检查工具链 ----
where wails >nul 2>&1
if errorlevel 1 (
    echo [错误] 未找到 wails 命令，请先安装：go install github.com/wailsapp/wails/v2/cmd/wails@latest
    pause
    exit /b 1
)
where go >nul 2>&1
if errorlevel 1 (
    echo [错误] 未找到 go 命令，请先安装 Go
    pause
    exit /b 1
)

rem ---- 检查 NSIS（makensis），缺失则尝试使用仓库内的便携版 ----
where makensis >nul 2>&1
if errorlevel 1 (
    if exist "%~dp0.tools\nsis\nsis-3.10\makensis.exe" (
        set "PATH=%~dp0.tools\nsis\nsis-3.10;!PATH!"
        echo [提示] 使用仓库内便携版 NSIS: .tools\nsis\nsis-3.10
    ) else (
        echo [错误] 未找到 makensis（NSIS）。打安装器必须先装 NSIS：
        echo        1^) 下载 https://nsis.sourceforge.io/Download 并安装；或
        echo        2^) 把便携版解压到 .tools\nsis\ 下（脚本会自动使用）
        pause
        exit /b 1
    )
)

rem ---- 关掉正在运行的旧程序，避免 exe 被占用 ----
taskkill /im ServerManager2.exe /f >nul 2>&1
taskkill /im prj.exe /f >nul 2>&1

rem ---- 构建（前端 + 应用 + 安装器）----
echo [1/2] 编译前端、应用与安装器 ...
wails build -nsis
if errorlevel 1 (
    echo.
    echo [错误] 构建失败，请查看上方输出。
    pause
    exit /b 1
)

rem ---- 结果 ----
echo.
echo [2/2] 构建结果：
set "BIN=%~dp0prj\build\bin"
set "FOUND="
for %%F in ("%BIN%\ServerManager2-amd64-installer.exe" "%BIN%\ServerManager2.exe") do (
    if exist "%%~fF" (
        echo       %%~fF   %%~zF 字节  %%~tF
        set "FOUND=1"
    )
)
if not defined FOUND (
    echo       [警告] 未在 %BIN% 找到产物，请检查构建输出。
)

echo.
echo   安装器属性：CompanyName=INDEXxiaoxia, ProductName=ServerManager2
echo   安装路径：%%ProgramFiles%%\INDEXxiaoxia\ServerManager2
echo   注意：安装器内置的 WebView2 安装器需要联网；无网服务器请先手动安装
echo         微软官方离线运行时 MicrosoftEdgeWebView2RuntimeInstallerX64.exe
echo.
pause