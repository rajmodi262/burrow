package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// image.go is a minimal, dependency-free client for the Docker/OCI registry v2
// HTTP API. It fetches an anonymous pull token, resolves the manifest (handling
// multi-arch image indexes), downloads each gzipped layer and unpacks them --
// with OverlayFS-style whiteout handling -- into a local rootfs directory.
//
// This is what `podman pull` / OpenShift's image machinery do, in ~150 lines.

const registryHost = "https://registry-1.docker.io"

var manifestAccept = strings.Join([]string{
	"application/vnd.docker.distribution.manifest.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.index.v1+json",
}, ", ")

func imageCacheRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".burrow", "images")
}

// pullImage resolves an image reference (e.g. "alpine", "library/nginx:1.27"),
// pulls its layers, unpacks them into a cached rootfs and returns that path.
func pullImage(ref string) (string, error) {
	name, tag := parseRef(ref)
	cache := filepath.Join(imageCacheRoot(), strings.ReplaceAll(name, "/", "_")+"_"+tag)
	rootfs := filepath.Join(cache, "rootfs")
	if _, err := os.Stat(filepath.Join(cache, ".done")); err == nil {
		return rootfs, nil // already pulled
	}
	fmt.Fprintf(os.Stderr, "burrow: pulling %s:%s ...\n", name, tag)

	token, err := getToken(name)
	if err != nil {
		return "", err
	}
	layers, err := getLayers(name, tag, token)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(rootfs, 0o755); err != nil {
		return "", err
	}
	for i, dg := range layers {
		fmt.Fprintf(os.Stderr, "  layer %d/%d %s\n", i+1, len(layers), shortDigest(dg))
		blob, err := getBlob(name, dg, token)
		if err != nil {
			return "", err
		}
		err = extractLayer(blob, rootfs)
		blob.Close()
		if err != nil {
			return "", fmt.Errorf("extract layer %d: %w", i+1, err)
		}
	}
	if err := os.WriteFile(filepath.Join(cache, ".done"), []byte("ok\n"), 0o644); err != nil {
		return "", err
	}
	return rootfs, nil
}

func pullCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: burrow pull <image>")
		os.Exit(2)
	}
	rootfs, err := pullImage(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "burrow: pull:", err)
		os.Exit(1)
	}
	fmt.Println(rootfs)
}

func parseRef(ref string) (name, tag string) {
	tag = "latest"
	if i := strings.LastIndex(ref, ":"); i >= 0 && !strings.Contains(ref[i:], "/") {
		tag, ref = ref[i+1:], ref[:i]
	}
	if !strings.Contains(ref, "/") {
		ref = "library/" + ref // official images live under library/
	}
	return ref, tag
}

func getToken(name string) (string, error) {
	u := "https://auth.docker.io/token?service=registry.docker.io&scope=repository:" + name + ":pull"
	resp, err := http.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var t struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	if t.Token == "" {
		return "", fmt.Errorf("empty pull token")
	}
	return t.Token, nil
}

func getManifestRaw(name, ref, token string) ([]byte, error) {
	req, _ := http.NewRequest("GET", registryHost+"/v2/"+name+"/manifests/"+ref, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", manifestAccept)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("manifest %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}

// getLayers returns the ordered layer digests, following an image index to the
// linux/amd64 manifest when necessary.
func getLayers(name, tag, token string) ([]string, error) {
	raw, err := getManifestRaw(name, tag, token)
	if err != nil {
		return nil, err
	}
	var m struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
		} `json:"manifests"`
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if len(m.Manifests) > 0 { // multi-arch index: pick linux/amd64
		digest := ""
		for _, e := range m.Manifests {
			if e.Platform.Architecture == "amd64" && e.Platform.OS == "linux" {
				digest = e.Digest
				break
			}
		}
		if digest == "" {
			return nil, fmt.Errorf("no linux/amd64 manifest in index")
		}
		if raw, err = getManifestRaw(name, digest, token); err != nil {
			return nil, err
		}
		m.Layers = nil
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
	}
	var out []string
	for _, l := range m.Layers {
		out = append(out, l.Digest)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("manifest has no layers")
	}
	return out, nil
}

func getBlob(name, digest, token string) (io.ReadCloser, error) {
	req, _ := http.NewRequest("GET", registryHost+"/v2/"+name+"/blobs/"+digest, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("blob %s: %s", shortDigest(digest), resp.Status)
	}
	return resp.Body, nil
}

// extractLayer unpacks one gzipped tar layer over dest, applying OverlayFS
// whiteouts (.wh.<name> deletes; .wh..wh..opq clears the directory).
func extractLayer(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if name == "." || strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			continue // ignore unsafe paths
		}
		target := filepath.Join(dest, name)
		dir, base := filepath.Dir(target), filepath.Base(name)

		if base == ".wh..wh..opq" {
			if entries, e := os.ReadDir(dir); e == nil {
				for _, en := range entries {
					os.RemoveAll(filepath.Join(dir, en.Name()))
				}
			}
			continue
		}
		if strings.HasPrefix(base, ".wh.") {
			os.RemoveAll(filepath.Join(dir, strings.TrimPrefix(base, ".wh.")))
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			os.MkdirAll(dir, 0o755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			os.RemoveAll(target)
			os.Symlink(hdr.Linkname, target)
		case tar.TypeLink:
			os.RemoveAll(target)
			os.Link(filepath.Join(dest, filepath.Clean(hdr.Linkname)), target)
		}
	}
}

func shortDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		d = d[:12]
	}
	return d
}
