package knowledge

// chunk_pairing.go G-32 T2 配对引擎（ADR-KN-20261022-I 决策 2/5④）：
// diff-commit 的权威配对——纯函数、零 I/O，供三处共用：
//   ① data.CommitIndexedDocument tx 内（现载行集 → DiffPlan，唯一权威）；
//   ② 服务层预配对/embed 经济化 hint（ListChunksForPairing 窄端口喂同款输入，
//     允许与 tx 权威结果不一致——最坏后果=该块进 pending-reembed 口径自愈）；
//   ③ retryDocEmbeddings 寻址（现载行→同一函数→解析存活 id）。
//
// 配对规则（决策 2 登记；CON-16/DIS-4/CON-17/DIS-6 对抗评审后收紧）：
//   - 键 = HashContent(Content) + '|' + Role——口径与 content_key 列同
//     （'sha1:'+hex 裸字节），但配对权威恒按现载 content 现算，不读列值；
//   - multiset：同键候选按 chunk_index 升序贪心弹出，重复内容块确定性可复现；
//   - 父子约束：child 块的可配旧行受限于其父的配对结果（ParentChunkID 必须
//     等于新父配到的旧父行 ID）；父未配对（added）→ 其全部新 child 强制 added、
//     对应旧 child 行入孤儿集（父死子随）。父约束【对称执行】：flat 新子块
//     （ParentIdx=-1/容退降级）只能收养 ParentChunkID=空串 的孤儿旧行——
//     策略降级 parent_child→flat 时，旧 parented 子行连同其父母同样全入
//     孤儿集（决策 2 父死子随的互易面；杜绝悬空父指针污染 current 表与
//     G-06 语境回填）；
//   - 角色隔离：child/parent 键分桶，绝不跨角色配对；
//   - ParentIdx = chunker SplitParentChild 输出 ParentIndex（所属父块在新集
//     中按出现序的序号，0..P-1——②③ 调用方原样透传，禁止换算成切片索引）；
//     -1/越界序号 = 无父声明（容退降级走 flat 口径，数据异常不构成提交阻塞）；
//   - 保留行 ID 不可变（T4 数据层职责，本函数只输出配对事实）——故无需
//     reparent 映射：kept 子块父指针指向的旧父行必然 kept（父子约束+flat
//     对称严禁的两侧担保，决策 5⑦ 重指子句在严格口径下恒为空操作，T4 建
//     compare-and-skip UPDATE 时勿当义务实现）；
//   - plan.Kept 顺序 = 按 NewIdx 严格升序（返回前排序——消费方可直接按
//     NewIdx 索引 newChunks，禁止依赖两轮 pass 的中间序）。

// PairingOldRow tx 现载旧行投影。
type PairingOldRow struct {
	ID            string
	Content       string
	ChunkIndex    int
	Role          string // 'child' | 'parent'
	ParentChunkID string // 空串 = 无父（flat 策略/父行自身）
}

// PairingNewChunk 摄取侧新块投影（本阶段 ID 未定，namespace 由 T4 赋）。
type PairingNewChunk struct {
	Content   string
	Role      string
	ParentIdx int // 所属父块的【父序号】= chunker SplitParentChild 输出
	// ParentIndex 原样透传（父块在新集中按出现序的序号，0..P-1）；
	// -1/越界 = 无父声明（flat 策略/容退降级）。禁止换算成切片索引——
	// 交织输出 [父0,子…,父1,子…] 下父1 的切片索引≠1，引擎自检映射。
}

// ChunkPair 一组配对事实：旧行 ↔ 新块。
type ChunkPair struct {
	OldID       string
	OldIndex    int    // 旧行 chunk_index（审计/调试）
	NewIdx      int    // 新块切片索引
	ContentHash string // 配对键的 hash 段（审计）
}

// ChunkPairPlan 配对结果。
type ChunkPairPlan struct {
	Kept    []ChunkPair // 保留对（按新块序）
	Added   []int       // 新块切片索引（配对失败/父死强制）
	Deleted []string    // 旧行 ID（孤儿，含父死子随）
}

// PairChunksForDiff 新旧 chunk 多重集配对。oldRows 顺序无关（内部按键分桶后
// 以 chunk_index 升序定序）；newChunks 必须按摄取序（chunk_index 等价序；
// 父块序号按出现序即等于 ParentIndex 语义）。
func PairChunksForDiff(oldRows []PairingOldRow, newChunks []PairingNewChunk) ChunkPairPlan {
	// 键桶：key → 旧行指针（chunk_index 升序；平手按切片序=调用方给序，稳定）。
	buckets := map[string][]int{}
	for i, r := range oldRows {
		k := pairingBucketKey(r.Content, r.Role)
		buckets[k] = append(buckets[k], i)
	}
	for _, idxs := range buckets {
		// 插入排序开销可忽略（同键重复块稀少）；保持确定性优先。
		for i := 1; i < len(idxs); i++ {
			for j := i; j > 0 &&
				(oldRows[idxs[j]].ChunkIndex < oldRows[idxs[j-1]].ChunkIndex ||
					(oldRows[idxs[j]].ChunkIndex == oldRows[idxs[j-1]].ChunkIndex && idxs[j] < idxs[j-1])); j-- {
				idxs[j], idxs[j-1] = idxs[j-1], idxs[j]
			}
		}
	}

	// 父序号 → 新集切片索引映射（ParentIdx 语义 = chunker ParentIndex 父序号；
	// 引擎自检映射，②③ 调用方无法再把父序号误透视成切片索引——DIS-6 评审
	// 项的结构性绝源。
	parentSlice := []int{}
	for i, nc := range newChunks {
		if nc.Role == "parent" {
			parentSlice = append(parentSlice, i)
		}
	}

	plan := ChunkPairPlan{}
	keptByNewIdx := map[int]ChunkPair{}
	pairedOld := map[int]bool{}

	// 第一轮：父块先行配对（父子约束的父侧事实）。
	// 第二轮（children）时父的配对结果必须可查，故先扫 parent 角色。
	for pass := 0; pass < 2; pass++ {
		for ni, nc := range newChunks {
			isParentRow := nc.Role == "parent"
			if (pass == 0) != isParentRow {
				continue
			}
			if !isParentRow && nc.ParentIdx >= 0 && nc.ParentIdx < len(parentSlice) {
				// 父子约束：父序号在有效域内——父必须已配对；可配旧 child
				// 限定 ParentChunkID == 旧父行 ID。
				pp, ok := keptByNewIdx[parentSlice[nc.ParentIdx]]
				if !ok {
					continue // 父未配对——交给下方强制 added
				}
				if oi, ok := popBucketFiltered(buckets, oldRows, pairingBucketKey(nc.Content, nc.Role), pp.OldID); ok {
					pair := ChunkPair{OldID: oldRows[oi].ID, OldIndex: oldRows[oi].ChunkIndex, NewIdx: ni, ContentHash: HashContent(nc.Content)}
					plan.Kept = append(plan.Kept, pair)
					keptByNewIdx[ni] = pair
					pairedOld[oi] = true
				}
				continue
			}
			// flat／父轮：键桶孤儿贪心——子块仅收养 ParentChunkID=空串 的旧行
			// （父死子随互易面：策略降级时旧 parented 子行不入 kept）；父行
			// ParentChunkID 恒空，语义不变。
			if oi, ok := popBucketFiltered(buckets, oldRows, pairingBucketKey(nc.Content, nc.Role), ""); ok {
				pair := ChunkPair{OldID: oldRows[oi].ID, OldIndex: oldRows[oi].ChunkIndex, NewIdx: ni, ContentHash: HashContent(nc.Content)}
				plan.Kept = append(plan.Kept, pair)
				keptByNewIdx[ni] = pair
				pairedOld[oi] = true
			}
		}
	}

	// 未配对新块 = added（父死强制 added 已并入此口径）。
	for ni := range newChunks {
		if _, ok := keptByNewIdx[ni]; !ok {
			plan.Added = append(plan.Added, ni)
		}
	}
	// 未配对旧行 = 孤儿（含父死子随——父未配对/被拒绝收养的旧 child 自然落此集）。
	for oi, r := range oldRows {
		if !pairedOld[oi] {
			plan.Deleted = append(plan.Deleted, r.ID)
		}
	}
	// Kept 按 NewIdx 严格升序承诺（头注§）——两轮 pass 的中间序是父段+子段，
	// 消费方按位索引 newChunks 的便利恒成立。
	kept := plan.Kept
	for i := 1; i < len(kept); i++ {
		for j := i; j > 0 && kept[j].NewIdx < kept[j-1].NewIdx; j-- {
			kept[j], kept[j-1] = kept[j-1], kept[j]
		}
	}
	return plan
}

func pairingBucketKey(content, role string) string {
	return HashContent(content) + "|" + role
}

// popBucketFiltered 键桶贪心弹出首个未消费旧行（已按 chunk_index 升序），
// 父指针严格等值筛选：parentID=空串 只收养 ParentChunkID 空串 的孤儿旧行
// （flat 对称严禁——策略降级时旧 parented 子行不得被收养，与其父母同入
// 孤儿集；受约束路径传旧父行 ID 限定其名下子块，即父死子随的父筛担保）。
func popBucketFiltered(buckets map[string][]int, rows []PairingOldRow, key, parentID string) (int, bool) {
	idxs, ok := buckets[key]
	if !ok {
		return -1, false
	}
	for pos, oi := range idxs {
		if oi < 0 {
			continue
		}
		if rows[oi].ParentChunkID != parentID {
			continue
		}
		idxs[pos] = -1
		buckets[key] = idxs
		return oi, true
	}
	return -1, false
}
