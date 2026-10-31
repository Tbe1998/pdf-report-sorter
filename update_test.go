package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// 端到端：拉 latest.json → 解析相对地址 → 下载整包 → sha256 校验 → 解压
func TestUpdateFlow(t *testing.T) {
	pkg := makeZip(t, map[string]string{
		"PDF文件夹搬运助手.exe": "FAKE-EXE-CONTENT",
		"使用说明.txt":       "说明",
	})

	mux := http.NewServeMux()
	var srvURL string
	mux.HandleFunc("/files/PDFFolderMover-v9.9.9.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Write(pkg)
	})
	mux.HandleFunc("/files/latest.json", func(w http.ResponseWriter, r *http.Request) {
		sum := sha256.Sum256(pkg)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version":        "9.9.9",
			"notes":          "测试更新",
			"package":        "PDFFolderMover-v9.9.9.zip",
			"package_size":   len(pkg),
			"package_sha256": hex.EncodeToString(sum[:]),
			"exe_name":       "PDF文件夹搬运助手.exe",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	man, err := fetchManifest(srvURL + "/files/latest.json")
	if err != nil {
		t.Fatalf("拉取清单失败: %v", err)
	}
	if man.Version != "9.9.9" || !man.isPackage() {
		t.Fatalf("清单解析错误: %+v", man)
	}
	if man.Package != srvURL+"/files/PDFFolderMover-v9.9.9.zip" {
		t.Fatalf("相对地址未解析正确: %s", man.Package)
	}
	if compareVersion(man.Version, appVersion) <= 0 {
		t.Fatal("版本比较应判断为新版本")
	}

	path, err := downloadPackage(man)
	if err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	defer os.Remove(path)

	dir := t.TempDir()
	if err := extractZip(path, dir); err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	exe, err := findExeIn(dir, man.ExeName)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(exe)
	if err != nil || string(b) != "FAKE-EXE-CONTENT" {
		t.Fatalf("解压内容不对: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(dir, "使用说明.txt")); err != nil {
		t.Fatal("更新包里的其它文件没解出来")
	}
}

// sha256 不对必须拒绝
func TestUpdateBadHash(t *testing.T) {
	pkg := makeZip(t, map[string]string{"a.exe": "abc"})
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/p.zip", func(w http.ResponseWriter, r *http.Request) { w.Write(pkg) })
	mux.HandleFunc("/latest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": "9.9.9", "package": base + "/p.zip", "package_sha256": "deadbeef",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL
	man, err := fetchManifest(srv.URL + "/latest.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := downloadPackage(man); err == nil {
		t.Fatal("sha256 不匹配却没有报错")
	}
}

// 只给单文件 url（没有 package）时要明确拒绝
func TestManifestWithoutPackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.1.1", "url": "a.exe"})
	}))
	defer srv.Close()
	if _, err := fetchManifest(srv.URL + "/latest.json"); err == nil {
		t.Fatal("缺少 package 字段应报错")
	}
}

func TestFetchManifest404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()
	if _, err := fetchManifest(srv.URL + "/latest.json"); err == nil {
		t.Fatal("404 应返回错误")
	}
}

// 解压要挡住 ../ 目录穿越
func TestExtractZipSlip(t *testing.T) {
	pkg := makeZip(t, map[string]string{
		"../evil.txt": "x",
		"ok.txt":      "y",
	})
	dir := t.TempDir()
	src := filepath.Join(dir, "p.zip")
	if err := os.WriteFile(src, pkg, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := extractZip(src, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
		t.Fatal("目录穿越没有被挡住")
	}
	if _, err := os.Stat(filepath.Join(out, "ok.txt")); err != nil {
		t.Fatal("正常文件应该被解出来")
	}
}
