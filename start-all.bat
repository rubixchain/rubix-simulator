@echo off
echo ===================================
echo Starting Rubix Network Simulator
echo ===================================
echo.

REM Start Backend
echo Starting Go Backend on port 8080...
start "Rubix Backend" cmd /k "cd backend && go run cmd/server/main.go"

REM Wait a bit for backend to start
timeout /t 3 /nobreak > nul

REM Start Frontend
echo Starting React Frontend on port 5173...
start "Rubix Frontend" cmd /k "npm run dev"

echo.
echo ===================================
echo Services Starting:
echo - Backend:  http://localhost:8080
echo - Frontend: http://localhost:5173
echo ===================================
echo.
echo The browser should open automatically.
echo Press any key to stop all services...
pause > nul

REM Kill both processes
taskkill /FI "WindowTitle eq Rubix Backend*" /T /F
taskkill /FI "WindowTitle eq Rubix Frontend*" /T /F