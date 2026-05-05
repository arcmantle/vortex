package release

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractBinariesDetectsZipWithoutExtension(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "downloaded-archive")
	writeZipArchive(t, archivePath, map[string]string{
		"nested/vortex.exe":      "gui",
		"nested/vortex-host.exe": "host",
	})

	extracted, err := ExtractBinaries(archivePath, t.TempDir(), []string{"vortex.exe", "vortex-host.exe"})
	if err != nil {
		t.Fatalf("ExtractBinaries() error = %v", err)
	}

	assertExtractedFile(t, extracted, "vortex.exe", "gui")
	assertExtractedFile(t, extracted, "vortex-host.exe", "host")
}

func TestExtractBinariesDetectsTarGzWithoutExtension(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "downloaded-archive")
	writeTarGzArchive(t, archivePath, map[string]string{
		"nested/vortex":      "gui",
		"nested/vortex-host": "host",
	})

	extracted, err := ExtractBinaries(archivePath, t.TempDir(), []string{"vortex", "vortex-host"})
	if err != nil {
		t.Fatalf("ExtractBinaries() error = %v", err)
	}

	assertExtractedFile(t, extracted, "vortex", "gui")
	assertExtractedFile(t, extracted, "vortex-host", "host")
}

func writeZipArchive(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("os.Create(%q): %v", archivePath, err)
	}

	zw := zip.NewWriter(f)
	for name, contents := range files {
		w, err := zw.Create(name)
		if err != nil {
			f.Close()
			t.Fatalf("zip Create(%q): %v", name, err)
		}
		if _, err := w.Write([]byte(contents)); err != nil {
			f.Close()
			t.Fatalf("zip Write(%q): %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		t.Fatalf("zip Close(): %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("file Close(): %v", err)
	}
}

func writeTarGzArchive(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("os.Create(%q): %v", archivePath, err)
	}

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, contents := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(contents)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			f.Close()
			t.Fatalf("tar WriteHeader(%q): %v", name, err)
		}
		if _, err := tw.Write([]byte(contents)); err != nil {
			f.Close()
			t.Fatalf("tar Write(%q): %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		f.Close()
		t.Fatalf("tar Close(): %v", err)
	}
	if err := gz.Close(); err != nil {
		f.Close()
		t.Fatalf("gzip Close(): %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("file Close(): %v", err)
	}
}

func assertExtractedFile(t *testing.T, extracted map[string]string, name, want string) {
	t.Helper()

	path, ok := extracted[name]
	if !ok {
		t.Fatalf("missing extracted file %q", name)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q): %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("contents for %q = %q, want %q", name, string(got), want)
	}
}