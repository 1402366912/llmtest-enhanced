@echo off
setlocal

echo ========================================
echo   LLM-Test 构建脚本 (Windows)
echo ========================================

echo.
echo [1/3] 安装前端依赖...
cd web
call npm install
if errorlevel 1 goto :error

echo.
echo [2/3] 构建前端...
call npm run build
if errorlevel 1 goto :error
cd ..

echo.
echo [3/3] 构建后端...
go build -o llm-test-server.exe ./cmd/server
if errorlevel 1 goto :error

echo.
echo ========================================
echo   构建完成！
echo ========================================
echo.
echo 启动命令:
echo   llm-test-server.exe
echo.
echo 然后访问: http://localhost:8080
echo.
goto :end

:error
echo.
echo 构建失败！
exit /b 1

:end
endlocal
