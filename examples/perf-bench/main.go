// perf-bench harness. Scans a directory using hush as a library and
// reports wallclock, peak heap, and finding count. Run alongside the
// hush CLI binary against the same input to compare the two paths.
//
//	bench --repo /path/to/repo --json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/valllabh/hush"
)

type result struct {
	Mode             string  `json:"mode"`
	Repo             string  `json:"repo"`
	FilesScanned     int     `json:"files_scanned"`
	BytesScanned     int64   `json:"bytes_scanned"`
	Findings         int     `json:"findings"`
	WallSeconds      float64 `json:"wall_seconds"`
	HeapAllocPeakMB  float64 `json:"heap_alloc_peak_mb"`
	HeapInUsePeakMB  float64 `json:"heap_inuse_peak_mb"`
}

func main() {
	repo := flag.String("repo", "", "Path to repository to scan")
	asJSON := flag.Bool("json", false, "Emit one JSON object per result line")
	useDetector := flag.Bool("v2", true, "Use the v2 NER detector (false = v1 regex+classifier)")
	prefilter := flag.Bool("prefilter", true, "Enable detector prefilter (v2 only)")
	flag.Parse()

	if *repo == "" {
		fmt.Fprintln(os.Stderr, "usage: bench --repo /path [--json] [--v2=false] [--prefilter=false]")
		os.Exit(2)
	}

	mode := "lib-v2"
	if !*useDetector {
		mode = "lib-v1"
	}
	if !*prefilter && *useDetector {
		mode += "-noprefilter"
	}

	s, err := hush.New(hush.Options{
		UseDetector:       *useDetector,
		DetectorPrefilter: *prefilter,
		MinConfidence:     0.5,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		os.Exit(2)
	}
	defer s.Close()

	r := result{Mode: mode, Repo: *repo}
	t0 := time.Now()

	// Periodic memory sampling. Cheap because runtime.ReadMemStats is
	// called once per file.
	var ms runtime.MemStats

	err = filepath.WalkDir(*repo, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip vendor / node_modules / .git which dominate noise.
			n := d.Name()
			if n == ".git" || n == "node_modules" || n == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip large files; perf-bench measures normal source code.
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		if info.Size() > 1<<20 {
			return nil
		}
		f, oerr := os.Open(path)
		if oerr != nil {
			return nil
		}
		findings, serr := s.ScanReader(f)
		f.Close()
		if serr != nil {
			return nil
		}
		r.FilesScanned++
		r.BytesScanned += info.Size()
		r.Findings += len(findings)

		runtime.ReadMemStats(&ms)
		if mb := float64(ms.HeapAlloc) / (1 << 20); mb > r.HeapAllocPeakMB {
			r.HeapAllocPeakMB = mb
		}
		if mb := float64(ms.HeapInuse) / (1 << 20); mb > r.HeapInUsePeakMB {
			r.HeapInUsePeakMB = mb
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk: %v\n", err)
	}

	r.WallSeconds = time.Since(t0).Seconds()

	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(r)
		return
	}
	fmt.Printf("%-18s repo=%s files=%d bytes=%s findings=%d wall=%.2fs heap_peak=%.0fMB\n",
		r.Mode, filepath.Base(strings.TrimRight(r.Repo, "/")),
		r.FilesScanned, humanBytes(r.BytesScanned), r.Findings,
		r.WallSeconds, r.HeapInUsePeakMB)
}

func humanBytes(n int64) string {
	const k = 1024
	if n < k {
		return fmt.Sprintf("%dB", n)
	}
	if n < k*k {
		return fmt.Sprintf("%.1fK", float64(n)/k)
	}
	if n < k*k*k {
		return fmt.Sprintf("%.1fM", float64(n)/(k*k))
	}
	return fmt.Sprintf("%.1fG", float64(n)/(k*k*k))
}
