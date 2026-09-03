'use strict';

const fs = require('fs');
const path = require('path');

const file = process.argv[2] || path.join(__dirname, 'PROMPT-BOUNDARY-LEDGER.json');
const ledger = JSON.parse(fs.readFileSync(file, 'utf8'));
const failures = [];
const check = (condition, message) => { if (!condition) failures.push(message); };

check(ledger.schema === 'cpa.s5-2.prompt-boundary-ledger.v1', 'schema');
check(ledger.source_modification === false, 'source_modification must be false');
check(ledger.aggregate.official_a_new_gap_requests === 61, 'A gap denominator');
check(ledger.aggregate.cpa_b_sanitized_requests === 60, 'B main denominator');
check(ledger.aggregate.cpa_b_native_prompt_ids_non_null === 0, 'B native prompt IDs must remain absent');
check(ledger.aggregate.cpa_b_gap_raw_body_files_scanned === 240, 'B gap stage denominator');
check(ledger.aggregate.cpa_b_gap_raw_cc_prompt_id_hits === 0, 'raw cc_prompt_id hits');
check(ledger.aggregate.cpa_b_gap_raw_promptId_hits === 0, 'raw promptId hits');
check(ledger.sequences.official_a_gap.length === 61, 'A sequence rows');
check(ledger.sequences.cpa_b_main.length === 60, 'B sequence rows');
check(ledger.scenario_matrix.length >= 8, 'scenario coverage');
for (const row of ledger.sequences.cpa_b_main) {
  check(row.prompt_id_sha256_prefix === null, `B prompt ID present at sequence ${row.sequence}`);
  check(row.source_path.startsWith('C:/'), `B source path is not absolute at sequence ${row.sequence}`);
}
for (const row of ledger.sequences.official_a_gap) {
  for (const key of ['body_sha256_prefix', 'session_sha256_prefix', 'prompt_id_sha256_prefix', 'prev_request_sha256_prefix', 'response_request_id_sha256_prefix', 'response_message_id_sha256_prefix']) {
    const value = row[key];
    check(value === null || /^sha256:[0-9a-f]{12}$/.test(value), `unredacted hash in A ${key}`);
  }
}
check(ledger.decision.generator_authorized === false, 'generator authorization');
check(ledger.controls_and_nonclaims.some((v) => /session ID/.test(v) && /random/.test(v)), 'non-derivation control');
if (failures.length) {
  console.error(`LEDGER_INVALID count=${failures.length}`);
  for (const failure of failures) console.error(`- ${failure}`);
  process.exit(1);
}
console.log(`LEDGER_VALID A_ROWS=${ledger.sequences.official_a_gap.length} B_ROWS=${ledger.sequences.cpa_b_main.length} B_NATIVE_PROMPT_IDS=${ledger.aggregate.cpa_b_native_prompt_ids_non_null} GAP_RAW_HITS=${ledger.aggregate.cpa_b_gap_raw_cc_prompt_id_hits}`);
