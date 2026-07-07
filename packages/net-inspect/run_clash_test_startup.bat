@echo off
title Clash Verge Latency Test - Startup
echo ====================================================
echo Waiting for 20 seconds for network and Clash to initialize...
echo ====================================================
timeout /t 20 /nobreak
echo.
echo Starting Clash Verge Latency Test...
"%~dp0net_inspect.exe" -clash -clash-concurrency 25 -clash-timeout 2000 -clash-hosts "https://generativelanguage.googleapis.com,https://drivers.amd.com,http://objects.githubusercontent.com,http://91.108.56.199"
echo.
echo ====================================================
echo Latency test completed!
echo ====================================================
echo Press any key to close this window...
pause > nul
