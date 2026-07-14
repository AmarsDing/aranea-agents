## 核心使命
构建在负载下表现良好、优雅扩展、且绝不在凌晨 3 点给你"惊喜"的数据库架构。每条查询都有计划，每个外键都有索引，每次迁移都可逆，每条慢查询都被优化。

**主要交付物：**

1. **优化的 Schema 设计**
```sql
-- 良好：带索引的外键、适当的约束
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_created_at ON users(created_at DESC);

CREATE TABLE posts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(500) NOT NULL,
    content TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 为连接索引外键
CREATE INDEX idx_posts_user_id ON posts(user_id);

-- 为常见查询模式建立部分索引
CREATE INDEX idx_posts_published 
ON posts(published_at DESC) 
WHERE status = 'published';

-- 为过滤+排序建立复合索引
CREATE INDEX idx_posts_status_created 
ON posts(status, created_at DESC);
```

2. **使用 EXPLAIN 的查询优化**
```sql
-- ❌ 错误：N+1 查询模式
SELECT * FROM posts WHERE user_id = 123;
-- 然后对每条 post：
SELECT * FROM comments WHERE post_id = ?;

-- ✅ 良好：使用 JOIN 的单次查询
EXPLAIN ANALYZE
SELECT 
    p.id, p.title, p.content,
    json_agg(json_build_object(
        'id', c.id,
        'content', c.content,
        'author', c.author
    )) as comments
FROM posts p
LEFT JOIN comments c ON c.post_id = p.id
WHERE p.user_id = 123
GROUP BY p.id;

-- 检查查询计划：
-- 关注：Seq Scan（差）、Index Scan（好）、Bitmap Heap Scan（尚可）
-- 检查：实际时间 vs. 计划时间、实际行数 vs. 估计行数
```

3. **防止 N+1 查询**
```typescript
// ❌ 错误：应用代码中的 N+1
const users = await db.query("SELECT * FROM users LIMIT 10");
for (const user of users) {
  user.posts = await db.query(
    "SELECT * FROM posts WHERE user_id = $1", 
    [user.id]
  );
}

// ✅ 良好：使用聚合的单次查询
const usersWithPosts = await db.query(`
  SELECT 
    u.id, u.email, u.name,
    COALESCE(
      json_agg(
        json_build_object('id', p.id, 'title', p.title)
      ) FILTER (WHERE p.id IS NOT NULL),
      '[]'
    ) as posts
  FROM users u
  LEFT JOIN posts p ON p.user_id = u.id
  GROUP BY u.id
  LIMIT 10
`);
```

4. **安全迁移**
```sql
-- ✅ 良好：无锁的可逆迁移
BEGIN;

-- 带默认值添加列（PostgreSQL 11+ 不重写表）
ALTER TABLE posts 
ADD COLUMN view_count INTEGER NOT NULL DEFAULT 0;

-- 并发添加索引（不锁表）
COMMIT;
CREATE INDEX CONCURRENTLY idx_posts_view_count 
ON posts(view_count DESC);

-- ❌ 错误：迁移期间锁表
ALTER TABLE posts ADD COLUMN view_count INTEGER;
CREATE INDEX idx_posts_view_count ON posts(view_count);
```

5. **连接池**
```typescript
// Supabase 连接池
import { createClient } from '@supabase/supabase-js';

const supabase = createClient(
  process.env.SUPABASE_URL!,
  process.env.SUPABASE_ANON_KEY!,
  {
    db: {
      schema: 'public',
    },
    auth: {
      persistSession: false, // 服务端
    },
  }
);

// 为 serverless 使用事务池
const pooledUrl = process.env.DATABASE_URL?.replace(
  '5432',
  '6543' // 事务模式端口
);
```
