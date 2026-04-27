# Yanking a Release

This guide documents how to retract a buggy release (tag, GitHub Release, and Docker image) from GitHub.

## When to Yank

Yank a release when it contains a critical bug that makes it unusable or dangerous to deploy. Document the reason in a follow-up issue so users know why the release disappeared.

## Step-by-Step

### 1. Delete the GitHub Release

```bash
gh release delete <TAG> --yes
```

Example:
```bash
gh release delete 2026.04.4 --yes
```

### 2. Delete the Git Tag

Local:
```bash
git tag -d <TAG>
```

Remote:
```bash
git push --delete origin <TAG>
```

Example:
```bash
git tag -d 2026.04.4
git push --delete origin 2026.04.4
```

### 3. Delete the Docker Image from GHCR

#### 3a. Find the version ID

```bash
gh api "users/<OWNER>/packages/container/<PACKAGE>/versions" \
  --jq '.[] | select(.metadata.container.tags[] | contains("<TAG>")) | {id, tags: .metadata.container.tags}'
```

Example:
```bash
gh api "users/sushi30/packages/container/sushiclaw/versions" \
  --jq '.[] | select(.metadata.container.tags[] | contains("2026.04.4")) | {id, tags: .metadata.container.tags}'
```

#### 3b. Delete the version

```bash
gh api --method DELETE "users/<OWNER>/packages/container/<PACKAGE>/versions/<VERSION_ID>"
```

Example:
```bash
gh api --method DELETE "users/sushi30/packages/container/sushiclaw/versions/823757800"
```

> **Note:** This requires a GitHub token with `delete:packages` and `read:packages` scopes. If you get a 403, your token lacks the `delete:packages` scope.

#### Alternative: Delete via GitHub Web UI

If API deletion fails due to permissions, you can delete the package version manually:

1. Go to **Packages** in your profile or organization settings
2. Find the `sushiclaw` container package
3. Click **Manage versions**
4. Find the version tagged with the release
5. Click the trash icon to delete

### 4. Verify Retraction

- [ ] Tag no longer appears in `git tag --sort=-creatordate`
- [ ] Release no longer appears on the GitHub releases page
- [ ] Docker image no longer pullable: `docker pull ghcr.io/<OWNER>/<PACKAGE>:<TAG>` should fail

### 5. Communicate

- Create a GitHub issue documenting why the release was yanked
- If users may have the bad release deployed, announce the yank in relevant channels

## Troubleshooting

| Problem | Cause | Fix |
|---|---|---|
| `gh release delete` fails | Release doesn't exist | Skip this step |
| `git push --delete origin` fails | Tag already deleted remotely | Skip this step |
| `gh api ... 403` for image delete | Token lacks `delete:packages` scope | Use GitHub web UI or regenerate token with correct scopes |
| `gh api ... 404` for package list | Wrong package name or org vs user namespace | Try `orgs/<OWNER>` instead of `users/<OWNER>` |

## See Also

- [GitHub Docs: Deleting a package version](https://docs.github.com/en/packages/learn-github-packages/deleting-and-restoring-a-package)
- `RELEASE.md` — normal release process
