import { describe, expect, it } from 'vitest';
import type { CodingTask } from '../../../services/kratos/agentbridge/v1';
import type { OfficeTaskRow } from '../../office/types';
import type { Session } from '../../session/types';
import { buildHandText, isAbsPath, mergeMeta, parseMeta, splitTitle, type HandLabels } from '../handoff';
import { buildTasks, flat, needsReview, taskStatus, type Sources } from '../model';

const L: HandLabels = {
  task: '【任务】',
  require: '【要求】',
  before: '【前面做到哪】',
  mats: '【给你的资料】',
  ask: '【要你做】',
  deliver: '【做完交回】',
  tail: '说明做了什么、还剩什么。',
  attached: '（附件）',
  sep: '、',
  semi: '；',
};

describe('交接单', () => {
  it('新任务：标题、要求、资料（附件 / 本机路径）、交回什么', () => {
    const text = buildHandText(
      {
        title: '核对账单',
        body: '以银行流水为准',
        materials: [{ name: '对账单.xlsx' }, { name: '流水.pdf', path: 'E:\\a\\流水.pdf' }],
        deliver: ['一张表'],
      },
      L,
    );
    expect(text).toBe(
      [
        '【任务】核对账单',
        '【要求】以银行流水为准',
        '【给你的资料】\n1. 对账单.xlsx（附件）\n2. 流水.pdf   E:\\a\\流水.pdf',
        '【做完交回】一张表；说明做了什么、还剩什么。',
      ].join('\n'),
    );
  });

  it('交接：前面做到哪、要你做；没资料就不写资料段', () => {
    const text = buildHandText(
      {
        title: '迁移',
        before: ['Claude Code：完成', 'Codex：失败，测试没过'],
        ask: '换个方法',
        materials: [],
        deliver: [],
      },
      L,
    );
    expect(text).toContain('【前面做到哪】Claude Code：完成；Codex：失败，测试没过');
    expect(text).toContain('【要你做】换个方法');
    expect(text).not.toContain('【给你的资料】');
    expect(text.endsWith('【做完交回】说明做了什么、还剩什么。')).toBe(true);
  });

  it('第一行是标题', () => {
    expect(splitTitle('  标题 \n第二行\n第三行 ')).toEqual({ title: '标题', body: '第二行\n第三行' });
  });

  it('metadata：合并时保留别人写的键，坏 JSON 当空', () => {
    expect(parseMeta('not json')).toEqual({});
    expect(JSON.parse(mergeMeta('{"other":1,"cc_task":"x"}', { cc_reviewed: true }))).toEqual({
      other: 1,
      cc_task: 'x',
      cc_reviewed: true,
    });
  });

  it('本机绝对路径才交给编码工具', () => {
    expect(isAbsPath('E:\\aranea\\a.md')).toBe(true);
    expect(isAbsPath('/data/a.md')).toBe(true);
    expect(isAbsPath('2026/11/a.md')).toBe(false);
    expect(isAbsPath('http://x/a.md')).toBe(false);
  });
});

const session = (over: Partial<Session>): Session =>
  ({
    id: 's',
    owner_type: 'agent',
    agent_id: 'a1',
    team_id: '',
    title: '会话',
    summary: '',
    status: 'completed',
    status_reason: '',
    created_at: '2026-11-19T01:00:00Z',
    last_message_at: '2026-11-19T02:00:00Z',
    archived_at: '',
    deleted_at: '',
    metadata_json: '',
    ...over,
  }) as Session;
const src = (over: Partial<Sources>): Sources => ({
  sessions: [],
  agentNames: { a1: '精灵助手', a2: '财务部' },
  teamNames: {},
  officeTasks: [],
  codingTasks: [],
  runs: [],
  terminals: [],
  presets: [],
  workspaces: [],
  ...over,
});
const record = session({
  id: 'rec',
  title: '任务资料 · 核对账单',
  metadata_json: JSON.stringify({ cc_role: 'task_record', cc_deliver: ['sheet'] }),
});
const office = {
  id: 'o1',
  title: '核对账单',
  source: 'delegate',
  source_ref: 'rec',
  status: 'open',
} as OfficeTaskRow;

describe('会话中心新建的任务', () => {
  it('资料袋不算执行；跟进会话和从资料袋派出的编码任务挂到同一个任务，按先后排', () => {
    const tasks = buildTasks(
      src({
        sessions: [
          record,
          session({
            id: 's2',
            agent_id: 'a2',
            created_at: '2026-11-19T05:00:00Z',
            metadata_json: JSON.stringify({ cc_task: 'rec', cc_hand: '【任务】接着做' }),
          }),
        ],
        officeTasks: [office],
        codingTasks: [
          {
            id: 'c1',
            agentKey: 'codex',
            status: 'done',
            prompt: '【任务】核对账单',
            sessionId: 'rec',
            createdAt: '2026-11-19T03:00:00Z',
            origin: 'workbench',
          } as CodingTask,
        ],
      }),
    );
    expect(tasks).toHaveLength(1);
    const t = tasks[0]!;
    expect(t.id).toBe('office:o1');
    expect(t.recordId).toBe('rec');
    expect(t.anchorId).toBe('rec');
    expect(flat(t).map((e) => e.id)).toEqual(['bridge:c1', 'session:s2']);
    expect(flat(t)[1]!.hand).toBe('【任务】接着做');
    expect(flat(t)[0]!.hand).toBe('【任务】核对账单');
  });

  it('选了交回什么：最后一位做完 → 等你验收；验收后不再等', () => {
    const base = src({
      sessions: [record, session({ id: 's2', metadata_json: JSON.stringify({ cc_task: 'rec' }) })],
      officeTasks: [office],
    });
    const [t] = buildTasks(base);
    expect(needsReview(t!)).toBe(true);
    expect(taskStatus(t!)).toBe('waiting');
    const [done] = buildTasks({ ...base, officeTasks: [{ ...office, status: 'done' }] });
    expect(needsReview(done!)).toBe(false);
    expect(taskStatus(done!)).toBe('done');
  });

  it('资料袋还没有执行（没派出去）就不显示；跟进的锚点找不到就自成一个任务', () => {
    const tasks = buildTasks(
      src({
        sessions: [record, session({ id: 's3', metadata_json: JSON.stringify({ cc_task: 'gone' }) })],
      }),
    );
    expect(tasks.map((t) => t.id)).toEqual(['auto:session:s3']);
  });

  it('交给下一位挂在旧任务的承载会话下（旧的交办登记也能接着交）', () => {
    const tasks = buildTasks(
      src({
        sessions: [
          session({ id: 'carrier' }),
          session({
            id: 'next',
            agent_id: 'a2',
            created_at: '2026-11-19T06:00:00Z',
            metadata_json: JSON.stringify({ cc_task: 'carrier' }),
          }),
        ],
        officeTasks: [{ ...office, id: 'o2', source_ref: 'carrier' }],
      }),
    );
    expect(tasks).toHaveLength(1);
    expect(flat(tasks[0]!).map((e) => e.refId)).toEqual(['carrier', 'next']);
    expect(tasks[0]!.officeId).toBe('o2');
  });

  it('会话里 LLM 派的编码任务不算交接单', () => {
    const tasks = buildTasks(
      src({
        codingTasks: [{ id: 'c9', agentKey: 'claude', status: 'running', prompt: 'x', origin: 'chat' } as CodingTask],
      }),
    );
    expect(flat(tasks[0]!)[0]!.hand).toBe('');
  });
});
