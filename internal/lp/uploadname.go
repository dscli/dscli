package lp

// Upload-name policy for chat.deepseek.com attachments.
//
// The site inspects a file name's extension and accepts only a subset of the
// extensions its client-side filter knows: a rejected extension (".gitignore")
// renders a "该格式暂不支持" card that blocks the send until the attachment is
// deleted by hand, while an extension missing from the client list (".sum",
// ".work", ".env") is dropped silently, and a name without any extension
// (Makefile) is dropped too. Appending ".txt" was accepted in every probed
// case, so the policy is: keep a verified extension as-is, otherwise upload a
// copy of the same name with ".txt" appended (content untouched).

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// verifiedUploadExts lists the extensions (leading dot included) that the site
// accepted as-is in the live probes of 2026-09-13: text and document formats,
// the image formats it reads text from, and PDF. Anything else, including an
// empty extension, is uploaded with a ".txt" suffix.
//
// The set holds the lower-case spellings the probes exercised, and matching is
// exact and case-sensitive: an unprobed spelling (".MD", ".PNG") takes the
// fail-safe rename, because a site that rejects the extension blocks the whole
// send while a rename only changes the visible name. Widen this (and the
// probe battery) only on evidence that the site accepts more forms.
//
// Re-verify by running TestLiveUploadNameProbe (gated behind
// DSCLI_LIVE_UPLOAD_PROBE=1) and updating this set from its output. The set is
// deliberately conservative: an unknown extension always gets the suffix, so
// site drift degrades to a visible rename instead of a blocked send.
//
// The probe battery is capped at the site's per-batch file limit, so it is a
// representative sample, not an exhaustive sweep: several entries here (for
// example ".yaml", ".scss", ".pl") come from the same 2026-09-13 probe round
// but are no longer covered by the current battery. On the next probe round,
// re-check those entries or fold them back into the battery while it stays
// within WebUploadMaxFiles.
var verifiedUploadExts = map[string]bool{
	".md": true, ".go": true, ".patch": true, ".txt": true,
	".yml": true, ".yaml": true, ".json": true, ".toml": true,
	".ini": true, ".conf": true, ".cfg": true, ".log": true,
	".csv": true, ".tsv": true, ".lock": true, ".mod": true,
	".sh": true, ".py": true, ".rs": true, ".ts": true, ".js": true,
	".css": true, ".scss": true, ".html": true, ".xml": true,
	".sql": true, ".proto": true, ".org": true, ".el": true, ".mk": true,
	".rb": true, ".pl": true, ".c": true, ".cpp": true, ".h": true,
	".java": true, ".cs": true, ".kt": true, ".swift": true, ".php": true,
	// Images and PDF: the site extracts text from them, so they upload as-is.
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".bmp": true, ".pdf": true,
}

// SafeUploadName returns an upload name the site accepts. A name whose
// extension is in verifiedUploadExts (exact, case-sensitive match) comes back
// unchanged with renamed false. Any other name - an unknown extension, an
// unprobed upper-case spelling, or no extension at all - gets ".txt" appended
// with renamed true; the content is untouched, so nothing is lost, and the
// caller can report the adjustment.
//
// One or more trailing dots name no format ("foo.", "a.."), so they are all
// dropped and the extension is re-derived from what remains: "foo." becomes
// "foo.txt", "foo.txt." becomes "foo.txt" (not "foo.txt.txt"), and "a.."
// becomes "a.txt" (not "a..txt"). Dropping dots already changes the name, so
// the result is reported as renamed even when the remaining extension is
// verified.
//
// A hidden file whose whole name is a verified extension (".txt", ".md") is
// passed through: filepath.Ext sees the whole name as the extension, and the
// policy keys on the extension alone. This follows from the extension model
// rather than from a direct measurement - the probe battery now carries a bare
// ".md" candidate so the next probe round can confirm it.
//
// ".svg" is deliberately NOT verified: the site treats it as an image and
// reports no extracted text, which is useless for a text review, so an SVG is
// renamed like any unknown extension.
func SafeUploadName(name string) (string, bool) {
	ext := filepath.Ext(name)
	renamed := false
	if ext == "." {
		name = strings.TrimRight(name, ".")
		ext = filepath.Ext(name)
		renamed = true
	}
	if verifiedUploadExts[ext] {
		return name, renamed
	}
	return name + ".txt", true
}

// UniqueUploadName inserts a numeric suffix before the extension when an
// earlier file already claimed the same upload name (x.go -> x_2.go,
// Makefile.txt -> Makefile_2.txt). The suffix must precede the extension:
// both callers hand the result to the site, which decides acceptance by
// extension, so a name that ended in "_2" would be rejected again.
//
// Two callers share it so the rule lives in one place:
//   - prepareUploadAttachments de-duplicates a normalized upload batch;
//   - code_review de-duplicates encoded attachment names (a/b__c and a__b/c
//     both encode to a__b__c).
//
// used is updated in place; the caller seeds it with any names already taken.
func UniqueUploadName(used map[string]bool, name string) string {
	if !used[name] {
		used[name] = true
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, i, ext)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

// uploadTempDir creates the private directory that holds the renamed copies.
// It is a package variable so tests can confine the directory to a test-owned
// root instead of scanning the system temp directory; production always uses
// the "dscli-upload-" prefix in os.TempDir.
var uploadTempDir = func() (string, error) {
	return os.MkdirTemp("", "dscli-upload-")
}

// preparedUploads is one normalized attachment batch: the paths to hand to
// Chrome, a stderr note per adjusted name, and the cleanup that removes the
// renamed copies.
type preparedUploads struct {
	files   []string
	notes   []string
	cleanup func()
}

// prepareUploadAttachments normalizes a batch of attachment paths for the
// site (called by webchatUpload). A file keeps its original path unless the
// site would reject its name (see SafeUploadName) or another file already
// claimed that name; such a file is copied into a private temp dir under the
// accepted name, so the original is never modified and stays 0600.
//
// The caller uploads the returned paths and must run cleanup (nil when
// nothing was copied, so a clean batch has no side effect). Upload limits are
// validated on the ORIGINAL paths before this runs, so a rename never changes
// the file count or the byte budget. On error the temp dir is removed before
// returning, so a failed batch leaks nothing.
func prepareUploadAttachments(files []string) (preparedUploads, error) {
	prepared := preparedUploads{files: make([]string, len(files))}
	used := make(map[string]bool, len(files))
	var dir string
	for i, path := range files {
		base := filepath.Base(path)
		safe, renamed := SafeUploadName(base)
		name := UniqueUploadName(used, safe)
		if !renamed && name == base {
			prepared.files[i] = path
			continue
		}
		if dir == "" {
			d, err := uploadTempDir()
			if err != nil {
				return preparedUploads{}, fmt.Errorf("创建上传临时目录失败: %w", err)
			}
			dir = d
			prepared.cleanup = func() { os.RemoveAll(dir) }
		}
		dst := filepath.Join(dir, name)
		if err := copyUploadFile(dst, path); err != nil {
			prepared.cleanup()
			return preparedUploads{}, err
		}
		prepared.files[i] = dst
		prepared.notes = append(prepared.notes, uploadNote(base, safe, name))
	}
	return prepared, nil
}

// uploadNote renders the stderr line for one adjusted attachment name, in the
// style of the surrounding "📎" upload messages. The reason is classified in a
// fixed order so the wording always names the actual cause:
//
//  1. de-duplication - the normalized name was already taken (safe != name);
//  2. trailing dots - the name ended with a dot and was normalized;
//  3. an extension the site does not accept.
//
// A name can be adjusted for more than one reason at once; the first match
// wins, which keeps the note deterministic.
func uploadNote(base, safe, name string) string {
	switch {
	case name != safe:
		return fmt.Sprintf("📎 %s 与已有附件重名，已按 %s 上传（内容不变）", base, name)
	case strings.HasSuffix(base, "."):
		return fmt.Sprintf("📎 %s 的结尾点已规范化，已按 %s 上传（内容不变）", base, name)
	default:
		return fmt.Sprintf("📎 %s 的扩展名网站不支持，已按 %s 上传（内容不变）", base, name)
	}
}

// copyUploadFile copies src to dst with 0600 permissions: the site only reads
// the name and the content, and the copy holds the same data as the original,
// so it must not be readable by other local users.
func copyUploadFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开附件 %s 失败: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("创建上传副本 %s 失败: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("复制附件 %s 失败: %w", src, err)
	}
	return out.Close()
}
