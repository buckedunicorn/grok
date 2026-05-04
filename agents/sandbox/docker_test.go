package sandbox_test

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/agents/sandbox"
)

// buildArgs is exercised through Run on a stub PATH where the docker
// binary is replaced with /bin/echo, so we can assert the exact argv the
// executor emitted.
func runWithArgvCapture(t *testing.T, d *sandbox.DockerExecutor, cmd string, args []string) []string {
	t.Helper()
	echo, err := exec.LookPath("echo")
	if err != nil {
		t.Skip("echo not on PATH")
	}
	d.BinPath = echo
	stdout, _, err := d.Run(context.Background(), cmd, args, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// echo prints its argv space-separated; split to recover argv.
	return strings.Fields(strings.TrimSpace(string(stdout)))
}

func TestDockerExecutor_RequiresImage(t *testing.T) {
	d := &sandbox.DockerExecutor{}
	_, _, err := d.Run(context.Background(), "echo", []string{"hi"}, nil)
	if err == nil {
		t.Fatal("expected error on empty Image")
	}
}

func TestDockerExecutor_minimalArgv(t *testing.T) {
	d := &sandbox.DockerExecutor{Image: "alpine:3.20"}
	got := runWithArgvCapture(t, d, "echo", []string{"hi"})
	// Hardening defaults are applied automatically:
	// network=none, no-new-privileges, cap-drop=ALL, read-only with
	// /tmp tmpfs, pids-limit=256, and the nobody:nogroup user.
	want := []string{
		"run", "--rm", "-i",
		"--network=none",
		"--security-opt=no-new-privileges",
		"--cap-drop=ALL",
		"--read-only", "--tmpfs=/tmp:rw,nosuid,nodev,size=64m",
		"--pids-limit", "256",
		"--user", "65534:65534",
		"--pull=missing",
		"--", "alpine:3.20", "echo", "hi",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %v\nwant %v", got, want)
	}
}

func TestDockerExecutor_NetworkOn_optsInToNetwork(t *testing.T) {
	d := &sandbox.DockerExecutor{Image: "alpine", NetworkOn: true}
	got := runWithArgvCapture(t, d, "true", nil)
	for _, a := range got {
		if a == "--network=none" {
			t.Fatal("--network=none should be absent when NetworkOn=true")
		}
	}
}

func TestDockerExecutor_AllowSetUID_omitsNoNewPrivs(t *testing.T) {
	d := &sandbox.DockerExecutor{Image: "alpine", AllowSetUID: true}
	got := runWithArgvCapture(t, d, "true", nil)
	for _, a := range got {
		if a == "--security-opt=no-new-privileges" {
			t.Fatal("no-new-privileges should be absent when AllowSetUID=true")
		}
	}
}

func TestDockerExecutor_WritableRoot_omitsReadOnly(t *testing.T) {
	d := &sandbox.DockerExecutor{Image: "alpine", WritableRoot: true}
	got := runWithArgvCapture(t, d, "true", nil)
	for _, a := range got {
		if a == "--read-only" {
			t.Fatal("--read-only should be absent when WritableRoot=true")
		}
	}
}

func TestDockerExecutor_CapsAddedAfterDrop(t *testing.T) {
	d := &sandbox.DockerExecutor{Image: "alpine", Caps: []string{"NET_BIND_SERVICE"}}
	got := runWithArgvCapture(t, d, "true", nil)
	dropIdx, addIdx := -1, -1
	for i, a := range got {
		if a == "--cap-drop=ALL" {
			dropIdx = i
		}
		if a == "--cap-add=NET_BIND_SERVICE" {
			addIdx = i
		}
	}
	if dropIdx == -1 || addIdx == -1 || addIdx < dropIdx {
		t.Errorf("cap-add must follow cap-drop: drop=%d add=%d argv=%v", dropIdx, addIdx, got)
	}
}

func TestDockerExecutor_PIDsLimit_negativeDisables(t *testing.T) {
	d := &sandbox.DockerExecutor{Image: "alpine", PIDsLimit: -1}
	got := runWithArgvCapture(t, d, "true", nil)
	for _, a := range got {
		if a == "--pids-limit" {
			t.Fatal("--pids-limit should be absent when PIDsLimit < 0")
		}
	}
}

func TestDockerExecutor_imageSeparatorBlocksFlagInjection(t *testing.T) {
	// If Image were ever set to "--privileged", the "--" separator before
	// the image position must keep docker from interpreting it as a flag.
	d := &sandbox.DockerExecutor{Image: "--privileged"}
	got := runWithArgvCapture(t, d, "echo", nil)
	idxSep := -1
	for i, a := range got {
		if a == "--" {
			idxSep = i
			break
		}
	}
	if idxSep == -1 {
		t.Fatal("missing -- separator before image")
	}
	if idxSep+1 >= len(got) || got[idxSep+1] != "--privileged" {
		t.Errorf("expected -- followed by image, got %v", got[idxSep:])
	}
}

func TestDockerExecutor_allKnobsArgv(t *testing.T) {
	d := &sandbox.DockerExecutor{
		Image:      "alpine:3.20",
		Workdir:    "/work",
		Env:        []string{"FOO=bar", "BAZ=qux"},
		Mounts:     map[string]string{"/host/in": "/in", "/host/data": "/data"},
		MountsRW:   map[string]string{"/host/out": "/out"},
		NetworkOff: true,
		MemoryMB:   512,
		CPUs:       1.5,
		PullPolicy: "never",
		User:       "1000:1000",
		ExtraArgs:  []string{"--security-opt", "no-new-privileges"},
	}
	got := runWithArgvCapture(t, d, "/bin/sh", []string{"-c", "true"})

	// Spot-check key flags appear; full ordering is internal but stable:
	mustContain(t, got, "--network=none")
	mustContain(t, got, "--workdir", "/work")
	mustContain(t, got, "--user", "1000:1000")
	mustContain(t, got, "--memory", "512m")
	mustContain(t, got, "--cpus", "1.5")
	mustContain(t, got, "--pull=never")
	mustContain(t, got, "--env", "FOO=bar")
	mustContain(t, got, "--env", "BAZ=qux")

	// Read-only mounts come before read-write, key-sorted within each.
	idxRO1 := mustIndex(t, got, "/host/data:/data:ro")
	idxRO2 := mustIndex(t, got, "/host/in:/in:ro")
	idxRW := mustIndex(t, got, "/host/out:/out")
	if idxRO1 > idxRO2 {
		t.Errorf("read-only mounts not key-sorted: %v vs %v", idxRO1, idxRO2)
	}
	if idxRO2 > idxRW {
		t.Errorf("read-only mount should precede read-write: %v vs %v", idxRO2, idxRW)
	}

	// ExtraArgs land before the image.
	idxExtra := mustIndex(t, got, "no-new-privileges")
	idxImage := mustIndex(t, got, "alpine:3.20")
	if idxExtra > idxImage {
		t.Errorf("ExtraArgs must come before image: extra=%d image=%d", idxExtra, idxImage)
	}

	// Image precedes the command.
	idxCmd := mustIndex(t, got, "/bin/sh")
	if idxImage > idxCmd {
		t.Errorf("image must come before cmd: image=%d cmd=%d", idxImage, idxCmd)
	}
}

func TestDockerExecutor_pullPolicyDefault(t *testing.T) {
	d := &sandbox.DockerExecutor{Image: "alpine"}
	got := runWithArgvCapture(t, d, "true", nil)
	if !slices.Contains(got, "--pull=missing") {
		t.Errorf("default PullPolicy should be 'missing', argv = %v", got)
	}
}

func TestDockerExecutor_invalidPullPolicyRejected(t *testing.T) {
	// unknown PullPolicy values are now rejected with a
	// clear error so misspellings do not silently degrade to docker's
	// own default.
	d := &sandbox.DockerExecutor{Image: "alpine", PullPolicy: "garbage"}
	_, _, err := d.Run(t.Context(), "true", nil, nil)
	if err == nil {
		t.Fatal("expected error from invalid PullPolicy")
	}
	if !strings.Contains(err.Error(), "unknown PullPolicy") {
		t.Errorf("error = %v, want \"unknown PullPolicy\"", err)
	}
}

func TestDockerExecutor_runWithoutDocker(t *testing.T) {
	// Point at a nonexistent binary; should error cleanly.
	d := &sandbox.DockerExecutor{Image: "alpine", BinPath: "/no/such/docker"}
	_, _, err := d.Run(context.Background(), "true", nil, nil)
	if err == nil {
		t.Fatal("expected error when docker binary missing")
	}
}

func TestDockerExecutor_Available_reportsMissingBinary(t *testing.T) {
	d := &sandbox.DockerExecutor{Image: "alpine", BinPath: "/no/such/docker"}
	if err := d.Available(); err == nil {
		t.Fatal("expected error from Available with missing binary")
	}
}

// --- integration smoke test (skipped if docker not present) ---

func TestDockerExecutor_integration_echo(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH")
	}
	d := &sandbox.DockerExecutor{Image: "alpine:3.20", PullPolicy: "missing"}
	if err := d.Available(); err != nil {
		t.Skipf("docker not available: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stdout, _, err := d.Run(ctx, "echo", []string{"sandbox-hello"}, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(string(stdout), "sandbox-hello") {
		t.Errorf("stdout = %q, want substring 'sandbox-hello'", stdout)
	}
}

func TestDockerExecutor_integration_ctxCancellation(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH")
	}
	d := &sandbox.DockerExecutor{Image: "alpine:3.20", PullPolicy: "missing"}
	if err := d.Available(); err != nil {
		t.Skipf("docker not available: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, _, err := d.Run(ctx, "sleep", []string{"30"}, nil)
	if err == nil {
		t.Fatal("expected error from ctx timeout killing the container")
	}
	// We don't assert the specific error type, docker SIGKILL behavior
	// varies. Just confirm we didn't actually wait 30s.
}

func TestDockerExecutor_satisfiesHarnessExecutorContract(t *testing.T) {
	// Compile-time check: DockerExecutor.Run signature must match
	// harness.Executor.Run (structural). If this stops compiling, the
	// harness contract changed and sandbox needs to follow.
	var d *sandbox.DockerExecutor
	type executor interface {
		Run(ctx context.Context, cmd string, args []string, stdin []byte) (stdout, stderr []byte, err error)
	}
	var _ executor = d
	_ = errors.New // keep imports clean if any future refactor needs an err handle
}

// --- helpers ---

func mustContain(t *testing.T, argv []string, want ...string) {
	t.Helper()
	for i := 0; i+len(want) <= len(argv); i++ {
		if reflect.DeepEqual(argv[i:i+len(want)], want) {
			return
		}
	}
	t.Errorf("argv missing subsequence %v\nargv: %v", want, argv)
}

func mustIndex(t *testing.T, argv []string, needle string) int {
	t.Helper()
	for i, a := range argv {
		if a == needle {
			return i
		}
	}
	t.Errorf("argv missing %q: %v", needle, argv)
	return -1
}
