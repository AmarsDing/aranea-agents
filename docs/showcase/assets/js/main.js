/* ═══════════════ Aranea Agents Showcase — main.js ═══════════════ */
(function () {
  'use strict';

  /* ───────── 1. Hero 粒子网络 ───────── */
  const canvas = document.getElementById('particles');
  if (canvas) {
    const ctx = canvas.getContext('2d');
    let W, H, pts = [];
    const N = 72, LINK = 150;
    function resize() {
      W = canvas.width = canvas.offsetWidth;
      H = canvas.height = canvas.offsetHeight;
    }
    function init() {
      resize();
      pts = Array.from({ length: N }, () => ({
        x: Math.random() * W, y: Math.random() * H,
        vx: (Math.random() - .5) * .35, vy: (Math.random() - .5) * .35,
        r: Math.random() * 1.6 + .6
      }));
    }
    function tick() {
      ctx.clearRect(0, 0, W, H);
      for (const p of pts) {
        p.x += p.vx; p.y += p.vy;
        if (p.x < 0 || p.x > W) p.vx *= -1;
        if (p.y < 0 || p.y > H) p.vy *= -1;
        ctx.beginPath();
        ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2);
        ctx.fillStyle = 'rgba(77,216,232,.55)';
        ctx.fill();
      }
      for (let i = 0; i < N; i++) {
        for (let j = i + 1; j < N; j++) {
          const dx = pts[i].x - pts[j].x, dy = pts[i].y - pts[j].y;
          const d = Math.hypot(dx, dy);
          if (d < LINK) {
            ctx.beginPath();
            ctx.moveTo(pts[i].x, pts[i].y);
            ctx.lineTo(pts[j].x, pts[j].y);
            ctx.strokeStyle = 'rgba(77,216,232,' + (0.14 * (1 - d / LINK)) + ')';
            ctx.lineWidth = 1;
            ctx.stroke();
          }
        }
      }
      requestAnimationFrame(tick);
    }
    init(); tick();
    window.addEventListener('resize', init);
  }

  /* ───────── 2. 滚动渐入 ───────── */
  const revealIO = new IntersectionObserver((entries) => {
    entries.forEach(e => { if (e.isIntersecting) { e.target.classList.add('revealed'); revealIO.unobserve(e.target); } });
  }, { threshold: 0.12 });
  document.querySelectorAll('.reveal').forEach(el => revealIO.observe(el));

  /* ───────── 3. 数字滚动 ───────── */
  const countIO = new IntersectionObserver((entries) => {
    entries.forEach(e => {
      if (!e.isIntersecting) return;
      countIO.unobserve(e.target);
      const target = +e.target.dataset.count, dur = 1600, t0 = performance.now();
      (function step(t) {
        const k = Math.min((t - t0) / dur, 1);
        e.target.textContent = Math.round(target * (1 - Math.pow(1 - k, 3)));
        if (k < 1) requestAnimationFrame(step);
      })(t0);
    });
  }, { threshold: 0.6 });
  document.querySelectorAll('.stat-num').forEach(el => countIO.observe(el));

  /* ───────── 4. 数据流节点依次点亮 ───────── */
  document.querySelectorAll('.chain').forEach(chain => {
    const nodes = chain.querySelectorAll('.chain-node');
    let idx = 0;
    const chainIO = new IntersectionObserver((es) => {
      if (!es[0].isIntersecting) return;
      chainIO.disconnect();
      setInterval(() => {
        nodes.forEach(n => n.classList.remove('lit'));
        nodes[idx % nodes.length].classList.add('lit');
        if (idx % nodes.length === nodes.length - 1) {
          // 末节点停留后全亮一轮
          setTimeout(() => nodes.forEach(n => n.classList.add('lit')), 700);
        }
        idx++;
      }, 1300);
    }, { threshold: 0.4 });
    chainIO.observe(chain);
  });

  /* ───────── 5. 模块总览数据渲染 ───────── */
  const ICONS = {
    dashboard: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="3" y="3" width="8" height="8" rx="2"/><rect x="13" y="3" width="8" height="5" rx="2"/><rect x="13" y="10" width="8" height="11" rx="2"/><rect x="3" y="13" width="8" height="8" rx="2"/></svg>',
    receipt: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M5 3h14v18l-2.5-1.6L14 21l-2.5-1.6L9 21l-2-1.3L5 21z"/><path d="M9 8h6M9 12h6"/></svg>',
    chat: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M21 12a8 8 0 0 1-8 8H4l2-3a8 8 0 1 1 15-5z"/><path d="M8.5 11h.01M12 11h.01M15.5 11h.01" stroke-linecap="round" stroke-width="2.2"/></svg>',
    history: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3.5 2" stroke-linecap="round"/></svg>',
    brain: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M12 4a4 4 0 0 0-4 4 4 4 0 0 0-3 6.5A4 4 0 0 0 8 20a4 4 0 0 0 4-2 4 4 0 0 0 4 2 4 4 0 0 0 3-5.5A4 4 0 0 0 16 8a4 4 0 0 0-4-4z"/><path d="M12 4v14"/></svg>',
    robot: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="5" y="8" width="14" height="10" rx="3"/><path d="M12 8V4M12 4h4" stroke-linecap="round"/><circle cx="9.5" cy="13" r="1" fill="currentColor"/><circle cx="14.5" cy="13" r="1" fill="currentColor"/></svg>',
    org: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="9" y="3" width="6" height="5" rx="1.5"/><rect x="3" y="15" width="6" height="5" rx="1.5"/><rect x="15" y="15" width="6" height="5" rx="1.5"/><path d="M12 8v3m0 0H6v4m6-4h6v4"/></svg>',
    team: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="8" cy="8" r="3"/><circle cx="16.5" cy="9.5" r="2.5"/><path d="M3 20c0-3 2.5-5 5-5s5 2 5 5M13.5 19c.3-2.4 1.8-4 3.5-4s3 1.6 3.5 4"/></svg>',
    graph: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="5" cy="6" r="2.5"/><circle cx="19" cy="6" r="2.5"/><circle cx="12" cy="18" r="2.5"/><path d="M7 7.5l3.5 8M17 7.5l-3.5 8M7.5 6h9"/></svg>',
    model: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="4" y="4" width="16" height="16" rx="3"/><path d="M9 2v2m6-2v2M9 20v2m6-2v2M2 9h2m-2 6h2m16-6h2m-2 6h2"/><circle cx="12" cy="12" r="3"/></svg>',
    channel: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M4 6h16M4 6v12h16V6" /><path d="M4 6l8 7 8-7"/></svg>',
    mcp: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M8 9l-4 3 4 3m8-6l4 3-4 3" stroke-linecap="round" stroke-linejoin="round"/><path d="M13 5l-2 14" stroke-linecap="round"/></svg>',
    tool: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M14.5 6.5a4 4 0 0 0-5.6 4.9L4 16.3V20h3.7l4.9-4.9a4 4 0 0 0 4.9-5.6L14.7 12l-2.7-2.7z"/></svg>',
    skill: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M12 2l2.4 5.3L20 8.2l-4 4.1.9 5.7L12 15.4 7.1 18l.9-5.7-4-4.1 5.6-.9z" stroke-linejoin="round"/></svg>',
    evolve: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M12 3v3m0 12v3M3 12h3m12 0h3" stroke-linecap="round"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1.4" fill="currentColor"/></svg>',
    report: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M6 3h9l4 4v14H6z"/><path d="M14 3v5h5M9 13h6M9 17h6"/></svg>',
    plugin: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M9 4v4H5v5h4v4h5v-4h4V8h-4V4z" stroke-linejoin="round"/></svg>',
    hook: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1.5 1.5" stroke-linecap="round"/><path d="M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7L12.5 19.5" stroke-linecap="round"/></svg>',
    webhook: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M10.2 6.3a4 4 0 1 1 5.6 5L13 14" stroke-linecap="round"/><path d="M13.8 17.7a4 4 0 1 1-5.6-5L11 10" stroke-linecap="round"/></svg>',
    a2a: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M7 16l-4-4 4-4m10 8l4-4-4-4" stroke-linecap="round" stroke-linejoin="round"/><path d="M3 12h18" stroke-dasharray="3 3"/></svg>',
    book: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2z"/><path d="M4 19a2 2 0 0 1 2-2h13"/></svg>',
    box: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M21 8l-9-5-9 5v8l9 5 9-5z" stroke-linejoin="round"/><path d="M3 8l9 5 9-5M12 13v8"/></svg>',
    eval: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M9 11l3 3 8-8" stroke-linecap="round" stroke-linejoin="round"/><path d="M20 12v6a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h9"/></svg>',
    observe: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6-10-6-10-6z"/><circle cx="12" cy="12" r="2.6"/></svg>',
    cron: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="12" cy="13" r="8"/><path d="M12 9v4l2.5 2.5M9 2l1.5 2M15 2l-1.5 2" stroke-linecap="round"/></svg>',
    monitor: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M3 12h4l2.5-6 4 12L16 12h5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    shop: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M4 9l1.5-5h13L20 9M4 9v11h16V9M4 9h16M9 20v-7h6v7"/></svg>',
    gear: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="12" cy="12" r="3"/><path d="M19 12a7 7 0 0 0-.1-1.2l2-1.5-2-3.4-2.3 1a7 7 0 0 0-2-1.2L14.2 3h-4l-.4 2.5a7 7 0 0 0-2 1.2l-2.3-1-2 3.4 2 1.5a7 7 0 0 0 0 2.4l-2 1.5 2 3.4 2.3-1a7 7 0 0 0 2 1.2l.4 2.5h4l.4-2.5a7 7 0 0 0 2-1.2l2.3 1 2-3.4-2-1.5c.06-.4.1-.8.1-1.2z"/></svg>'
  };

  const MODULE_GROUPS = [
    {
      name: '工作台', en: 'WORKSPACE', mods: [
        { icon: 'dashboard', name: '概览', en: 'OVERVIEW', desc: '平台忙不忙、错在哪、钱花哪了，一屏说清。', feat: 'Agent / 会话 / Token 指标聚合，根因引擎直指异常源头' },
        { icon: 'receipt', name: '用量事件', en: 'USAGE EVENTS', desc: 'Token 有没有烧超，不用月底对账才知道。', feat: '月度配额顶盖直接拦截超支会话，费用按模型 / Agent 归因到人' },
        { icon: 'chat', name: '聊天', en: 'CHAT', desc: '服务重启、任务中断，对话不必从头再来。', feat: '流式 + 思维链 + 计划 DAG 同屏渲染，中断任务一键续跑' },
        { icon: 'history', name: '会话历史', en: 'SESSIONS', desc: '上下文再长也不爆，每次交互都能倒带复盘。', feat: 'LLM 压缩长上下文 + CAS 版本守卫防写坏，时间线逐轮回放' },
        { icon: 'brain', name: '记忆中心', en: 'MEMORY', desc: '跨会话的"我记得你"，不是每次都重新认识。', feat: '事实 / 知识 / 传奇三级记忆级联，新会话开场自动注入' }
      ]
    },
    {
      name: '智能体', en: 'AGENTS', mods: [
        { icon: 'robot', name: 'Agent 管理', en: 'AGENTS', desc: '提示词写不好？让 AI 帮你改到好。', feat: '提示词 AI 精炼迭代，工具 / 技能 / 模型一站式装配' },
        { icon: 'org', name: '组织架构', en: 'ORGANIZATION', desc: 'Agent 也有编制：谁归谁管，组队自动对号入座。', feat: 'Taxonomy 树映射部门职责，团队组建按组织线选员' },
        { icon: 'team', name: 'Team 管理', en: 'TEAMS', desc: '一句话任务描述，自动拆 DAG、自动组队。', feat: '顺序 / 并行 / 生成评审 / 群智四模式，惰性组队按需启停' },
        { icon: 'graph', name: 'Graph 工作流', en: 'GRAPHS', desc: '流程跑到一半失败，不用从零重跑。', feat: '拖拽编排 DAG，人工介入节点 + 断点恢复 + 版本管理' }
      ]
    },
    {
      name: '模型与渠道', en: 'MODELS & CHANNELS', mods: [
        { icon: 'model', name: '模型管理', en: 'MODELS', desc: '某个模型挂了，业务无感切到备胎。', feat: '多 Provider 统一编目，健康检查 + 熔断降级自动切换' },
        { icon: 'channel', name: 'Channel 管理', en: 'CHANNELS', desc: '飞书消息进来，自动找到该干活的 Agent。', feat: '飞书长连接接入，routing 规则把消息路由给指定 Agent' }
      ]
    },
    {
      name: '工具与集成', en: 'TOOLS & INTEGRATIONS', mods: [
        { icon: 'mcp', name: 'MCP 管理', en: 'MCP', desc: '外部工具生态接进来，凭据还要管得住。', feat: '标准 MCP 协议接入，用户级凭据分级托管互相隔离' },
        { icon: 'tool', name: '工具管理', en: 'TOOLS', desc: '危险操作必须人点头，授权一次不烦第二次。', feat: '确认门拦截高危调用 + 会话级授权，执行全链路留痕' },
        { icon: 'skill', name: 'Skill 管理', en: 'SKILLS', desc: '一次成功任务，自动沉淀成人人可复用的 Skill。', feat: '从执行轨迹提炼步骤与工具调用序列，LLM 总结成 SKILL.md 审核入库' },
        { icon: 'evolve', name: '进化建议', en: 'EVOLUTION', desc: 'Agent 不是上线即巅峰，而是越用越强。', feat: '学习循环分析历史成败模式（置信度 ≥0.15 立项），LLM 写提案、人审后生效' },
        { icon: 'report', name: '经验报告', en: 'EXPERIENCE', desc: '哪个工具好用、哪个总拖后腿，用数据说话。', feat: '成功率 50% + 频次 30% + 耗时 20% 加权评分，失败标签聚类反哺优化' },
        { icon: 'plugin', name: 'Plugin 管理', en: 'PLUGINS', desc: '扩展能力不用改代码，声明一下就插上。', feat: '声明式配置 Schema，即插即用' },
        { icon: 'hook', name: 'Hook / 回调', en: 'HOOKS', desc: '回调投递失败，不等于事件丢失。', feat: '投递全程追踪，失败自动重放，事件完整可达' },
        { icon: 'webhook', name: 'Webhook 管理', en: 'WEBHOOKS', desc: '外部系统的事件，直接变成 Agent 的任务。', feat: '入站事件按规则路由绑定 Agent，双向集成闭环' },
        { icon: 'a2a', name: 'A2A', en: 'AGENT-TO-AGENT', desc: '别的系统的 Agent，也能像本地一样调起来。', feat: '远程 Agent 发现 / 调用 / 审计一体化' }
      ]
    },
    {
      name: '知识与数据', en: 'KNOWLEDGE & DATA', mods: [
        { icon: 'book', name: '知识库', en: 'KNOWLEDGE', desc: 'Word、图片、PPT 扔进来，都能变成可检索的知识。', feat: '多模态统一归一化为 Markdown 再切块向量化；无 LLM 降级原文可检索' },
        { icon: 'box', name: '制品管理', en: 'ARTIFACTS', desc: 'Agent 产出的文件，链接不会 24 小时就失效。', feat: 'artifact:// 本地持久化 + 在线预览，音视频流式播放' },
        { icon: 'eval', name: '评估管理', en: 'EVALUATION', desc: 'Agent 质量好不好，让评分体系说话不靠感觉。', feat: '评估集 + LLM 自动打分，多轮结果对比一键导出' }
      ]
    },
    {
      name: '调度与运维', en: 'OPS', mods: [
        { icon: 'observe', name: '可观测性', en: 'OBSERVABILITY', desc: '线上出问题，不用登录服务器翻日志。', feat: 'Runner / 会话 / 供应商状态聚合，诊断包一键导出' },
        { icon: 'cron', name: 'Cron 调度', en: 'CRON', desc: '周期性的活儿，到点自动派给 Agent 干。', feat: 'Cron 表达式驱动周期执行，每次运行历史可查' },
        { icon: 'monitor', name: '监控日志', en: 'MONITOR', desc: '日志不是事后翻的，是实时推着看的。', feat: '结构化 Pipeline 多 Sink 分发，WebSocket 秒级推送 + 调用链瀑布' },
        { icon: 'shop', name: '生态商店', en: 'ECOSYSTEM', desc: '一整套能力打包带走，导入不怕半路翻车。', feat: 'Pack 事务化原子导入，一键装配完整能力包' },
        { icon: 'gear', name: '系统设置', en: 'SETTINGS', desc: '全局开关集中管，改一处全平台生效。', feat: 'Web Research、评估 LLM 等平台级配置中枢' }
      ]
    }
  ];

  const groupsRoot = document.getElementById('moduleGroups');
  if (groupsRoot) {
    MODULE_GROUPS.forEach(g => {
      const group = document.createElement('div');
      group.className = 'module-group reveal';
      group.innerHTML =
        '<div class="module-group-head"><h3>' + g.name + '</h3>' +
        '<span class="mg-count">' + g.en + ' · ' + g.mods.length + '</span>' +
        '<div class="mg-line"></div></div>' +
        '<div class="module-grid">' +
        g.mods.map(m =>
          '<div class="module-card">' +
          '<div class="module-card-top"><div class="module-icon">' + ICONS[m.icon] + '</div>' +
          '<div><div class="module-name">' + m.name + '</div><div class="module-en">' + m.en + '</div></div></div>' +
          '<div class="module-desc">' + m.desc + '</div>' +
          '<div class="module-feat">' + m.feat + '</div>' +
          '</div>'
        ).join('') +
        '</div>';
      groupsRoot.appendChild(group);
      revealIO.observe(group);
    });
    // 卡片跟随鼠标的微光
    document.querySelectorAll('.module-card').forEach(card => {
      card.addEventListener('mousemove', e => {
        const r = card.getBoundingClientRect();
        card.style.setProperty('--mx', ((e.clientX - r.left) / r.width * 100) + '%');
        card.style.setProperty('--my', ((e.clientY - r.top) / r.height * 100) + '%');
      });
    });
  }

  /* ───────── 6. 运镜画廊 ───────── */
  const CANVAS_W = 1180, CANVAS_H = 708;

  const GALLERY_FEATURES = {
    chat: [
      { cam: 'agents', zoom: 1.9, title: 'Agent 列表', desc: '按名称搜索 Agent / Team，在线 · 空闲 · 已完成状态徽标实时刷新' },
      { cam: 'plan', zoom: 2.4, title: '计划 DAG 条', desc: '精灵拆解的执行计划，阶段状态随执行逐一点亮' },
      { cam: 'runcard', zoom: 1.9, title: '团队运行卡', desc: '成员状态 chips + 进度条 + 耗时，内嵌消息流不跳页' },
      { cam: 'sessions', zoom: 1.9, title: 'SESSION 面板', desc: '今日会话按时间归档，进度条直览每个会话执行进度' },
      { cam: 'composer', zoom: 2.4, title: '输入合成器', desc: '对话模式与模型随选随切，@ 即可提及 Agent' }
    ],
    team: [
      { cam: 'dag', zoom: 2.4, title: '阶段 DAG 条', desc: '阶段按 DAG 流转：已完成绿色收敛，执行中呼吸高亮' },
      { cam: 'stage1', zoom: 2.6, title: '已完成阶段', desc: '成员耗时与执行轨迹清晰可溯' },
      { cam: 'stage2', zoom: 2.3, title: '并行阶段', desc: '并行成员独立状态灯，阶段进度条实时推进' },
      { cam: 'members', zoom: 1.6, title: '成员会话面板', desc: '思考 / 行动 / 回复流内嵌展开，无需跳转页面' },
      { cam: 'input', zoom: 2.4, title: '双功能输入栏', desc: '运行中可 ⏹ 暂停 Agent，输入文本即变 ➤ 注入指令' }
    ],
    graph: [
      { cam: 'palette', zoom: 1.8, title: '组件库', desc: '智能体 / 控制流分组拖入画布，人工干预节点琥珀色警示' },
      { cam: 'gcanvas', zoom: 1.35, title: '编排画布', desc: '点阵网格 + 紫色函数节点 + 流动贝塞尔连线，小地图总览全局' },
      { cam: 'props', zoom: 1.8, title: 'GRAPH 属性', desc: '入口 / 结束节点、执行引擎、检查点与版本集中配置' }
    ],
    monitor: [
      { cam: 'metrics', zoom: 2.2, title: '运行指标卡', desc: '活跃 Agent、团队数量、今日调用与告警一屏直览' },
      { cam: 'tabs', zoom: 2.6, title: '统一观测入口', desc: 'Usage / Alerts / Audit / Events / Traces / Logs 六视图切换' },
      { cam: 'logs', zoom: 1.6, title: '实时日志流', desc: '结构化日志按级别着色，WebSocket 秒级推送' },
      { cam: 'trace', zoom: 1.6, title: '调用链瀑布', desc: '每次执行耗时瀑布呈现，瓶颈定位一眼可见' }
    ]
  };

  function offsetInCanvas(el, canvasEl) {
    let x = 0, y = 0, node = el;
    while (node && node !== canvasEl) {
      x += node.offsetLeft; y += node.offsetTop;
      node = node.offsetParent;
    }
    return { x, y, w: el.offsetWidth, h: el.offsetHeight };
  }

  document.querySelectorAll('.gallery').forEach(galleryEl => {
    const key = galleryEl.dataset.gallery;
    const features = GALLERY_FEATURES[key];
    if (!features) return;

    const viewport = galleryEl.querySelector('.stage-viewport');
    const canvasEl = galleryEl.querySelector('.stage-canvas');
    const spotlight = galleryEl.querySelector('.spotlight');
    const listEl = galleryEl.querySelector('.feature-list');
    const capText = galleryEl.querySelector('.cap-text');

    // 渲染功能清单
    features.forEach((f, i) => {
      const li = document.createElement('li');
      li.innerHTML = '<b>' + (i + 1) + '. ' + f.title + '</b><span>' + f.desc + '</span>';
      li.addEventListener('click', () => { select(i, true); });
      listEl.appendChild(li);
    });
    const items = listEl.querySelectorAll('li');

    // 预计算各目标在画布中的未变换坐标
    let rects = [];
    function measure() {
      const prev = canvasEl.style.transform;
      canvasEl.style.transition = 'none';
      canvasEl.style.transform = 'none';
      rects = features.map(f => {
        const el = canvasEl.querySelector('[data-cam="' + f.cam + '"]');
        return el ? offsetInCanvas(el, canvasEl) : null;
      });
      canvasEl.style.transform = prev;
      canvasEl.style.transition = '';
    }

    let current = -1, timer = null, paused = false;

    function apply(i) {
      const r = rects[i];
      if (!r) return;
      const vpW = viewport.clientWidth, vpH = viewport.clientHeight;
      const base = vpW / CANVAS_W;
      // 目标必须完整落入视口：zoom 受目标宽高约束
      const fit = Math.min(
        features[i].zoom,
        (vpW * 0.84) / (r.w * base),
        (vpH * 0.8) / (r.h * base)
      );
      const s = base * Math.max(fit, 1);
      const cx = r.x + r.w / 2, cy = r.y + r.h / 2;
      let tx = vpW / 2 - cx * s, ty = vpH / 2 - cy * s;
      // clamp：画布边缘不留白
      tx = Math.min(0, Math.max(vpW - CANVAS_W * s, tx));
      ty = Math.min(0, Math.max(vpH - CANVAS_H * s, ty));
      canvasEl.style.transform = 'translate(' + tx + 'px,' + ty + 'px) scale(' + s + ')';
      // 聚光灯（视口坐标系）
      const pad = 8;
      spotlight.style.left = (r.x * s + tx - pad) + 'px';
      spotlight.style.top = (r.y * s + ty - pad) + 'px';
      spotlight.style.width = (r.w * s + pad * 2) + 'px';
      spotlight.style.height = (r.h * s + pad * 2) + 'px';
      spotlight.classList.add('on');
      items.forEach((li, j) => li.classList.toggle('active', j === i));
      capText.textContent = features[i].title + ' — ' + features[i].desc;
    }

    function overview() {
      const vpW = viewport.clientWidth;
      const s = vpW / CANVAS_W;
      canvasEl.style.transform = 'scale(' + s + ')';
      spotlight.classList.remove('on');
      items.forEach(li => li.classList.remove('active'));
      capText.textContent = '全景视角 — 镜头即将运镜至功能点…';
    }

    function select(i, manual) {
      current = i;
      apply(i);
      if (manual) restart();
    }

    function next() { select((current + 1) % features.length, false); }

    function restart() {
      clearInterval(timer);
      timer = setInterval(() => { if (!paused) next(); }, 4200);
    }

    galleryEl.addEventListener('mouseenter', () => { paused = true; });
    galleryEl.addEventListener('mouseleave', () => { paused = false; });

    window.addEventListener('resize', () => {
      measure();
      if (current >= 0) apply(current); else overview();
    });

    // 进入视口后启动
    const galleryIO = new IntersectionObserver((es) => {
      if (!es[0].isIntersecting) return;
      galleryIO.disconnect();
      measure();
      overview();
      setTimeout(() => { select(0, false); restart(); }, 1600);
    }, { threshold: 0.35 });
    galleryIO.observe(galleryEl);
  });
})();
