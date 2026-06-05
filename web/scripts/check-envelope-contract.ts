/**
 * EnvelopeType contract check — verifies frontend EnvelopeType values
 * match the backend Go constants defined in internal/event/contract/envelope.go.
 *
 * Run: npx ts-node scripts/check-envelope-contract.ts
 */
import * as fs from 'fs';
import * as path from 'path';

// Backend EnvelopeType values (from internal/event/contract/envelope.go)
const BACKEND_ENVELOPE_TYPES: string[] = [
  'text_delta',
  'text_done',
  'tool_call',
  'tool_result',
  'state_delta',
  'transfer',
  'runner_completion',
  'context_usage',
  'run_status',
  'error',
  'log',
  'flow_log',
  'graph_node_start',
  'graph_node_end',
  'checkpoint',
  'intent_pass',
  'member_message_start',
  'member_delta',
  'member_message_done',
  'team_run_started',
  'team_run_finished',
  'team_step_started',
  'team_step_finished',
  'team_run_failed',
  'team_summary',
  'graph_step',
  'graph_execution_done',
  'graph_node_error',
  'graph_node_custom',
  'graph_task_status',
  'knowledge_ingest',
  'mcp.session.reconnect',
  'mcp.health.alert',
  'alert.notify',
  'orchestration_agent_status',
  'user_feedback',
  'session.status_changed',
  'spirit_team_assembled',
  'spirit_team_completed',
  'spirit_team_failed',
  'spirit_team_progress',
  'spirit_teams_all_completed',
  'spirit_synthesis_completed',
  'spirit_plan_created',
  'spirit_allocation_created',
  'spirit_orchestration_started',
  'spirit_orchestration_checkpoint',
  'spirit_orchestration_interrupted',
  'token_usage',
  'metrics_updated',
  'butler.orchestration.started',
  'butler.orchestration.completed',
  'butler.orchestration.failed',
  'skill.health_changed',
  'skill.evolution_proposed',
  'monitor.auto_healed',
  'monitor.self_check_completed',
];

function extractFrontendTypes(envelopePath: string): Set<string> {
  const content = fs.readFileSync(envelopePath, 'utf-8');
  // Extract string literal values from the EnvelopeType union type
  const regex = /'([^']+)'/g;
  const types = new Set<string>();
  let match: RegExpExecArray | null;
  while ((match = regex.exec(content)) !== null) {
    types.add(match[1]);
  }
  return types;
}

function main(): void {
  const envelopePath = path.resolve(__dirname, '../src/realtime/envelope.ts');

  if (!fs.existsSync(envelopePath)) {
    console.error(`ERROR: envelope.ts not found at ${envelopePath}`);
    process.exit(1);
  }

  const frontendTypes = extractFrontendTypes(envelopePath);
  const backendSet = new Set(BACKEND_ENVELOPE_TYPES);

  const missingInFrontend: string[] = [];
  for (const t of backendSet) {
    if (!frontendTypes.has(t)) {
      missingInFrontend.push(t);
    }
  }

  const extraInFrontend: string[] = [];
  for (const t of frontendTypes) {
    if (!backendSet.has(t)) {
      extraInFrontend.push(t);
    }
  }

  let hasError = false;

  if (missingInFrontend.length > 0) {
    console.error('ERROR: Frontend is missing these EnvelopeType values (defined in backend but not in frontend):');
    for (const t of missingInFrontend) {
      console.error(`  - ${t}`);
    }
    hasError = true;
  }

  if (extraInFrontend.length > 0) {
    console.warn('WARN: Frontend has extra EnvelopeType values not in backend:');
    for (const t of extraInFrontend) {
      console.warn(`  + ${t}`);
    }
  }

  if (!hasError) {
    console.log(`OK: Frontend EnvelopeType contract aligned with backend (${frontendTypes.size} types)`);
  }

  process.exit(hasError ? 1 : 0);
}

main();
