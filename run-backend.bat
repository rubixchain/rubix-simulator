@echo off
echo Starting Rubix Backend Server...
echo.

cd backend

REM Build the backend (optional, for production)
REM echo Building backend...
REM go build -o rubix-simulator.exe cmd/server/main.go

REM Run the backend
echo Starting server on port 8080...
go run cmd/server/main.go

pause