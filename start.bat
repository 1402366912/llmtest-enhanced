@echo off
setlocal

echo ========================================
echo   LLM-Test 生产模式启动
echo ========================================
echo.

:: 检查是否已构建
if not exist "llm-test-server.exe" (
    echo [提示] 未找到构建文件，正在构建...
    call build.bat
    if errorlevel 1 (
        echo [错误] 构建失败
        exit /b 1
    )
    echo.
)

echo 启动服务器...
echo.
echo   Web界面: http://localhost:8080
echo.
echo   功能:
echo   - 多机器管理（创建、切换）
echo   - 测试配置管理
echo   - 一键测试执行
echo   - 结果可视化
echo   - 性能天梯图
echo.
echo   按 Ctrl+C 停止服务器
echo.
echo ========================================
echo.

llm-test-server.exe -config config.yaml -addr :8080

endlocal


