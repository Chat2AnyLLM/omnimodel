@echo off
REM Quick setup script to add make to PATH for this session
REM Run this in PowerShell: .\setup-make.bat

if exist "C:\Program Files (x86)\GnuWin32\bin\make.exe" (
    echo Adding GnuWin32 make to PATH...
    set PATH=C:\Program Files (x86)\GnuWin32\bin;%PATH%
    make --version
    echo.
    echo ✓ make is now available in this session
    echo.
    echo Try: make help
) else (
    echo make.exe not found at C:\Program Files ^(x86^)\GnuWin32\bin
    echo.
    echo To install make, run one of:
    echo   winget install GnuWin32.Make
    echo   choco install make (requires admin)
    echo   scoop install make
)
