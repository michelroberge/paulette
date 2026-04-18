@echo off
setlocal

echo [1/3] Building frontend...
cd "%~dp0frontend"
call npm run build
if errorlevel 1 (
    echo ERROR: Frontend build failed.
    exit /b 1
)

echo [2/3] Copying dist to backend/static...
cd "%~dp0"
if exist "backend\static\assets" rmdir /s /q "backend\static\assets"
xcopy /e /y "frontend\dist\*" "backend\static\"
if errorlevel 1 (
    echo ERROR: Copy failed.
    exit /b 1
)

echo [3/3] Building backend...
cd "%~dp0backend"
go build -o paulette.exe .
if errorlevel 1 (
    echo ERROR: Backend build failed.
    exit /b 1
)

echo.
echo Done. Run backend\paulette.exe and open http://localhost:8080
endlocal
