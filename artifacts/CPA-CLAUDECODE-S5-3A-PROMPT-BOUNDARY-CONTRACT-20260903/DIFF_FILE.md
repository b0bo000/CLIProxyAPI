# S5-3A Diff Summary

Changed branch/field: `feat/s3-resolved-profile` / internal typed prompt-boundary
contract for `B-12 cc_prompt_id`.

- Adds a private context key and typed adapter hint in `sdk/cliproxy/executor`.
- Adds strict Claude validation for kind/transaction combinations.
- Adds identical pre-upstream guards in Execute and ExecuteStream.
- Adds context, helper and executor parity tests.
- Adds no prompt-ID generator, state owner, billing insertion or default config.

The exact source diff is `SOURCE-DIFF.patch`.
