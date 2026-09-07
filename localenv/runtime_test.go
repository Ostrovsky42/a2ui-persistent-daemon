package localenv

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func testDescriptor(instance string) Descriptor {
	return Descriptor{
		Version:      DescriptorVersion,
		InstanceID:   instance,
		Socket:       "/tmp/a2ui-test/a2ui.sock",
		Server:       "http://127.0.0.1:18080",
		Session:      "default",
		Preset:       "dashboard",
		ViewerPolicy: "auto",
	}
}

func TestResolvePathsUsesSocketDirectory(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "a2ui.sock")
	paths, err := ResolvePaths(socket)
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	if paths.Dir != dir {
		t.Fatalf("Dir=%q, want %q", paths.Dir, dir)
	}
	if paths.Environment != filepath.Join(dir, "environment.json") {
		t.Fatalf("Environment=%q", paths.Environment)
	}
	if paths.UpLock != filepath.Join(dir, "up.lock") || paths.SupervisorLock != filepath.Join(dir, "supervisor.lock") {
		t.Fatalf("lock paths=%#v", paths)
	}
}

func TestDescriptorWriteReadIsPrivateAndReplacesStaleMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	paths, err := ResolvePaths(filepath.Join(dir, "a2ui.sock"))
	if err != nil {
		t.Fatal(err)
	}

	stale := testDescriptor("stale-instance")
	if err := WriteDescriptor(paths, stale); err != nil {
		t.Fatalf("WriteDescriptor(stale): %v", err)
	}
	fresh := testDescriptor("fresh-instance")
	if err := WriteDescriptor(paths, fresh); err != nil {
		t.Fatalf("WriteDescriptor(fresh): %v", err)
	}

	got, err := ReadDescriptor(paths)
	if err != nil {
		t.Fatalf("ReadDescriptor: %v", err)
	}
	if !reflect.DeepEqual(got, fresh) {
		t.Fatalf("descriptor=%#v, want %#v", got, fresh)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("runtime dir permissions=%#o, want 0700", perm)
	}
	fileInfo, err := os.Stat(paths.Environment)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("environment permissions=%#o, want 0600", perm)
	}

	matches, err := filepath.Glob(filepath.Join(dir, ".environment.json.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary descriptor files remain after rename: %v", matches)
	}
}

func TestDescriptorAtomicReplacementNeverExposesPartialJSON(t *testing.T) {
	dir := t.TempDir()
	paths, err := ResolvePaths(filepath.Join(dir, "a2ui.sock"))
	if err != nil {
		t.Fatal(err)
	}
	first := testDescriptor("instance-a")
	second := testDescriptor("instance-b")
	if err := WriteDescriptor(paths, first); err != nil {
		t.Fatalf("initial WriteDescriptor: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			want := first
			if i%2 == 1 {
				want = second
			}
			if err := WriteDescriptor(paths, want); err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}
	}()

	for i := 0; i < 200; i++ {
		got, err := ReadDescriptor(paths)
		if err != nil {
			t.Fatalf("ReadDescriptor during replacement: %v", err)
		}
		if !reflect.DeepEqual(got, first) && !reflect.DeepEqual(got, second) {
			t.Fatalf("observed partial/unexpected descriptor: %#v", got)
		}
	}
	wg.Wait()
	select {
	case err := <-errCh:
		t.Fatalf("writer error: %v", err)
	default:
	}
}

func TestValidateDescriptorRecognizesIdentityMismatch(t *testing.T) {
	expected := testDescriptor("instance-a")
	if err := ValidateDescriptor(expected, expected); err != nil {
		t.Fatalf("matching descriptor rejected: %v", err)
	}
	actual := expected
	actual.InstanceID = "instance-b"
	if err := ValidateDescriptor(actual, expected); !errors.Is(err, ErrDescriptorMismatch) {
		t.Fatalf("identity mismatch error=%v, want ErrDescriptorMismatch", err)
	}
}
