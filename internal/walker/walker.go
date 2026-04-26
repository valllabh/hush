// Package walker collects scannable files from a set of paths.
package walker

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
)

// DefaultSkipDirs are directories we never descend into (matched by basename).
var DefaultSkipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true,
	"node_modules": true, "bower_components": true, "vendor": true,
	".venv": true, "venv": true, "env": true, "__pycache__": true, ".tox": true,
	".next": true, ".nuxt": true, "dist": true, "build": true,
	"target": true, ".gradle": true, ".idea": true, ".vscode": true,
	".cache": true, ".pytest_cache": true, ".ruff_cache": true, ".mypy_cache": true,
	".terraform": true, ".angular": true, ".svelte-kit": true, ".parcel-cache": true,
}

// DefaultSkipExts are binary/noise extensions we skip by default.
var DefaultSkipExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".ico": true, ".bmp": true,
	".mp3": true, ".mp4": true, ".wav": true, ".mov": true, ".avi": true, ".ogg": true,
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true, ".7z": true, ".rar": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
	".so": true, ".dylib": true, ".dll": true, ".a": true, ".o": true, ".class": true, ".jar": true,
	".pyc": true, ".pyo": true, ".exe": true, ".bin": true, ".wasm": true,
	".onnx": true, ".safetensors": true, ".pt": true, ".pth": true, ".ckpt": true, ".h5": true,
	".ttf": true, ".woff": true, ".woff2": true, ".eot": true,
	".lock": true, ".sum": true,
}

// Options tunes the walk.
type Options struct {
	MaxFileSize     int64           // skip files larger than this (default 10 MB)
	SkipDirs        map[string]bool // dir basenames to skip (matches anywhere)
	SkipExts        map[string]bool // extensions to skip
	OnlyExts        map[string]bool // if non-empty, allowlist of extensions
	SkipPaths       []string        // absolute path prefixes to exclude entirely
	SkipPatterns    []string        // doublestar globs; match excludes the path (e.g. **/dump/**)
	IncludePatterns []string        // doublestar globs; if non-empty, only matching paths scanned
	FollowLinks     bool
}

// DefaultOptions returns sane defaults.
func DefaultOptions() Options {
	return Options{
		MaxFileSize: 10 * 1024 * 1024,
		SkipDirs:    DefaultSkipDirs,
		SkipExts:    DefaultSkipExts,
	}
}

// Walk expands roots (files or dirs) into a stream of file paths sent on out.
// Closes out when done. Errors are sent on errs; walking continues past them.
func Walk(roots []string, opts Options, out chan<- string, errs chan<- error) {
	defer close(out)
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			errs <- err
			continue
		}
		if !info.IsDir() {
			if shouldScan(root, info, opts) && !LooksBinary(root) {
				out <- root
			}
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				errs <- err
				return nil
			}
			absPath, aerr := filepath.Abs(path)
			if aerr != nil {
				absPath = path
			}
			if d.IsDir() {
				name := d.Name()
				if opts.SkipDirs[name] {
					return filepath.SkipDir
				}
				if pathPrefixMatch(absPath, opts.SkipPaths) {
					return filepath.SkipDir
				}
				if globMatch(absPath, opts.SkipPatterns) {
					return filepath.SkipDir
				}
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil {
				errs <- ierr
				return nil
			}
			if pathPrefixMatch(absPath, opts.SkipPaths) {
				return nil
			}
			if globMatch(absPath, opts.SkipPatterns) {
				return nil
			}
			if len(opts.IncludePatterns) > 0 && !globMatch(absPath, opts.IncludePatterns) {
				return nil
			}
			if !shouldScan(path, info, opts) {
				return nil
			}
			if LooksBinary(path) {
				return nil
			}
			out <- path
			return nil
		})
	}
}

func shouldScan(path string, info fs.FileInfo, opts Options) bool {
	if info.Mode()&os.ModeType != 0 {
		return false // skip symlinks/devices/etc
	}
	if info.Size() == 0 || info.Size() > opts.MaxFileSize {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	if len(opts.OnlyExts) > 0 {
		return opts.OnlyExts[ext]
	}
	if opts.SkipExts[ext] {
		return false
	}
	return true
}

// LooksBinary returns true if the file at path appears to be binary based
// on a 512-byte sniff: invalid UTF-8 or more than 30% non-printable bytes
// classifies as binary. Used to skip files that would otherwise feed
// garbage into the regex/tokenizer pipeline. Read errors are conservative:
// treat as binary so we skip rather than crash on the next read.
func LooksBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	var buf [512]byte
	n, _ := f.Read(buf[:])
	if n == 0 {
		return false
	}
	b := buf[:n]
	// NUL byte is the classic binary signal.
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	if !utf8.Valid(b) {
		return true
	}
	nonPrintable := 0
	for _, c := range b {
		// allow tab, LF, CR, FF, and printable ASCII (0x20-0x7E); count
		// other control bytes as non-printable. UTF-8 multi-byte (>= 0x80)
		// is allowed since utf8.Valid passed.
		if c == '\t' || c == '\n' || c == '\r' || c == '\f' {
			continue
		}
		if c < 0x20 || c == 0x7F {
			nonPrintable++
		}
	}
	return float64(nonPrintable)/float64(n) > 0.30
}

// pathPrefixMatch returns true if any of the given absolute prefixes matches.
func pathPrefixMatch(abs string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return false
	}
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		// Match either exact prefix or prefix + path separator to avoid
		// /foo/bar matching /foo/barbaz.
		if abs == p || strings.HasPrefix(abs, p+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// globMatch returns true if any doublestar glob matches the abs path.
func globMatch(abs string, globs []string) bool {
	for _, g := range globs {
		if g == "" {
			continue
		}
		ok, _ := doublestar.Match(g, abs)
		if ok {
			return true
		}
	}
	return false
}

// ClassifyPattern auto-routes a user string into the right Options field.
// This is what powers the unified --file-include / --file-exclude flag so
// users don't have to pick between path/basename/ext/pattern.
//
//	/abs  or ~/abs      -> path prefix
//	*.ext               -> extension
//	contains * or ?     -> wildcard pattern
//	plain word          -> directory/file basename (matches anywhere)
//
// apply=true installs into Skip* (exclude); false into Include* (allowlist).
func ClassifyPattern(raw string, apply bool, opts *Options) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	// Absolute or home-expanded path prefix.
	if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "~/") {
		norm := NormalisePaths([]string{raw})
		if apply {
			opts.SkipPaths = append(opts.SkipPaths, norm...)
		} else {
			for _, p := range norm {
				opts.IncludePatterns = append(opts.IncludePatterns, p+"/**")
			}
		}
		return
	}
	// Extension-only shorthand: *.ext
	if strings.HasPrefix(raw, "*.") && !strings.ContainsAny(raw[2:], "*?/") {
		ext := "." + strings.ToLower(raw[2:])
		if apply {
			opts.SkipExts[ext] = true
		} else {
			if opts.OnlyExts == nil {
				opts.OnlyExts = map[string]bool{}
			}
			opts.OnlyExts[ext] = true
		}
		return
	}
	// Wildcard pattern anywhere.
	if strings.ContainsAny(raw, "*?") {
		if apply {
			opts.SkipPatterns = append(opts.SkipPatterns, raw)
		} else {
			opts.IncludePatterns = append(opts.IncludePatterns, raw)
		}
		return
	}
	// Bare name -> basename match anywhere.
	if apply {
		if opts.SkipDirs == nil {
			opts.SkipDirs = map[string]bool{}
		}
		opts.SkipDirs[raw] = true
	} else {
		opts.IncludePatterns = append(opts.IncludePatterns, "**/"+raw+"/**")
	}
}

// NormalisePaths converts user-supplied paths (maybe relative, maybe `~/`) to
// cleaned absolute paths for prefix matching.
func NormalisePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	home, _ := os.UserHomeDir()
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "~/") && home != "" {
			p = filepath.Join(home, p[2:])
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		out = append(out, filepath.Clean(p))
	}
	return out
}

// ParseExts normalises a comma list like "py,env,yml" to {".py": true, ...}.
func ParseExts(csv string) map[string]bool {
	if csv == "" {
		return nil
	}
	out := map[string]bool{}
	for _, e := range strings.Split(csv, ",") {
		e = strings.TrimSpace(strings.ToLower(e))
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		out[e] = true
	}
	return out
}
