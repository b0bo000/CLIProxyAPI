# Historical Diff Pointer

The withdrawn implementation is preserved in Git history:

- Implementation: `b6493c9d`
- Rollback-artifact mode change: `a8c51445`
- Historical execution-log record: `baafc7db`
- Reverts from the active branch: `0bc530f3`, `6810c7be`, `a864da08`

To inspect the exact historical patch without changing the working tree:

```text
git show --stat b6493c9d
git diff 704f980a b6493c9d -- internal sdk docs artifacts
```

The active tree is intentionally not restored to the historical
implementation.
