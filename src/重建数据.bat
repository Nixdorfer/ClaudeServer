@echo off
chcp 65001 >nul

echo 正在重建数据库...
echo.

set PGPASSWORD=xdfrt123

echo 断开所有数据库连接...
psql -U postgres -d postgres -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='claude_db' AND pid <> pg_backend_pid();"

echo.
echo 删除数据库...
psql -U postgres -d postgres -c "DROP DATABASE IF EXISTS claude_db;"

echo.
echo 创建数据库...
psql -U postgres -d postgres -c "CREATE DATABASE claude_db;"

echo.
echo 重建表结构...
psql -U postgres -d claude_db -f database.sql

echo.
echo 数据库重建完成！
pause