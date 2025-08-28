@echo off
echo Building Rubix Simulator for Production...
echo.

REM Build backend
echo Building backend...
cd backend
go build -o rubix-simulator.exe cmd/server/main.go
if %errorlevel% neq 0 (
    echo Backend build failed!
    pause
    exit /b 1
)
echo Backend built successfully: backend\rubix-simulator.exe
cd ..
echo.

REM Build frontend
echo Building frontend...
cd frontend
if not exist node_modules (
    echo Installing frontend dependencies...
    call npm install
)
call npm run build
if %errorlevel% neq 0 (
    echo Frontend build failed!
    pause
    exit /b 1
)
echo Frontend built successfully: frontend\dist
cd ..
echo.

echo ========================================
echo Build completed successfully!
echo.
echo Backend executable: backend\rubix-simulator.exe
echo Frontend build: frontend\dist
echo.
echo To run in production:
echo   1. Run backend\rubix-simulator.exe
echo   2. Serve frontend\dist with a web server
echo ========================================
pause