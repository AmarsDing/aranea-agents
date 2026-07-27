\echo '=== agents matching xiaohongshu/小红书/weibo/微博/social ==='
SELECT agent_key, left(display_name,20) AS name, status FROM agents WHERE agent_key ILIKE '%xiaohongshu%' OR display_name LIKE '%小红书%' OR agent_key ILIKE '%weibo%' OR display_name LIKE '%微博%' OR agent_key ILIKE '%social%' OR display_name LIKE '%社交媒体%' ORDER BY agent_key;
\echo '=== test teams in teams table ==='
SELECT team_key, left(display_name,24) AS name, status FROM teams WHERE team_key LIKE 'test_social%' ORDER BY team_key;
