package data

import (
	"context"
	"database/sql"
	"errors"

	bizknowledge "aranea-agents/internal/biz/knowledge"

	"aranea-agents/pkg/loggateway"
)

// chunkContentKey G-32 稳定键口径（ADR-KN-20261022-I 决策 2 登记）：
// 直调 bizknowledge.HashContent（'sha1:'+hex 裸字节，
// internal/biz/knowledge/vault_filer.go）——对抗评审 CON-4：此前「data 层
// 不得 import biz」的镜像理由失实（本包 archive/search 等十余文件本就
// import biz/knowledge，且 biz/knowledge 不回指 data），公式双登记会在
// HashContent 口径演进时酿成 content_key 列与配对键静默分裂。
func chunkContentKey(content string) string {
	return bizknowledge.HashContent(content)
}

// BackfillKnowledgeG32 G-32 启动回填 runner（T1；ADR-KN-20261022-I 决策 2/10）：
//  1. knowledge_chunks.content_key 空串 行按批现算回填（配对权威恒 Go 侧现算；
//     空串 不构成配对阻塞，回填快慢只影响 (doc_id, content_key) 索引可用度）；
//  2. document_versions 空串 行对文本 doc（asset_uri=空串 且 content_text<>空串）补
//     body hash——资产类（图片/编译，决策 4 冻结语义）保持 空串 未知档，首差
//     才有版本行、免一次性伪版本。
//
// 幂等分批：UPDATE 自带 空串 护航谓词，崩溃重启断点续跑；启动路径同步执行，
// best-effort（调用方 Warn 收尾，不阻断 readiness）。种子行本身只由迁移
// 20261294 一次性纯 SQL 产生，本 runner 不建版本行（Ensure 不挂种子循环）。
func BackfillKnowledgeG32(ctx context.Context, db *sql.DB, batch int, lg loggateway.Logger) error {
	if db == nil {
		return nil
	}
	if batch <= 0 {
		batch = 200
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	chunks, err := backfillChunkContentKeys(ctx, db, batch)
	if err != nil {
		return err
	}
	versions, err := backfillVersionBodyHashes(ctx, db, batch)
	if err != nil {
		return err
	}
	if chunks > 0 || versions > 0 {
		lg.Info("knowledge G-32 backfill applied (content_key / version body-hash)",
			loggateway.StepID("data.schema.knowledge_g32_backfill"),
			loggateway.Int("chunks", chunks),
			loggateway.Int("versions", versions))
	}
	return nil
}

func backfillChunkContentKeys(ctx context.Context, db *sql.DB, batch int) (int, error) {
	total := 0
	for {
		rows, err := db.QueryContext(ctx,
			`SELECT id, content FROM knowledge_chunks WHERE content_key = '' ORDER BY id LIMIT $1`, batch)
		if err != nil {
			return total, err
		}
		type keyItem struct{ id, key string }
		var items []keyItem
		for rows.Next() {
			var id, content string
			if err := rows.Scan(&id, &content); err != nil {
				_ = rows.Close()
				return total, err
			}
			items = append(items, keyItem{id: id, key: chunkContentKey(content)})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return total, err
		}
		_ = rows.Close()
		if len(items) == 0 {
			return total, nil
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return total, err
		}
		for _, it := range items {
			if _, err := tx.ExecContext(ctx,
				`UPDATE knowledge_chunks SET content_key = $2 WHERE id = $1 AND content_key = ''`, it.id, it.key); err != nil {
				_ = tx.Rollback()
				return total, err
			}
		}
		if err := tx.Commit(); err != nil {
			return total, err
		}
		total += len(items)
		if len(items) < batch {
			return total, nil
		}
	}
}

func backfillVersionBodyHashes(ctx context.Context, db *sql.DB, batch int) (int, error) {
	total := 0
	stalledStreak := 0 // 满批连续零落盘 = 文档正被热改，连续 3 轮放弃本轮启动
	// （best-effort runner，留给下次启动收敛；防阻塞 readiness）。
	for {
		rows, err := db.QueryContext(ctx, `
			SELECT dv.doc_id, dv.version_no, d.content_text
			FROM document_versions dv
			JOIN knowledge_documents d ON d.id = dv.doc_id
			WHERE dv.content_hash = '' AND d.asset_uri = '' AND d.content_text <> ''
			ORDER BY dv.doc_id, dv.version_no LIMIT $1`, batch)
		if err != nil {
			return total, err
		}
		type verItem struct {
			docID string
			ver   int
			key   string
			body  string
		}
		var items []verItem
		for rows.Next() {
			var docID, body string
			var ver int
			if err := rows.Scan(&docID, &ver, &body); err != nil {
				_ = rows.Close()
				return total, err
			}
			items = append(items, verItem{docID: docID, ver: ver, key: chunkContentKey(body), body: body})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return total, err
		}
		_ = rows.Close()
		if len(items) == 0 {
			return total, nil
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return total, err
		}
		batchApplied := 0
		for _, it := range items {
			// DIS-3 原子护航（FOR SHARE 核身锁行，对抗评审修正版）：同 tx 内
			// 先核身【当前正文仍等于所hash正文】并 Key-Share 锁行——锁定后本 tx
			// 提交前正文不可变；谓词失配（SELECT→UPDATE 窗口漂变/此前失败续跑
			// 赶上文档换代）即 0 行落盘跳过，留空串待下轮按新正文重算。杜绝把
			// 从未对应任何版本提交态的 hash 钉死进治理账本真相源。
			// 注：不能靠 UPDATE...FROM d 的 d.content_text=$4 谓词——普通 FROM
			// 连接行不带 row mark，READ COMMITTED 的 EPQ 只对【更新目标+显式
			// FOR 锁行】重取重估，普通连接行仍是语句快照旧值，守护形同虚设。
			var one int
			herr := tx.QueryRowContext(ctx, `
				SELECT 1 FROM knowledge_documents
				WHERE id = $1 AND asset_uri = '' AND content_text = $2
				FOR SHARE`, it.docID, it.body).Scan(&one)
			if errors.Is(herr, sql.ErrNoRows) {
				continue // 漂移跳过（待下轮按新正文重算）
			}
			if herr != nil {
				_ = tx.Rollback()
				return total, herr
			}
			res, err := tx.ExecContext(ctx, `
				UPDATE document_versions SET content_hash = $3
				WHERE doc_id = $1 AND version_no = $2 AND content_hash = ''`,
				it.docID, it.ver, it.key)
			if err != nil {
				_ = tx.Rollback()
				return total, err
			}
			n, _ := res.RowsAffected()
			batchApplied += int(n)
		}
		if err := tx.Commit(); err != nil {
			return total, err
		}
		total += batchApplied
		if len(items) < batch {
			return total, nil
		}
		if batchApplied == 0 {
			stalledStreak++
			if stalledStreak >= 3 {
				return total, nil
			}
		} else {
			stalledStreak = 0
		}
	}
}
