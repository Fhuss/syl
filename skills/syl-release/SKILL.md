---
name: syl-release
description: "Release manager for Syl. Bumps semantic versioning (patch, minor, major), executes test suites, generates git release tags, and pushes to trigger GoReleaser GitHub Actions."
risk: safe
source: local
---

# Syl Release Skill

This skill guides the assistant and user in cutting semantic releases for the Syl repository (`github.com/Fhuss/syl`).

---

## 1. Quick Release Commands

When the user asks to bump or cut a release (e.g., *"release a patch"*, *"bump syl to minor"*, *"cut v0.2.0"*):

### Run the Release Script:
From `/Users/fabian/Dev/syl`:

```bash
# Bump patch (e.g., v0.1.0 -> v0.1.1): bugfixes, UI tweaks
./scripts/release.sh patch

# Bump minor (e.g., v0.1.0 -> v0.2.0): new features, commands, flags
./scripts/release.sh minor

# Bump major (e.g., v0.1.0 -> v1.0.0): breaking changes, architecture overhauls
./scripts/release.sh major

# Specify exact tag:
./scripts/release.sh v0.2.0
```

---

## 2. Release Procedure Checklist

When asked to perform a release, follow this sequence:

1. **Verify Git Tree**: Ensure all work is committed and working directory is clean (`git status`).
2. **Review Recent Commits**: Check `git log -n 10 --oneline` to recommend the appropriate SemVer bump:
   - `fix:` / bug fixes ➔ **patch**
   - `feat:` / new features / enhancements ➔ **minor**
   - Breaking CLI/API changes ➔ **major**
3. **Execute Pre-flight**: Run `./scripts/release.sh <bump_type>`.
   - The script automatically runs `go test ./...` across all packages before creating any tag.
   - If tests fail, tagging is aborted.
4. **Monitor GitHub Actions**:
   - GoReleaser will build binaries for macOS (`arm64`, `amd64`), Linux, and Windows.
   - Check status using:
     ```bash
     gh run list --workflow=release.yml --limit 1
     ```
     Or view in browser: `https://github.com/Fhuss/syl/actions`
5. **Verify Published Release**:
   - `gh release view` or check `https://github.com/Fhuss/syl/releases`
