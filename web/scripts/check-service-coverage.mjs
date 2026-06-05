/**
 * Service coverage check — compares backend proto services with
 * frontend createXxxService factory functions in web/src/services/index.ts.
 *
 * Run: node scripts/check-service-coverage.mjs
 */
import * as fs from 'fs';
import * as path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const PROJECT_ROOT = path.resolve(__dirname, '..');
const PROTO_DIR = path.join(PROJECT_ROOT, '..', 'api', 'kratos');
const SERVICES_INDEX = path.join(PROJECT_ROOT, 'src', 'services', 'index.ts');
const GENERATED_DIR = path.join(PROJECT_ROOT, 'src', 'services', 'kratos');

// ── 1. Collect proto service names ──────────────────────────────────
function collectProtoServices() {
  const services = new Map(); // ServiceName → proto path
  if (!fs.existsSync(PROTO_DIR)) {
    console.warn(`WARN: Proto directory not found at ${PROTO_DIR}`);
    return services;
  }
  const dirs = fs.readdirSync(PROTO_DIR, { withFileTypes: true })
    .filter((d) => d.isDirectory());

  for (const dir of dirs) {
    const v1Dir = path.join(PROTO_DIR, dir.name, 'v1');
    if (!fs.existsSync(v1Dir)) continue;
    const protoFiles = fs.readdirSync(v1Dir).filter((f) => f.endsWith('.proto'));
    for (const pf of protoFiles) {
      const content = fs.readFileSync(path.join(v1Dir, pf), 'utf-8');
      const regex = /^service\s+(\w+)\s*\{/gm;
      let match;
      while ((match = regex.exec(content)) !== null) {
        services.set(match[1], `${dir.name}/v1/${pf}`);
      }
    }
  }
  return services;
}

// ── 2. Collect generated client factory names ───────────────────────
function collectGeneratedClients() {
  const clients = new Set();
  if (!fs.existsSync(GENERATED_DIR)) {
    console.warn(`WARN: Generated directory not found at ${GENERATED_DIR}`);
    return clients;
  }
  const dirs = fs.readdirSync(GENERATED_DIR, { withFileTypes: true })
    .filter((d) => d.isDirectory());

  for (const dir of dirs) {
    const indexPath = path.join(GENERATED_DIR, dir.name, 'v1', 'index.ts');
    if (!fs.existsSync(indexPath)) continue;
    const content = fs.readFileSync(indexPath, 'utf-8');
    const regex = /export function (create\w+ServiceClient)/g;
    let match;
    while ((match = regex.exec(content)) !== null) {
      clients.add(match[1]);
    }
  }
  return clients;
}

// ── 3. Collect frontend factory function names ──────────────────────
function collectFrontendFactories() {
  const factories = new Set();
  if (!fs.existsSync(SERVICES_INDEX)) {
    console.error(`ERROR: services/index.ts not found at ${SERVICES_INDEX}`);
    process.exit(1);
  }
  const content = fs.readFileSync(SERVICES_INDEX, 'utf-8');
  const regex = /export function (create\w+Service)\s*\(/g;
  let match;
  while ((match = regex.exec(content)) !== null) {
    factories.add(match[1]);
  }
  return factories;
}

// ── 4. Derive expected factory name from service name ───────────────
// e.g. "AgentService" → "createAgentService"
function serviceToFactory(serviceName) {
  const base = serviceName.replace(/Service$/, '');
  return `create${base}Service`;
}

// ── 5. Derive expected factory name from generated client name ──────
// e.g. "createAgentServiceClient" → "createAgentService"
function clientToFactory(clientName) {
  return clientName.replace(/Client$/, '');
}

// ── Main ────────────────────────────────────────────────────────────
function main() {
  const protoServices = collectProtoServices();
  const generatedClients = collectGeneratedClients();
  const frontendFactories = collectFrontendFactories();

  console.log('=== Service Coverage Report ===\n');
  console.log(`Backend proto services:   ${protoServices.size}`);
  console.log(`Generated TS clients:     ${generatedClients.size}`);
  console.log(`Frontend factory funcs:   ${frontendFactories.size}`);
  console.log();

  // Check: proto services without frontend factory
  const missingFromProto = [];
  for (const [svcName, protoPath] of protoServices) {
    const expectedFactory = serviceToFactory(svcName);
    if (!frontendFactories.has(expectedFactory)) {
      missingFromProto.push(`${svcName} (expected ${expectedFactory}, from ${protoPath})`);
    }
  }

  // Check: generated clients without frontend factory
  const missingFromGenerated = [];
  for (const clientName of generatedClients) {
    const expectedFactory = clientToFactory(clientName);
    if (!frontendFactories.has(expectedFactory)) {
      missingFromGenerated.push(`${clientName} (expected ${expectedFactory})`);
    }
  }

  // Check: frontend factories without generated client (manual services)
  const manualServices = [];
  for (const factoryName of frontendFactories) {
    const expectedClient = `${factoryName}Client`;
    if (!generatedClients.has(expectedClient)) {
      manualServices.push(factoryName);
    }
  }

  let hasError = false;

  if (missingFromProto.length > 0) {
    console.error('ERROR: Proto services without frontend factory function:');
    for (const item of missingFromProto) {
      console.error(`  - ${item}`);
    }
    hasError = true;
  }

  if (missingFromGenerated.length > 0) {
    console.error('ERROR: Generated TS clients without frontend factory function:');
    for (const item of missingFromGenerated) {
      console.error(`  - ${item}`);
    }
    hasError = true;
  }

  if (manualServices.length > 0) {
    console.warn('WARN: Frontend factory functions without proto-generated client (manual HTTP):');
    for (const item of manualServices) {
      console.warn(`  ~ ${item}`);
    }
  }

  if (!hasError) {
    console.log('OK: All proto services have corresponding frontend factory functions');
  }

  console.log();
  process.exit(hasError ? 1 : 0);
}

main();
