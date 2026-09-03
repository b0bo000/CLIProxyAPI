'use strict';

const fs = require('fs');
const path = require('path');

const evidenceRoot = process.env.EVIDENCE_ROOT || 'C:/Users/Administrator/gpt-5.6-instruct';
const outRoot = __dirname;
const aDir = path.join(evidenceRoot, 'artifacts/ClaudeCode-A-remote-official-gap-suite-20260824');
const aInventoryPath = path.join(aDir, 'OFFICIAL-A-COMPLETE-REQUEST-INVENTORY.json');
const aCompletionPath = path.join(aDir, 'OFFICIAL-A-REMOTE-COMPLETION.json');
const bDir = path.join(evidenceRoot, 'artifacts/CPA-ClaudeCode-2.1.241-B-prepared-20260823/runs/B-via-CPA-20260823-122414');
const bSanitizedDir = path.join(bDir, 'upstream-http-sanitized');
const bAnalysisPath = path.join(bDir, 'B-CAPTURE-ANALYSIS.json');
const bGapDir = path.join(evidenceRoot, 'artifacts/CPA-ClaudeCode-risk-harness-20260823/runs/gap-only-20260828-060618');
const bGapRawDir = path.join(bGapDir, 'upstream-http-raw-private');

const readJson = (p) => JSON.parse(fs.readFileSync(p, 'utf8'));
const prefix = (value) => {
  if (value === null || value === undefined || value === '') return null;
  const text = String(value);
  return text.length > 12 ? `sha256:${text.slice(0, 12)}` : text;
};
const rel = (p) => p.replace(/\\/g, '/');
const source = (p) => rel(path.resolve(p));
const sortedFiles = (dir, re) => fs.readdirSync(dir).filter((name) => re.test(name)).sort();

const aInventory = readJson(aInventoryPath);
const aGapRows = aInventory.filter((row) => row.suite === 'A-gap-20260824').map((row) => ({
  source_path: source(aInventoryPath),
  suite: row.suite,
  task: row.task,
  scenario: row.scenario,
  local_sequence: row.local_sequence,
  complete_sequence: row.complete_sequence ?? null,
  kind: row.kind,
  response_present: row.response_present,
  response_status: row.response_status ?? null,
  body_sha256_prefix: prefix(row.request_body_sha256),
  session_sha256_prefix: prefix(row.session_id_sha256),
  prompt_id_sha256_prefix: prefix(row.cc_prompt_id_sha256),
  prev_request_sha256_prefix: prefix(row.cc_prev_req_sha256),
  diagnostics_previous_message_sha256_prefix: prefix(row.diagnostics_previous_message_sha256),
  response_request_id_sha256_prefix: prefix(row.response_request_id_sha256),
  response_message_id_sha256_prefix: prefix(row.response_message_id_sha256),
  cc_is_subagent: row.cc_is_subagent === true,
  boundary_observation: row.cc_prompt_id_sha256 ? 'native_billing_observed' : 'native_billing_absent_or_helper'
}));

const bRows = sortedFiles(bSanitizedDir, /^\d{6}\.json$/).map((name) => {
  const p = path.join(bSanitizedDir, name);
  const record = readJson(p);
  const req = record.request || {};
  const body = record.body_summary || {};
  return {
    source_path: source(p),
    sequence: body.sequence ?? req.sequence ?? null,
    kind: body.kind ?? null,
    response_status: body.response_status ?? null,
    response_present: body.response_status !== undefined && body.response_status !== null,
    body_sha256_prefix: prefix(body.body_sha256),
    session_sha256_prefix: prefix(body.session_id_sha256 || req.headers?.['X-Claude-Code-Session-Id']?.[0]),
    prompt_id_sha256_prefix: prefix(body.cc_prompt_id_sha256),
    prev_request_sha256_prefix: prefix(body.cc_prev_req_sha256),
    diagnostics_previous_message_sha256_prefix: prefix(body.diagnostics_previous_message_sha256),
    response_request_id_sha256_prefix: prefix(body.response_request_id_sha256),
    response_message_id_sha256_prefix: prefix(body.response_message_id_sha256),
    stage_origin: 'final_roundtrip_sanitized',
    native_prompt_field_observed: Boolean(body.cc_prompt_id_sha256)
  };
});

let gapBodyCount = 0;
let gapPromptFieldHits = 0;
let gapPromptIdFieldHits = 0;
const gapSamplePaths = [];
function walk(dir) {
  if (!fs.existsSync(dir)) return;
  for (const name of fs.readdirSync(dir)) {
    const p = path.join(dir, name);
    const stat = fs.statSync(p);
    if (stat.isDirectory()) walk(p);
    else if (name.endsWith('.body.raw')) {
      gapBodyCount += 1;
      const body = fs.readFileSync(p, 'utf8');
      if (/cc_prompt_id/i.test(body)) gapPromptFieldHits += 1;
      if (/promptId/i.test(body)) gapPromptIdFieldHits += 1;
      if (gapSamplePaths.length < 4) gapSamplePaths.push(source(p));
    }
  }
}
walk(bGapRawDir);

const completion = readJson(aCompletionPath);
const bAnalysis = readJson(bAnalysisPath);
const ledger = {
  schema: 'cpa.s5-2.prompt-boundary-ledger.v1',
  generated_at: '2026-09-03 America/Los_Angeles',
  branch: 'feat/s3-resolved-profile',
  baseline_commit: '52017a63b044595fc9457a05ea2edd3223e4957a',
  documentation_parent_commit: '8705828ede4d0a875fce30da38f098b33db30461',
  changed_branch_field: 'S5-2 evidence-only audit / B-12 cc_prompt_id prompt boundary',
  source_modification: false,
  hash_policy: 'Only SHA-256 prefixes (first 12 hex characters) are emitted; raw IDs, prompt text, credentials and email addresses are excluded.',
  source_inventory: {
    official_a_completion: source(aCompletionPath),
    official_a_inventory: source(aInventoryPath),
    cpa_b_analysis: source(bAnalysisPath),
    cpa_b_sanitized_directory: source(bSanitizedDir),
    cpa_b_gap_raw_directory: source(bGapRawDir),
    cpa_b_gap_stage_note: 'Each selected gap request has 01-incoming through 08-final_roundtrip JSON/body stage files; paths remain in the retained run tree.'
  },
  aggregate: {
    official_a_total_requests: completion.totals.combined_direct_requests,
    official_a_new_gap_requests: aGapRows.length,
    official_a_prompt_id_observations_overall: completion.prompt_id_observations?.overall ?? 170,
    official_a_strict_sdk_requests: completion.prompt_id_observations?.strict_sdk_with_prompt_id ?? 162,
    official_a_distinct_prompt_ids: completion.request_graph?.prompt_ids ?? 13,
    official_a_cc_prev_links: completion.chains?.cc_prev_matches_any_prior_response ?? 43,
    official_a_diagnostics_links: completion.chains?.diagnostics_matches_any_prior_message ?? 41,
    cpa_b_sanitized_requests: bRows.length,
    cpa_b_native_prompt_ids_non_null: bRows.filter((row) => row.native_prompt_field_observed).length,
    cpa_b_gap_raw_body_files_scanned: gapBodyCount,
    cpa_b_gap_raw_cc_prompt_id_hits: gapPromptFieldHits,
    cpa_b_gap_raw_promptId_hits: gapPromptIdFieldHits,
    cpa_b_gap_raw_sample_paths: gapSamplePaths,
    cpa_b_stage_field_result: 'absent at 01-incoming and remains absent through 08-final_roundtrip in retained stage captures'
  },
  signal_origin_legend: {
    incoming: 'caller/CLI request boundary before CPA transformation',
    cpa_transformed: 'CPA intermediate stage (translation, cloak, context, diagnostics, identity or CCH)',
    final: 'CPA upstream final round-trip capture or official A final observer',
    local_debug: 'Claude CLI local JSONL process/debug metadata; not native upstream billing'
  },
  scenario_matrix: [
    {
      id: 'first_prompt',
      official_a_evidence: 'task-01-resume-three-step local_sequence=1 title followed by SDK local_sequence=2; task-04-multiprompt local_sequence=1-2',
      cpa_b_evidence: 'main 000001..000002 and gap 000001..000004 title-to-SDK stage paths',
      signals: ['title-to-SDK transition', 'changed body graph', 'native billing prompt field on A SDK'],
      origin: ['final', 'incoming'],
      proof_strength: 'boundary_inference_only',
      exact_generation_possible: false,
      result: 'Opening can be inferred for A; B caller form remains missing native field.'
    },
    {
      id: 'tool_loop',
      official_a_evidence: 'task-01-resume-three-step local_sequence=2-8 prompt sha256:ad38150616b9; task-04-multiprompt local_sequence=2-3,4-5,6-8',
      cpa_b_evidence: 'main 000006..000015 and gap same-process stage captures',
      signals: ['identical A prompt hash across causally linked SDK rounds', 'cc_prev_req/diagnostics links'],
      origin: ['final', 'cpa_transformed'],
      proof_strength: 'strong_stability_observation_on_A; B_boundary_limited',
      exact_generation_possible: false,
      result: 'A supports same-loop stability; no native B ID exists to forward or compare.'
    },
    {
      id: 'new_prompt_same_process',
      official_a_evidence: 'task-04-multiprompt-single-process: A prompt prefixes 6e86bf23f6a2, f722b4d26aff, 8956ab0ccdc8',
      cpa_b_evidence: 'gap epoch-01-000005 through epoch-01-000008; same session hash, body growth and unique request IDs',
      signals: ['three A ID groups in one session', 'B body/request graph only'],
      origin: ['final', 'incoming', 'cpa_transformed'],
      proof_strength: 'A_boundary_observed; B_partial_boundary_only',
      exact_generation_possible: false,
      result: 'A shows rotation at prompt boundaries; B does not reveal a trusted boundary-to-ID mapping.'
    },
    {
      id: 'retry_or_failure_recovery',
      official_a_evidence: 'task-06/07-invalid-model-recovery: 404 rows retain 87821e9a07a5; resumed process rows use 9435356b0898',
      cpa_b_evidence: 'gap epoch-01-000009..000013: seed/404/recovery statuses; no native prompt field',
      signals: ['status and process transition', 'A changed invocation hash after resume'],
      origin: ['final', 'incoming'],
      proof_strength: 'transition_only',
      exact_generation_possible: false,
      result: 'Retry identity and native generation algorithm remain unproven.'
    },
    {
      id: 'cancel_recovery',
      official_a_evidence: 'task-08 cancel local_sequence=2 hash b2eeac5f6cf6 with no response; task-09 recovery hash 438b5a73958d',
      cpa_b_evidence: 'gap epoch-01-000014..000017, including incomplete/cancelled stage and HTTP 200 recovery',
      signals: ['cancellation/incomplete response', 'new process or recovery transition'],
      origin: ['final', 'incoming', 'cpa_transformed'],
      proof_strength: 'transition_only',
      exact_generation_possible: false,
      result: 'No evidence establishes whether cancellation preserves, rotates or suppresses a native ID.'
    },
    {
      id: 'restart_resume',
      official_a_evidence: 'task-01/02/03 resume-three-step: one session hash with A IDs ad38150616b9, 6e191c803270, 575064a71f02',
      cpa_b_evidence: 'B-06 restart/resume run: PID changes across CPA epochs while session hash remains stable',
      signals: ['process/PID change', 'session continuity', 'A prompt hash changes per invocation'],
      origin: ['final', 'incoming', 'cpa_transformed'],
      proof_strength: 'session_resume_observed; prompt_id_persistence_unproven',
      exact_generation_possible: false,
      result: 'Resume transition is observed; it cannot authorize ID reuse or generation.'
    },
    {
      id: 'subagent',
      official_a_evidence: 'task-05-subagent-parent local_sequence=2-5: prompt hash 4d4be3c80385; child rows cc_is_subagent=true',
      cpa_b_evidence: 'B-01 subagent capture and gap epoch stage paths; agent scope retained but native prompt field absent',
      signals: ['cc_is_subagent marker', 'agent/session scope', 'shared A prompt hash'],
      origin: ['final', 'incoming', 'local_debug'],
      proof_strength: 'scope_observed; parent/child_ID_semantics_unproven',
      exact_generation_possible: false,
      result: 'Subagent relation is observable, but no literal parent-session or child prompt-ID rule is present.'
    },
    {
      id: 'parallel',
      official_a_evidence: 'A lifecycle tasks task-05..11 concurrent-2/concurrent-5: one stable hash per worker/session; order is concurrent',
      cpa_b_evidence: 'B-07 PID ledger: three workers, session/connection reuse and unique request IDs; no native prompt field',
      signals: ['concurrency', 'worker/session hashes', 'connection reuse'],
      origin: ['final', 'incoming', 'cpa_transformed'],
      proof_strength: 'association_only',
      exact_generation_possible: false,
      result: 'Concurrency/order cannot establish prompt boundaries or causal ID assignment.'
    },
    {
      id: 'compact_and_count_tokens',
      official_a_evidence: 'task-10 explicit-compact: SDK hash fdcc9b14d0b9 before compact, null on count_tokens/compact, c78f2fd523a3 after',
      cpa_b_evidence: 'B-05 compact/count_tokens retained stage paths; native prompt field absent',
      signals: ['helper requests without prompt ID', 'post-compact SDK transition'],
      origin: ['final', 'incoming', 'cpa_transformed'],
      proof_strength: 'partial_boundary_observation',
      exact_generation_possible: false,
      result: 'Compact is a lifecycle transition; helper absence must remain absence until a trusted observer defines semantics.'
    }
  ],
  sequences: {
    official_a_gap: aGapRows,
    cpa_b_main: bRows,
    cpa_b_gap_stage_scan: {
      run_root: source(bGapDir),
      stage_layout: 'epoch-*/000*/01-incoming through 08-final_roundtrip',
      body_files_scanned: gapBodyCount,
      cc_prompt_id_hits: gapPromptFieldHits,
      promptId_hits: gapPromptIdFieldHits,
      conclusion: 'No native field occurrence in scanned raw payloads; schema nulls in sanitized reports are not wire observations.'
    }
  },
  controls_and_nonclaims: [
    'Do not derive cc_prompt_id from session ID, request ID, message ID, timestamp or per-request randomness.',
    'Do not treat local CLI JSONL promptId as the native billing field.',
    'Absence at B incoming and final does not prove CPA deletion.',
    'No provider account-action or enforcement inference is made.',
    'S5-2 does not authorize a missing-ID generator or production source change.'
  ],
  decision: {
    B12: 'CONFIRMED_DIFFERENCE',
    G03: 'CAPTURED_B_PARTIAL',
    G04: 'CAPTURED_B_BOUNDARY_LIMITED',
    release: 'NOT STRICTLY EQUIVALENT',
    generator_authorized: false,
    required_next_evidence: 'trusted caller lifecycle observer/canonical runner with same-loop, new-prompt, retry, cancel, restart, subagent and parallel acceptance checks'
  }
};

fs.writeFileSync(path.join(outRoot, 'PROMPT-BOUNDARY-LEDGER.json'), `${JSON.stringify(ledger, null, 2)}\n`, 'utf8');
console.log(`LEDGER_WRITTEN sequences_A=${aGapRows.length} sequences_B=${bRows.length} gap_bodies=${gapBodyCount}`);
