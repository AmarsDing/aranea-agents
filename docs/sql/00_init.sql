-- ============================================================
-- Aranea Agents 数据库初始化脚本 (SQLite)
-- 所有建表语句均使用 IF NOT EXISTS，可重复执行
-- 本文件为主入口，按模块引用各子文件
-- ============================================================

.read 01_core.sql
.read 02_agent.sql
.read 03_session.sql
.read 04_channel.sql
.read 05_skill.sql
.read 06_cron.sql
.read 07_monitor.sql
.read 08_usage.sql
.read 09_tool.sql
.read 10_memory_l0.sql
.read 10_memory_l1.sql
.read 10_memory_l2.sql
.read 10_memory_l3.sql
.read 10_memory_l4.sql
.read 11_evolution.sql
.read 99_indexes.sql
