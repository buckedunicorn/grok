// Package sandbox provides sandboxing implementations of the harness
// Executor contract.
//
// The package shells out to vendor CLIs (docker, today; firecracker-ctl
// later if demand surfaces) instead of importing their Go SDKs, so no
// transitive dependencies bleed into grok consumers. Users need the
// vendor binary installed on $PATH; that's it.
//
// DockerExecutor satisfies the structural contract that
// agents/harness.Executor requires:
//
//	Run(ctx context.Context, cmd string, args []string, stdin []byte) (stdout, stderr []byte, err error)
//
// Wire it into a Harness:
//
//	exec := &sandbox.DockerExecutor{
//	    Image:      "alpine:3.20",
//	    NetworkOff: true,
//	    MemoryMB:   256,
//	}
//	h := harness.NewDefault(client.Chat, "grok-4-1-fast-reasoning",
//	    harness.WithExecutor(exec),
//	)
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// DockerExecutor runs commands inside ephemeral Docker containers via the
// docker CLI. Each Run() invocation creates a `docker run --rm ...`
// container, executes the command, and tears the container down.
//
// Required: Image. Everything else is optional with sensible defaults.
//
// Hardening defaults applied automatically (override individually):
//
//   - --network=none (set NetworkOn=true to opt in to networking)
//   - --security-opt=no-new-privileges (set AllowSetUID=true to disable)
//   - --cap-drop=ALL (set Caps to add specific capabilities back)
//   - --read-only with /tmp tmpfs (set WritableRoot=true to disable)
//   - --pids-limit=256 (set PIDsLimit explicitly to override; <0 disables)
//   - --user=65534:65534 (override via User; "" gets the default)
//   - stdout/stderr capped at MaxOutputBytes (default 1 MiB) per stream
//
// The struct is safe for concurrent use; each Run spawns its own docker
// process and shares no mutable state.
type DockerExecutor struct {
	// Image is the container image. Required, e.g. "alpine:3.20" or
	// "ghcr.io/your-org/agent-runtime:v2".
	Image string

	// Workdir is the working directory inside the container. Optional.
	Workdir string

	// Env adds container-side environment variables in "KEY=VALUE" form.
	// These are passed through `docker run --env`. Host environment is
	// NOT inherited; every var the command needs must be listed.
	Env []string

	// Mounts maps host paths to container paths (host -> container).
	// Use absolute host paths. Bound read-only by default; pass
	// MountsRW to opt into read-write.
	Mounts map[string]string

	// MountsRW is the same shape as Mounts but binds read-write.
	MountsRW map[string]string

	// NetworkOn opts INTO container networking. Default is off
	// (--network=none). Leave false for any untrusted code.
	NetworkOn bool

	// NetworkOff is deprecated; networking is OFF by default. The
	// field is retained as a no-op so callers that explicitly set it
	// to true continue to work. To enable networking, set NetworkOn.
	//
	// Deprecated: use NetworkOn.
	NetworkOff bool

	// MemoryMB sets the container memory limit. 0 = no limit.
	MemoryMB int

	// CPUs sets the CPU quota in fractional cores. 0 = no limit, e.g. 1.5
	// for "one and a half cores".
	CPUs float64

	// PullPolicy is one of "always", "missing", "never". Defaults to
	// "missing": pull only if not already cached locally. "never" lets
	// you ensure no surprise network calls during sandboxed runs.
	// Unknown values cause Run to return an error.
	PullPolicy string

	// User runs the container as the given UID:GID (e.g. "1000:1000").
	// Defaults to "65534:65534" (nobody:nogroup) when empty. Set to
	// "0:0" only if the workload genuinely needs container-root.
	User string

	// AllowSetUID disables --security-opt=no-new-privileges. Only
	// relevant if the container image relies on setuid binaries.
	AllowSetUID bool

	// WritableRoot disables --read-only and the /tmp tmpfs. Lets the
	// container persist artifacts in image layers.
	WritableRoot bool

	// Caps lists Linux capabilities to add back after the default
	// --cap-drop=ALL. Empty means no caps. Be specific.
	Caps []string

	// PIDsLimit caps in-container processes to defeat fork bombs.
	// 0 uses the default (256). A negative value disables the limit.
	PIDsLimit int

	// MaxOutputBytes caps stdout and stderr each at the given size.
	// Bytes past the cap are dropped and a "[truncated]" marker is
	// appended. 0 uses the default (1 MiB). A negative value disables
	// the cap (not recommended; an untrusted process can drive the
	// host out of memory by spamming stdout).
	MaxOutputBytes int64

	// BinPath overrides the docker binary. Defaults to "docker"
	// (resolved against $PATH).
	BinPath string

	// ExtraArgs are passed to `docker run` verbatim, before the image.
	// Use sparingly; anything important should have a typed field.
	// ExtraArgs is appended AFTER the hardening defaults so callers
	// can override individual flags by re-adding them here.
	ExtraArgs []string
}

// Run satisfies harness.Executor. It assembles a `docker run` invocation,
// pipes stdin in if non-nil, captures stdout/stderr (each capped at
// MaxOutputBytes), and tears the container down on completion or
// context cancellation. The returned error is the underlying
// *exec.ExitError when the command itself exited non-zero, allowing
// callers to inspect ExitCode().
func (d *DockerExecutor) Run(ctx context.Context, cmd string, args []string, stdin []byte) ([]byte, []byte, error) {
	if d.Image == "" {
		return nil, nil, fmt.Errorf("sandbox: DockerExecutor.Image is required")
	}
	if err := validatePullPolicy(d.PullPolicy); err != nil {
		return nil, nil, err
	}

	bin := d.BinPath
	if bin == "" {
		bin = "docker"
	}

	dockerArgs, err := d.buildArgs(cmd, args)
	if err != nil {
		return nil, nil, err
	}
	c := exec.CommandContext(ctx, bin, dockerArgs...)
	if len(stdin) > 0 {
		c.Stdin = bytes.NewReader(stdin)
	}
	outCap := d.MaxOutputBytes
	if outCap == 0 {
		outCap = 1 * 1024 * 1024
	}
	out := newCappedBuffer(outCap)
	errBuf := newCappedBuffer(outCap)
	c.Stdout = out
	c.Stderr = errBuf
	runErr := c.Run()
	return out.Bytes(), errBuf.Bytes(), runErr
}

// buildArgs returns the argv to docker (excluding the binary itself).
// Exposed package-private for testing; the argv shape is the
// load-bearing part of this package.
func (d *DockerExecutor) buildArgs(cmd string, cmdArgs []string) ([]string, error) {
	if err := validatePullPolicy(d.PullPolicy); err != nil {
		return nil, err
	}

	args := []string{"run", "--rm", "-i"}

	// Network: default deny.
	if !d.NetworkOn {
		args = append(args, "--network=none")
	}

	// no-new-privileges: default on.
	if !d.AllowSetUID {
		args = append(args, "--security-opt=no-new-privileges")
	}

	// Capabilities: default drop ALL, opt-in via Caps.
	args = append(args, "--cap-drop=ALL")
	for _, c := range d.Caps {
		args = append(args, "--cap-add="+c)
	}

	// Read-only root + tmpfs for /tmp: default on.
	if !d.WritableRoot {
		args = append(args, "--read-only", "--tmpfs=/tmp:rw,nosuid,nodev,size=64m")
	}

	// PID limit: default 256, < 0 disables.
	pids := d.PIDsLimit
	if pids == 0 {
		pids = 256
	}
	if pids > 0 {
		args = append(args, "--pids-limit", strconv.Itoa(pids))
	}

	if d.Workdir != "" {
		args = append(args, "--workdir", d.Workdir)
	}

	user := d.User
	if user == "" {
		user = "65534:65534" // nobody:nogroup
	}
	args = append(args, "--user", user)

	if d.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", d.MemoryMB))
	}
	if d.CPUs > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(d.CPUs, 'f', -1, 64))
	}

	switch d.PullPolicy {
	case "":
		args = append(args, "--pull=missing")
	default:
		args = append(args, "--pull="+d.PullPolicy)
	}

	for _, e := range d.Env {
		args = append(args, "--env", e)
	}

	// Mounts: read-only first (deterministic ordering), then read-write.
	for host, container := range sortedKVs(d.Mounts) {
		args = append(args, "--volume", host+":"+container+":ro")
	}
	for host, container := range sortedKVs(d.MountsRW) {
		args = append(args, "--volume", host+":"+container)
	}

	args = append(args, d.ExtraArgs...)
	// "--" terminates option parsing so a stray "--something" in d.Image
	// (whether from a misconfiguration or hostile input) cannot be
	// reinterpreted as a docker flag. Defense in depth: Docker's CLI
	// already requires flags before the image, but a separator costs
	// nothing.
	args = append(args, "--", d.Image)
	args = append(args, cmd)
	args = append(args, cmdArgs...)
	return args, nil
}

// validatePullPolicy returns nil for "" or any of "always", "missing",
// "never", and an error otherwise. Catches misspellings before they
// silently degrade to the docker default.
func validatePullPolicy(p string) error {
	switch p {
	case "", "always", "missing", "never":
		return nil
	}
	return fmt.Errorf("sandbox: unknown PullPolicy %q (want \"\", \"always\", \"missing\", or \"never\")", p)
}

// cappedBuffer wraps bytes.Buffer with a per-stream byte cap. Writes
// past the cap are dropped after appending a single "[truncated]"
// marker so the caller can tell.
type cappedBuffer struct {
	limit     int64
	buf       bytes.Buffer
	written   int64
	truncated bool
}

func newCappedBuffer(limit int64) *cappedBuffer {
	if limit < 0 {
		// Negative means unbounded; substitute MaxInt64 for simplicity.
		limit = 1<<63 - 1
	}
	return &cappedBuffer{limit: limit}
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.truncated {
		return len(p), nil
	}
	if c.written+int64(len(p)) <= c.limit {
		c.written += int64(len(p))
		return c.buf.Write(p)
	}
	room := c.limit - c.written
	if room > 0 {
		c.buf.Write(p[:room])
		c.written = c.limit
	}
	c.buf.WriteString("\n[truncated]\n")
	c.truncated = true
	return len(p), nil
}

func (c *cappedBuffer) Bytes() []byte { return c.buf.Bytes() }

// sortedKVs yields map entries in key-sorted order. Stability matters
// because argv affects container identity (e.g. caching); two equivalent
// configs should produce identical argv.
func sortedKVs(m map[string]string) func(yield func(k, v string) bool) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// stdlib sort would import sort; keep deps zero. Insertion sort is
	// fine for the tiny key counts (host paths) we expect.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return func(yield func(k, v string) bool) {
		for _, k := range keys {
			if !yield(k, m[k]) {
				return
			}
		}
	}
}

// Available reports whether the docker binary is callable. Tests and
// programs that want to fail fast on missing dependencies should call this
// at startup. Returns nil on success, the underlying error otherwise.
func (d *DockerExecutor) Available() error {
	bin := d.BinPath
	if bin == "" {
		bin = "docker"
	}
	c := exec.Command(bin, "version", "--format", "{{.Client.Version}}")
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sandbox: docker not available (%w): %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
