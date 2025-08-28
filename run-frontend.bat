@echo off
echo Starting Rubix Frontend...
echo.

cd frontend

REM Check if node_modules exists
if not exist node_modules (
    echo Installing dependencies...
    call npm install
    echo.
)

echo Starting development server on http://localhost:5173
echo.
npm run dev