@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0prj"

echo ==========================================================
echo   构建 ServerManager2 免安装版
echo   产物: prj\build\bin\ServerManager2.exe（无需再挪动）
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

rem ---- 先关掉正在运行的旧程序，否则 exe 被占用写不进去 ----
taskkill /im ServerManager2.exe /f >nul 2>&1
taskkill /im prj.exe /f >nul 2>&1

rem ---- 构建（含前端编译）----
echo [1/2] 编译前端与应用 ...
wails build
if errorlevel 1 (
    echo.
    echo [错误] 构建失败，请查看上方输出。
    pause
    exit /b 1
)

rem ---- 结果 ----
echo.
echo [2/2] 构建结果：
set "TARGET=%~dp0prj\build\bin\ServerManager2.exe"
if exist "%TARGET%" (
    for %%A in ("%TARGET%") do echo       %%~fA   %%~zA 字节  %%~tA
    echo.
    echo   免安装版可直接双击运行；首次运行前请确保系统已安装 WebView2 运行时。
) else (
    echo       [警告] 未在预期位置找到 ServerManager2.exe，请检查 prj\build\bin\ 目录。
)

echo.
pause