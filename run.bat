@echo off
echo Starting Rubix Simulator...
echo.

REM Start backend server
echo Starting backend server...
start "Rubix Backend" /D backend cmd /k "go run cmd/server/main.go"

REM Wait for backend to start
echo Waiting for backend to initialize...
timeout /t 5 /nobreak > nul

REM Check if backend is running
curl -s http://localhost:8080/health > nul 2>&1
if %errorlevel% neq 0 (
    echo Backend failed to start! Check the backend window for errors.
    pause
    exit /b 1
)

echo Backend started successfully on http://localhost:8080
echo.

REM Start frontend
echo Starting frontend...
cd frontend

REM Check if node_modules exists
if not exist node_modules (
    echo Installing frontend dependencies...
    call npm install
)

echo Frontend starting on http://localhost:5173
echo.
echo ========================================
echo Rubix Simulator is running!
echo Backend: http://localhost:8080
echo Frontend: http://localhost:5173
echo.
echo Press Ctrl+C to stop
echo ========================================
echo.

REM Run frontend in new window
start "Rubix Frontend" cmd /k "npm run dev"

REM Keep this window open
echo.
echo Both servers are running in separate windows.
echo Close this window to stop monitoring.
pause