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
        { icon: 'dashboard', name: '概览', en: 'OVERVIEW', desc: '平台运行态势总览大屏，核心指标与系统健康一屏尽览。', feat: 'Agent / 会话 / Token 指标聚合，状态面板实时刷新' },
        { icon: 'receipt', name: '用量事件', en: 'USAGE EVENTS', desc: 'Token 消耗与费用明细的全量追踪流水。', feat: '按模型 / Agent 多维归因，异常用量自动预警' },
        { icon: 'chat', name: '聊天', en: 'CHAT', desc: '多智能体对话主战场，人与 Agent 的协同界面。', feat: '流式输出 + 思维链 + 计划 DAG 同屏实时渲染' },
        { icon: 'history', name: '会话历史', en: 'SESSIONS', desc: '全量会话归档检索，每一次交互都可回放。', feat: '时间线还原交互轮次与工具调用细节' },
        { icon: 'brain', name: '记忆中心', en: 'MEMORY', desc: '智能体的长期记忆中枢，跨会话的知识沉淀。', feat: '事实 / 知识 / 传奇三级记忆级联，图谱化浏览' }
      ]
    },
    {
      name: '智能体', en: 'AGENTS', mods: [
        { icon: 'robot', name: 'Agent 管理', en: 'AGENTS', desc: '智能体的创建、装配与配置中心。', feat: '提示词 / 工具 / 技能一站式装配，AI 辅助精炼' },
        { icon: 'org', name: '组织架构', en: 'ORGANIZATION', desc: '部门与职责的数字化映射。', feat: 'Taxonomy 树定义 Agent 的组织位置与汇报线' },
        { icon: 'team', name: 'Team 管理', en: 'TEAMS', desc: '多智能体协作团队的编排与运行。', feat: '顺序 / 并行 / 生成评审 / 群智四模式，模板化组队' },
        { icon: 'graph', name: 'Graph 工作流', en: 'GRAPHS', desc: '可视化 DAG 流程编排与执行观测。', feat: '拖拽连线定义节点流转，支持人工介入节点' }
      ]
    },
    {
      name: '模型与渠道', en: 'MODELS & CHANNELS', mods: [
        { icon: 'model', name: '模型管理', en: 'MODELS', desc: 'Provider / Model 可用清单与校验来源。', feat: '多供应商统一编目，健康度检查与降级策略' },
        { icon: 'channel', name: 'Channel 管理', en: 'CHANNELS', desc: '外部消息渠道的接入与路由配置。', feat: '飞书 / Webhook 凭据托管，消息路由绑定 Agent' }
      ]
    },
    {
      name: '工具与集成', en: 'TOOLS & INTEGRATIONS', mods: [
        { icon: 'mcp', name: 'MCP 管理', en: 'MCP', desc: 'MCP 服务器接入网关，连接外部工具生态。', feat: '标准协议接入，用户级凭据分级托管' },
        { icon: 'tool', name: '工具管理', en: 'TOOLS', desc: '内置与自定义工具的统一目录。', feat: 'Schema 可视化编辑，运行审计全链路留痕' },
        { icon: 'skill', name: 'Skill 管理', en: 'SKILLS', desc: '可复用技能资产库，Agent 的能力积木。', feat: '版本化管理，健康度与运行统计闭环' },
        { icon: 'evolve', name: '进化建议', en: 'EVOLUTION', desc: 'Agent 自我进化引擎，越用越强。', feat: '从运行经验提炼技能改进建议，审核后生效' },
        { icon: 'report', name: '经验报告', en: 'EXPERIENCE', desc: '运行经验的结构化沉淀与复盘。', feat: '失败标签聚类分析，反哺技能与提示词优化' },
        { icon: 'plugin', name: 'Plugin 管理', en: 'PLUGINS', desc: '平台能力的扩展插件体系。', feat: '声明式配置 Schema，即插即用' },
        { icon: 'hook', name: 'Hook / 回调', en: 'HOOKS', desc: '事件驱动的生命周期回调机制。', feat: '钩子 + 投递追踪，失败可重放' },
        { icon: 'webhook', name: 'Webhook 管理', en: 'WEBHOOKS', desc: '外部系统事件的入站口。', feat: '入站事件路由到 Agent，打通双向集成' },
        { icon: 'a2a', name: 'A2A', en: 'AGENT-TO-AGENT', desc: '跨系统 Agent 互联协议网关。', feat: '远程 Agent 发现、调用与审计一体化' }
      ]
    },
    {
      name: '知识与数据', en: 'KNOWLEDGE & DATA', mods: [
        { icon: 'book', name: '知识库', en: 'KNOWLEDGE', desc: '文档摄取与语义检索管线。', feat: '多模态抽取归一化为 Markdown，向量化索引' },
        { icon: 'box', name: '制品管理', en: 'ARTIFACTS', desc: 'Agent 产出物的统一仓库。', feat: '文件制品全生命周期管理与在线预览' },
        { icon: 'eval', name: '评估管理', en: 'EVALUATION', desc: '智能体质量的科学化度量。', feat: '评估集 + 自动打分 + 结果对比导出' }
      ]
    },
    {
      name: '调度与运维', en: 'OPS', mods: [
        { icon: 'observe', name: '可观测性', en: 'OBSERVABILITY', desc: '运行全景的观测仪表盘。', feat: 'Runner / 会话 / 供应商状态实时聚合' },
        { icon: 'cron', name: 'Cron 调度', en: 'CRON', desc: '定时任务驱动的自动化执行。', feat: 'Cron 表达式驱动 Agent 周期执行，历史可查' },
        { icon: 'monitor', name: '监控日志', en: 'MONITOR', desc: '结构化日志流水线与调用链追踪。', feat: '实时日志流 + 调用链瀑布 + 审计追溯' },
        { icon: 'shop', name: '生态商店', en: 'ECOSYSTEM', desc: '能力包的分发与装配市场。', feat: 'Pack 导入导出，一键装配完整能力' },
        { icon: 'gear', name: '系统设置', en: 'SETTINGS', desc: '平台级参数的配置中枢。', feat: 'Web Research、评估 LLM 等全局配置' }
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
      { cam: 'sessions', zoom: 2.1, title: '会话树侧栏', desc: '多会话并行管理，标题自动取自首条用户消息' },
      { cam: 'plan', zoom: 2.3, title: '执行计划 DAG', desc: '精灵拆解的任务计划，步骤状态随执行实时点亮' },
      { cam: 'stream', zoom: 1.75, title: '活动消息流', desc: '思考 / 行动 / 回复分级渲染，流式光标逐字输出' },
      { cam: 'team', zoom: 2.3, title: '团队成员面板', desc: '成员状态实时聚合，运行中头像呼吸灯提醒' },
      { cam: 'composer', zoom: 2.6, title: '输入合成器', desc: '@ 提及 Agent，全宽 / 紧凑双模式自由切换' }
    ],
    team: [
      { cam: 'stage1', zoom: 2.5, title: '顺序阶段', desc: '已完成阶段绿色收敛，执行轨迹清晰可溯' },
      { cam: 'stage2', zoom: 2.0, title: '并行阶段', desc: '并行层宽度自适应，成员独立状态灯' },
      { cam: 'runcard', zoom: 2.3, title: '成员运行卡片', desc: '内嵌展开成员会话，进度条呼吸动画，无需跳页' },
      { cam: 'stage3', zoom: 2.5, title: '评审阶段', desc: '生成评审模式：产物经评审 Agent 把关后流转' }
    ],
    graph: [
      { cam: 'palette', zoom: 2.3, title: '节点面板', desc: 'Agent / 条件分支 / 人工介入 / 工具节点拖入画布' },
      { cam: 'gcanvas', zoom: 1.55, title: '编排画布', desc: '网格画布 + 流动连线，条件分支以琥珀色高亮' },
      { cam: 'props', zoom: 2.3, title: '属性面板', desc: '节点级模型、失败策略与超时的精细配置' }
    ],
    monitor: [
      { cam: 'metrics', zoom: 1.9, title: '运行指标带', desc: '活跃 Runner、事件量与告警数一目了然' },
      { cam: 'logs', zoom: 1.85, title: '实时日志流', desc: '结构化日志按级别着色，Pipeline 秒级推送' },
      { cam: 'trace', zoom: 1.85, title: '调用链瀑布', desc: '每次执行的耗时瀑布图，瓶颈定位一眼可见' }
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
