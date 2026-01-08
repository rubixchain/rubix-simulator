@echo off
setlocal enabledelayedexpansion

echo Shutting down Rubix nodes...
echo.

set "base_port=25000"
set "nodes_dir=backend\rubix-data\nodes"
set "rubix_exe=backend\rubix-data\rubixgoplatform\windows\rubixgoplatform.exe"

REM Check if nodes directory exists
if not exist "%nodes_dir%" (
    echo No nodes directory found.
    pause
    exit /b
)

REM Loop through node directories
for /d %%d in (%nodes_dir%\node*) do (
    REM Extract node number from directory name
    set "dirname=%%~nxd"

    REM Handle both old format (node0) and new format (node_25000_0)
    echo !dirname! | findstr /R "^node_[0-9]*_[0-9]*$" >nul
    if !errorlevel! equ 0 (
        REM New format: node_{port}_{index} - extract last part after last underscore
        for /f "tokens=3 delims=_" %%i in ("!dirname!") do set "nodenum=%%i"
    ) else (
        REM Old format: node{index}
        set "nodenum=!dirname:node=!"
    )

    REM Calculate port
    set /a port=%base_port% + !nodenum!

    echo Shutting down !dirname! on port !port!...
    "%rubix_exe%" shutdown -port !port!
)

echo.
echo All nodes shut down.
endlocal
pause