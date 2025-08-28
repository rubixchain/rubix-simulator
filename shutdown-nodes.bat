@echo off
setlocal enabledelayedexpansion

echo Shutting down Rubix nodes...
echo.

set "base_port=20000"
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
    set "nodenum=!dirname:node=!"
    
    REM Calculate port
    set /a port=%base_port% + !nodenum!
    
    echo Shutting down node!nodenum! on port !port!...
    "%rubix_exe%" shutdown -port !port!
)

echo.
echo All nodes shut down.
endlocal
pause