// web/src/components/chat/__tests__/ExecutionReportCard.spec.ts
// B.10.17 任务执行总结报告：ExecutionReportCard 四板块渲染 + NoticeBlock 分支。
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import ExecutionReportCard from '../ExecutionReportCard.vue';
import NoticeBlock from '../NoticeBlock.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import { parseExecutionReport } from '../../../features/chat/executionReport';
import type { Step } from '../../../features/chat/v2Types';

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': zhCN },
});

/** Quasar stubs — label props must render as text (same pattern as MemberSessionPanel.spec). */
const quasarStubs = {
  'q-chip': { props: ['label'], template: '<span class="q-chip-stub">{{ label }}</span>' },
  'q-icon': { template: '<i />' },
  'q-expansion-item': {
    props: ['label'],
    template: '<div class="q-expansion-stub"><div class="q-expansion-stub__label">{{ label }}</div><slot /></div>',
  },
};

function mkEnvelope(over: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    version: 1,
    kind: 'execution_report',
    content: '## 分析结论\n一切正常。',
    strategy: 'hybrid',
    degraded: false,
    overview: {
      query: '调研量子计算并输出报告',
      final_status: 'completed',
      duration_ms: 12300,
      total_units: 2,
      completed_units: 2,
      failed_units: 0,
      token_in: 1000,
      token_out: 2000,
    },
    team_results: [
      {
        team_id: 'team-1',
        team_name: '调研团队',
        task_name: '任务A',
        status: 'completed',
        summary: '完成资料收集',
        key_findings: '',
        duration_ms: 8000,
        error_message: '',
      },
      {
        team_id: 'team-2',
        team_name: '分析团队',
        task_name: '任务B',
        status: 'failed',
        summary: '',
        key_findings: '',
        error_message: '模型超时',
      },
    ],
    deliverables: [
      {
        node_id: 'st_1',
        unit_name: '调研团队',
        summary: '调研报告摘要',
        type: 'document',
        format: 'markdown',
        size_chars: 500,
      },
    ],
    synthesized_at: '2026-07-22T10:00:00Z',
    ...over,
  };
}

function mkStep(over: Partial<Step> = {}): Step {
  return {
    ID: 's1',
    TurnID: 't1',
    TaskID: 'tk1',
    SessionID: 'sess-1',
    SpiritSessionID: 'spirit-1',
    Kind: 'notice',
    AuthorAgentKey: 'spirit-synthesis',
    Seq: 1,
    Version: 1,
    Content: '',
    Reasoning: '',
    ToolName: '',
    ToolCallID: '',
    ToolArgs: null,
    ToolResult: null,
    ToolDurationMs: 0,
    ToolErrorCode: '',
    NoticeType: '',
    Status: 'completed',
    IsFinal: false,
    StartedAt: '',
    CompletedAt: null,
    ...over,
  };
}

function mountCard(envelope: Record<string, unknown>) {
  const report = parseExecutionReport(JSON.stringify(envelope));
  expect(report).not.toBeNull();
  return mount(ExecutionReportCard, {
    props: { report: report!, stepId: 's1' },
    global: { plugins: [i18n], stubs: quasarStubs },
  });
}

describe('ExecutionReportCard', () => {
  it('renders four sections: header/overview, analysis, team results, deliverables', () => {
    const wrapper = mountCard(mkEnvelope());
    const text = wrapper.text();
    expect(text).toContain('任务执行总结报告');
    expect(text).toContain('已完成');
    expect(text).toContain('2/2');
    expect(text).toContain('12.3s');
    expect(text).toContain('调研量子计算并输出报告');
    expect(text).toContain('token 1.0k↑/2.0k↓');
    // analysis markdown rendered via v-html
    expect(wrapper.html()).toContain('分析结论');
    // team results rows
    expect(text).toContain('调研团队');
    expect(text).toContain('任务A');
    expect(text).toContain('8.0s');
    expect(text).toContain('完成资料收集');
    expect(text).toContain('分析团队');
    expect(text).toContain('模型超时');
    // deliverables rows
    expect(text).toContain('st_1');
    expect(text).toContain('markdown');
    expect(text).toContain('500 字');
    expect(text).toContain('调研报告摘要');
  });

  it('maps partial_failure to warning status label', () => {
    const env = mkEnvelope();
    (env.overview as Record<string, unknown>).final_status = 'partial_failure';
    const wrapper = mountCard(env);
    expect(wrapper.text()).toContain('部分失败');
    expect(wrapper.find('.execution-report-card--partial_failure').exists()).toBe(true);
  });

  it('maps failed to negative status label', () => {
    const env = mkEnvelope();
    (env.overview as Record<string, unknown>).final_status = 'failed';
    const wrapper = mountCard(env);
    expect(wrapper.text()).toContain('失败');
    expect(wrapper.find('.execution-report-card--failed').exists()).toBe(true);
  });

  it('shows degraded hint and skips empty analysis content', () => {
    const wrapper = mountCard(mkEnvelope({ degraded: true, content: '' }));
    expect(wrapper.text()).toContain('智能分析结论生成失败');
    expect(wrapper.find('.execution-report-card__content').exists()).toBe(false);
  });

  it('hides deliverables section when empty', () => {
    const wrapper = mountCard(mkEnvelope({ deliverables: [] }));
    expect(wrapper.find('.execution-report-card__deliverable').exists()).toBe(false);
  });

  it('falls back to -- when unit duration is missing', () => {
    const env = mkEnvelope();
    const results = env.team_results as Array<Record<string, unknown>>;
    delete results[0].duration_ms;
    const wrapper = mountCard(env);
    expect(wrapper.text()).toContain('--');
  });
});

describe('parseExecutionReport', () => {
  it('parses a valid envelope', () => {
    const report = parseExecutionReport(JSON.stringify(mkEnvelope()));
    expect(report).not.toBeNull();
    expect(report!.overview?.finalStatus).toBe('completed');
    expect(report!.teamResults).toHaveLength(2);
    expect(report!.teamResults[1].errorMessage).toBe('模型超时');
    expect(report!.deliverables).toHaveLength(1);
  });

  it('returns null for non-JSON content', () => {
    expect(parseExecutionReport('plain notice text')).toBeNull();
  });

  it('returns null for wrong kind', () => {
    expect(parseExecutionReport(JSON.stringify({ kind: 'other' }))).toBeNull();
  });

  it('tolerates missing optional fields', () => {
    const report = parseExecutionReport(JSON.stringify({ kind: 'execution_report' }));
    expect(report).not.toBeNull();
    expect(report!.overview).toBeNull();
    expect(report!.teamResults).toEqual([]);
    expect(report!.deliverables).toEqual([]);
  });
});

describe('NoticeBlock execution-report branch', () => {
  it('renders ExecutionReportCard for synthesis_completed with valid envelope', () => {
    const wrapper = mount(NoticeBlock, {
      props: {
        step: mkStep({
          NoticeType: 'synthesis_completed',
          Content: JSON.stringify(mkEnvelope()),
        }),
      },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    expect(wrapper.find('.execution-report-card').exists()).toBe(true);
    expect(wrapper.find('.notice-block').exists()).toBe(false);
  });

  it('falls back to default notice for synthesis_completed with invalid JSON', () => {
    const wrapper = mount(NoticeBlock, {
      props: {
        step: mkStep({ NoticeType: 'synthesis_completed', Content: 'not json at all' }),
      },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    expect(wrapper.find('.execution-report-card').exists()).toBe(false);
    expect(wrapper.find('.notice-block').exists()).toBe(true);
  });

  it('renders default notice for other notice types even with envelope-shaped content', () => {
    const wrapper = mount(NoticeBlock, {
      props: {
        step: mkStep({ NoticeType: 'info', Content: JSON.stringify(mkEnvelope()) }),
      },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    expect(wrapper.find('.execution-report-card').exists()).toBe(false);
    expect(wrapper.find('.notice-block').exists()).toBe(true);
  });
});
