# S5-3A Prompt Boundary Contract

Changed branch/field: `feat/s3-resolved-profile` / internal typed prompt-boundary
contract for `B-12 cc_prompt_id`.

The modified tree adds an adapter-owned Go context contract with `absent`,
`new`, `continue` and `ambiguous` states. The Claude executor validates the
contract identically in Execute and ExecuteStream. This step does not allocate,
store, insert or commit any prompt ID and does not change the default wire
request.

Disposition: `S5-3A_CONTRACT_IMPLEMENTED_GENERATION_NOT_AUTHORIZED`.
