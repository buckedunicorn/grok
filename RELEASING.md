# Releasing

This project follows [Semantic Versioning](https://semver.org/). Go module consumers fetch versions directly from VCS, so pushing a `vX.Y.Z` tag is the entire publish step — there is no registry to upload to.

## Versioning policy

- **`v0.x.y`** (current): API may break between minor releases. Consumers should pin to an exact tag in production.
- **`v1.0.0`** (when the surface stabilizes): API stable; only minor bumps may add functionality.
- **`v2.0.0+`**: requires renaming the module path to include `/v2` (`github.com/buckedunicorn/grok/v2`) per the [Go modules spec](https://go.dev/ref/mod#major-version-suffixes).

The [`examples/discord-bot`](./examples/discord-bot) submodule is not versioned independently; copy or fork it for production use.

## Cutting a release

1. **Sync and confirm a clean tree on `main`.**
   ```sh
   git checkout main && git pull --ff-only
   git status   # must report nothing
   ```

2. **Update [`CHANGELOG.md`](./CHANGELOG.md).**
   - Move every entry under `## [Unreleased]` into a new `## [X.Y.Z] - YYYY-MM-DD` heading.
   - Leave an empty `## [Unreleased]` block at the top.
   - Update the link references at the bottom: bump the `[Unreleased]` compare to `vX.Y.Z...HEAD` and add a fresh `[X.Y.Z]` line pointing at the new tag.

3. **Run every gate locally.**
   ```sh
   gofmt -s -l .                 # must print nothing
   go vet ./...
   go build ./...
   go test -race -shuffle=on ./...
   govulncheck ./...
   ```
   Repeat the build/vet/test inside `examples/discord-bot/` (it is a separate module).

4. **Commit and tag.**
   ```sh
   git add CHANGELOG.md
   git commit -m "release: vX.Y.Z"
   git tag -a vX.Y.Z -m "grok vX.Y.Z"
   ```

5. **Push.**
   ```sh
   git push origin main
   git push origin vX.Y.Z
   ```

That is the release. Within a few minutes, `go get github.com/buckedunicorn/grok@vX.Y.Z` will work for any consumer; the public Go module proxy (`proxy.golang.org`) caches the tag automatically on first request.

## Optional follow-ups

- **GitHub Release.** Open <https://github.com/buckedunicorn/grok/releases/new>, pick the new tag, and paste the corresponding `CHANGELOG.md` section as the body. This makes the release discoverable from the repo's Releases tab and gives users a stable URL to link to.
- **Verify the proxy.** A quick smoke check that the tag is reachable:
  ```sh
  GOPROXY=https://proxy.golang.org go list -m github.com/buckedunicorn/grok@vX.Y.Z
  ```

## Hotfixes

Land the fix on `main`, cut a patch release the same way (`vX.Y.Z+1`). Branching off the previous tag is only worth it once `main` has diverged enough that a forward-only patch would drag in unrelated changes.

## Yanking a bad release

Do not delete the tag — `proxy.golang.org` has already cached it. Instead, retract it in `go.mod` and ship a fix:

```
retract vX.Y.Z // <reason>
```

Then commit, tag the next patch version, and push both. Consumers who already pinned to the bad tag will see a warning on `go mod tidy`.
